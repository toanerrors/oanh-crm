package sqlite

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"oanh.local/accounting/internal/accounting"
)

type BackupOptions struct {
	ID           string
	Kind         string
	ActorID      string
	ActorName    string
	Note         string
	RequestHash  string
	ScheduledFor string
}
type BackupInfo struct {
	ID           string   `json:"id"`
	Filename     string   `json:"filename"`
	Size         int64    `json:"size"`
	SHA256       string   `json:"sha256,omitempty"`
	VerifiedAt   string   `json:"verifiedAt,omitempty"`
	Kind         string   `json:"kind"`
	ActorID      string   `json:"actorId,omitempty"`
	ActorName    string   `json:"actorName,omitempty"`
	Note         string   `json:"note,omitempty"`
	RequestHash  string   `json:"-"`
	ScheduledFor string   `json:"scheduledFor,omitempty"`
	Manifest     Manifest `json:"manifest"`
}
type backupMetadata struct {
	BackupInfo
	RequestHash string `json:"requestHash,omitempty"`
}

func recoveryID(id string) bool {
	if len(id) == 0 || len(id) > 100 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func backupPath(root, id string) (string, error) {
	if !recoveryID(id) {
		return "", accounting.Fail("NOT_FOUND", "Không tìm thấy bản sao lưu.")
	}
	return filepath.Join(root, "backups", "backup-"+id+".zip"), nil
}
func backupID(path string) string {
	return strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "backup-"), ".zip")
}
func (s *Store) backupInfo(path string) (BackupInfo, error) {
	info := BackupInfo{ID: backupID(path), Filename: filepath.Base(path), Kind: "manual"}
	if !recoveryID(info.ID) {
		return info, fmt.Errorf("invalid backup id")
	}
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() {
		return info, fmt.Errorf("invalid backup file")
	}
	info.Size = stat.Size()
	b, err := os.ReadFile(path + ".json")
	if err == nil {
		var meta backupMetadata
		if err = json.Unmarshal(b, &meta); err != nil || meta.ID != info.ID || meta.Filename != info.Filename {
			return info, fmt.Errorf("invalid backup metadata")
		}
		meta.BackupInfo.RequestHash = meta.RequestHash
		if meta.Size != info.Size {
			meta.VerifiedAt = ""
		}
		meta.Size = info.Size
		return meta.BackupInfo, nil
	}
	if !os.IsNotExist(err) {
		return info, err
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		return info, err
	}
	defer archive.Close()
	for _, f := range archive.File {
		if f.Name != "manifest.json" {
			continue
		}
		r, e := f.Open()
		if e != nil {
			return info, e
		}
		raw, e := io.ReadAll(io.LimitReader(r, (1<<20)+1))
		r.Close()
		if e != nil || len(raw) > 1<<20 {
			return info, fmt.Errorf("invalid manifest")
		}
		if e = json.Unmarshal(raw, &info.Manifest); e != nil {
			return info, e
		}
		return info, nil
	}
	return info, fmt.Errorf("missing manifest")
}
func (s *Store) publishBackupMetadata(path string, m Manifest, options BackupOptions) (BackupInfo, error) {
	hash, size, err := checksumFile(path)
	if err != nil {
		return BackupInfo{}, err
	}
	info := BackupInfo{ID: backupID(path), Filename: filepath.Base(path), Size: size, SHA256: hash, VerifiedAt: Now(), Kind: options.Kind, ActorID: options.ActorID, ActorName: options.ActorName, Note: options.Note, RequestHash: options.RequestHash, ScheduledFor: options.ScheduledFor, Manifest: m}
	if info.Kind == "" {
		info.Kind = "manual"
	}
	raw, err := json.Marshal(backupMetadata{BackupInfo: info, RequestHash: options.RequestHash})
	if err != nil {
		return info, err
	}
	return info, WriteFile(path+".json", raw)
}
func (s *Store) ListBackups(ctx context.Context, company string) ([]BackupInfo, int, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "backups"))
	if os.IsNotExist(err) {
		return []BackupInfo{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	result := []BackupInfo{}
	unreadable := 0
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return nil, 0, err
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "backup-") || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, e := s.backupInfo(filepath.Join(s.Root, "backups", entry.Name()))
		if e != nil {
			unreadable++
			continue
		}
		if info.Manifest.CompanyID == company {
			result = append(result, info)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Manifest.CreatedAt > result[j].Manifest.CreatedAt })
	return result, unreadable, nil
}
func (s *Store) ReadBackup(ctx context.Context, id, company string, verify bool) (string, BackupInfo, error) {
	path, err := backupPath(s.Root, id)
	if err != nil {
		return "", BackupInfo{}, err
	}
	info, err := s.backupInfo(path)
	if err != nil || info.Manifest.CompanyID != company {
		return "", BackupInfo{}, accounting.Fail("NOT_FOUND", "Không tìm thấy bản sao lưu của doanh nghiệp.")
	}
	hash, _, err := checksumFile(path)
	if err != nil || info.SHA256 != "" && hash != info.SHA256 {
		return "", info, accounting.Fail("INVALID_BACKUP", "Tệp sao lưu đã thay đổi hoặc bị hỏng. Không thể sử dụng gói này.")
	}
	if verify || info.VerifiedAt == "" {
		staged, e := s.Stage(ctx, path)
		if e != nil {
			return "", info, accounting.Fail("INVALID_BACKUP", "Gói sao lưu không đạt kiểm tra toàn vẹn hoặc đối soát.")
		}
		defer os.RemoveAll(filepath.Join(s.Root, "staging", staged.ID))
		info, err = s.publishBackupMetadata(path, staged.Manifest, BackupOptions{Kind: info.Kind, ActorID: info.ActorID, ActorName: info.ActorName, Note: info.Note, RequestHash: info.RequestHash, ScheduledFor: info.ScheduledFor})
		if err != nil {
			return "", info, err
		}
	}
	return path, info, nil
}

type RecoveryCurrent struct {
	CompanyID   string `json:"companyId"`
	CompanyName string `json:"companyName"`
	Epoch       string `json:"epoch"`
	Revision    int64  `json:"revision"`
	Documents   int    `json:"documents"`
	Posted      int    `json:"posted"`
	Attachments int    `json:"attachments"`
}

func (s *Store) RecoveryCurrent(ctx context.Context) (RecoveryCurrent, error) {
	var current RecoveryCurrent
	tx, err := s.Read.BeginTx(ctx, nil)
	if err != nil {
		return current, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `SELECT company_id,company_name,epoch,revision,(SELECT count(*) FROM documents),(SELECT count(*) FROM documents WHERE status='posted'),(SELECT count(*) FROM attachments) FROM meta WHERE id=1`).Scan(&current.CompanyID, &current.CompanyName, &current.Epoch, &current.Revision, &current.Documents, &current.Posted, &current.Attachments)
	if err != nil {
		return current, err
	}
	return current, tx.Commit()
}

type RestorePreview struct {
	Staged
	Current   RecoveryCurrent `json:"current"`
	ExpiresAt string          `json:"expiresAt"`
}
type restoreApproval struct {
	RestorePreview
	ActorID string `json:"actorId"`
}

func (s *Store) PrepareRestore(ctx context.Context, path, actor string) (RestorePreview, error) {
	var result RestorePreview
	staged, err := s.Stage(ctx, path)
	if err != nil {
		return result, accounting.Fail("INVALID_BACKUP", "Gói không đạt kiểm tra checksum, phiên bản hoặc đối soát. Dữ liệu đang dùng vẫn được giữ nguyên.")
	}
	result.Staged = staged
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(filepath.Join(s.Root, "staging", staged.ID))
		}
	}()
	result.Current, err = s.RecoveryCurrent(ctx)
	if err != nil {
		return result, err
	}
	result.ExpiresAt = time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339Nano)
	raw, err := json.Marshal(restoreApproval{RestorePreview: result, ActorID: actor})
	if err != nil {
		return result, err
	}
	if err = WriteFile(filepath.Join(s.Root, "staging", staged.ID, "approval.json"), raw); err != nil {
		return result, err
	}
	ok = true
	return result, nil
}
func (s *Store) CheckRestoreApproval(ctx context.Context, id, actor string) (RestorePreview, error) {
	var approval restoreApproval
	if !recoveryID(id) {
		return approval.RestorePreview, accounting.Fail("PREVIEW_REQUIRED", "Hãy kiểm tra gói khôi phục trước.")
	}
	raw, err := os.ReadFile(filepath.Join(s.Root, "staging", id, "approval.json"))
	if err != nil || json.Unmarshal(raw, &approval) != nil || approval.ID != id || approval.ActorID != actor {
		return approval.RestorePreview, accounting.Fail("PREVIEW_REQUIRED", "Mở lại bước kiểm tra gói bằng tài khoản đang dùng.")
	}
	expiry, err := time.Parse(time.RFC3339Nano, approval.ExpiresAt)
	if err != nil || !time.Now().Before(expiry) {
		return approval.RestorePreview, accounting.Fail("PREVIEW_REQUIRED", "Kết quả kiểm tra đã hết hạn 30 phút. Hãy kiểm tra lại gói.")
	}
	current, err := s.RecoveryCurrent(ctx)
	if err != nil {
		return approval.RestorePreview, err
	}
	if current.Epoch != approval.Current.Epoch || current.CompanyID != approval.Current.CompanyID {
		return approval.RestorePreview, accounting.Fail("DATASET_CHANGED", "Phiên dữ liệu đã thay đổi. Hãy kiểm tra lại gói.")
	}
	if current.Revision != approval.Current.Revision {
		return approval.RestorePreview, accounting.Fail("VERSION_CONFLICT", "Dữ liệu đã thay đổi sau lúc kiểm tra. Hãy kiểm tra lại gói trước khi khôi phục.")
	}
	return approval.RestorePreview, nil
}

func (s *Store) BackupDetails(path string) (BackupInfo, error) { return s.backupInfo(path) }

type RecoveryEvent struct {
	ID                string `json:"id"`
	Phase             string `json:"phase"`
	ActorName         string `json:"actorName"`
	StartedAt         string `json:"startedAt"`
	FinishedAt        string `json:"finishedAt"`
	SnapshotCreatedAt string `json:"snapshotCreatedAt"`
	Reason            string `json:"reason"`
	PreRestoreBackup  string `json:"preRestoreBackup"`
}

func (s *Store) RecoveryEvents(company string) []RecoveryEvent {
	entries, err := os.ReadDir(filepath.Join(s.Root, "recovery"))
	if err != nil {
		return []RecoveryEvent{}
	}
	result := []RecoveryEvent{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.Root, "recovery", entry.Name()))
		if err != nil {
			continue
		}
		var event map[string]string
		if json.Unmarshal(raw, &event) != nil || event["companyId"] != company {
			continue
		}
		result = append(result, RecoveryEvent{ID: strings.TrimSuffix(entry.Name(), ".json"), Phase: event["phase"], ActorName: event["actorName"], StartedAt: event["startedAt"], FinishedAt: event["finishedAt"], SnapshotCreatedAt: event["snapshotCreatedAt"], Reason: event["reason"], PreRestoreBackup: backupID(event["preRestoreBackup"])})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartedAt > result[j].StartedAt })
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

func (s *Store) CleanupRestorePreviews(now time.Time) error {
	s.Gate.RLock()
	defer s.Gate.RUnlock()
	root := filepath.Join(s.Root, "staging")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !recoveryID(entry.Name()) {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(root, entry.Name(), "approval.json"))
		if e != nil {
			continue
		}
		var approval restoreApproval
		if json.Unmarshal(raw, &approval) != nil || approval.ID != entry.Name() {
			continue
		}
		expiry, e := time.Parse(time.RFC3339Nano, approval.ExpiresAt)
		if e == nil && now.After(expiry) {
			if e = os.RemoveAll(filepath.Join(root, entry.Name())); e != nil {
				return e
			}
		}
	}
	return nil
}
