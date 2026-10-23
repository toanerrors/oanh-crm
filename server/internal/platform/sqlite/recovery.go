package sqlite

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"oanh.local/accounting/internal/accounting"
)

var ApplicationVersion = "development"

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Manifest struct {
	ApplicationVersion string `json:"appVersion,omitempty"`
	Format             int    `json:"format"`
	SchemaVersion      int    `json:"schemaVersion,omitempty"`
	Attachments        *int   `json:"attachments,omitempty"`
	FirstDate          string `json:"firstDate,omitempty"`
	LastDate           string `json:"lastDate,omitempty"`
	SchemaHash         string `json:"schemaHash"`
	SQLiteVersion      string `json:"sqliteVersion"`
	CompanyID          string `json:"companyId"`
	CompanyName        string `json:"companyName"`
	Epoch              string `json:"epoch"`
	Revision           int64  `json:"revision"`
	CreatedAt          string `json:"createdAt"`
	Documents          int    `json:"documents"`
	Posted             int    `json:"posted"`
	Debit              string `json:"debit"`
	Credit             string `json:"credit"`
	Files              []File `json:"files,omitempty"`
}
type Staged struct {
	ID       string   `json:"id"`
	Manifest Manifest `json:"manifest"`
}

func checksumFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}
func reconcile(ctx context.Context, db *sql.DB) (Manifest, error) {
	var m Manifest
	m.Format = 1
	m.ApplicationVersion = ApplicationVersion
	var version int
	if err := db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		return m, err
	}
	m.SchemaHash = schemaHash(version)
	m.SchemaVersion = version
	m.Files = []File{}
	if err := Check(ctx, db); err != nil {
		return m, err
	}
	if err := db.QueryRowContext(ctx, "SELECT company_id,company_name,epoch,revision FROM meta WHERE id=1").Scan(&m.CompanyID, &m.CompanyName, &m.Epoch, &m.Revision); err != nil {
		return m, err
	}
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&m.SQLiteVersion); err != nil {
		return m, err
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*),COALESCE(min(date),''),COALESCE(max(date),'') FROM documents").Scan(&m.Documents, &m.FirstDate, &m.LastDate); err != nil {
		return m, err
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE status='posted'").Scan(&m.Posted); err != nil {
		return m, err
	}
	var attachmentCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM attachments").Scan(&attachmentCount); err != nil {
		return m, err
	}
	m.Attachments = &attachmentCount
	rows, err := db.QueryContext(ctx, "SELECT d.id,l.debit,l.credit FROM documents d JOIN document_lines l ON l.document_id=d.id WHERE d.status='posted' ORDER BY d.id,l.position")
	if err != nil {
		return m, err
	}
	defer rows.Close()
	debit, credit := new(big.Int), new(big.Int)
	balances := map[string]*big.Int{}
	for rows.Next() {
		var id string
		var d, c int64
		if err = rows.Scan(&id, &d, &c); err != nil {
			return m, err
		}
		if balances[id] == nil {
			balances[id] = new(big.Int)
		}
		balances[id].Add(balances[id], big.NewInt(d))
		balances[id].Sub(balances[id], big.NewInt(c))
		debit.Add(debit, big.NewInt(d))
		credit.Add(credit, big.NewInt(c))
	}
	if err = rows.Err(); err != nil {
		return m, err
	}
	for _, n := range balances {
		if n.Sign() != 0 {
			return m, fmt.Errorf("unbalanced posted document")
		}
	}
	m.Debit = debit.String()
	if len(balances) != m.Posted {
		return m, fmt.Errorf("posted document has no journal lines")
	}
	m.Credit = credit.String()
	return m, nil
}
func (s *Store) Backup(ctx context.Context) (string, Manifest, error) {
	return s.BackupWithOptions(ctx, BackupOptions{})
}
func (s *Store) BackupWithOptions(ctx context.Context, options BackupOptions) (string, Manifest, error) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	if options.ID != "" {
		existing, e := backupPath(s.Root, options.ID)
		if e != nil {
			return "", Manifest{}, e
		}
		if _, e = os.Stat(existing); e == nil {
			info, e := s.backupInfo(existing)
			if e != nil {
				return "", Manifest{}, e
			}
			if info.RequestHash != options.RequestHash {
				return "", Manifest{}, accounting.Fail("IDEMPOTENCY_CONFLICT", "Khóa tạo bản sao đã dùng với nội dung khác.")
			}
			path, checked, e := s.ReadBackup(ctx, options.ID, info.Manifest.CompanyID, true)
			return path, checked.Manifest, e
		} else if !os.IsNotExist(e) {
			return "", Manifest{}, e
		}
	}

	var m Manifest
	dir, err := os.MkdirTemp(s.Root, "backup-staging-")
	if err != nil {
		return "", m, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "accounting.sqlite")
	if _, err = s.Write.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return "", m, err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return "", m, err
	}
	err = f.Sync()
	f.Close()
	if err != nil {
		return "", m, err
	}
	db, err := Connect(ctx, path, true)
	if err != nil {
		return "", m, err
	}
	defer db.Close()
	m, err = reconcile(ctx, db)
	if err != nil {
		return "", m, err
	}
	m.CreatedAt = Now()
	hash, size, err := checksumFile(path)
	if err != nil {
		return "", m, err
	}
	m.Files = append(m.Files, File{Path: "accounting.sqlite", SHA256: hash, Size: size})
	rows, err := db.QueryContext(ctx, "SELECT DISTINCT hash,size FROM attachments ORDER BY hash")
	if err != nil {
		return "", m, err
	}
	for rows.Next() {
		var h string
		var size int64
		if err = rows.Scan(&h, &size); err != nil {
			rows.Close()
			return "", m, err
		}
		if len(h) != 64 || strings.ContainsAny(h, "/\\.") {
			rows.Close()
			return "", m, fmt.Errorf("invalid attachment hash")
		}
		source := filepath.Join(s.Dataset, "attachments", h)
		actual, n, e := checksumFile(source)
		if e != nil || actual != h || n != size {
			rows.Close()
			return "", m, fmt.Errorf("attachment verification failed")
		}
		if err = os.MkdirAll(filepath.Join(dir, "attachments"), 0700); err != nil {
			rows.Close()
			return "", m, err
		}
		b, e := os.ReadFile(source)
		if e != nil {
			rows.Close()
			return "", m, e
		}
		if e = WriteFile(filepath.Join(dir, "attachments", h), b); e != nil {
			rows.Close()
			return "", m, e
		}
		m.Files = append(m.Files, File{Path: "attachments/" + h, SHA256: h, Size: n})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", m, err
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		return "", m, err
	}
	if err = WriteFile(filepath.Join(dir, "manifest.json"), encoded); err != nil {
		return "", m, err
	}
	backupDir := filepath.Join(s.Root, "backups")
	if err = os.MkdirAll(backupDir, 0700); err != nil {
		return "", m, err
	}
	id := options.ID
	if id == "" {
		id = ID()
	}
	dest := filepath.Join(backupDir, "backup-"+id+".zip")
	pending := filepath.Join(backupDir, ".pending-"+ID()+".tmp")
	output, err := os.OpenFile(pending, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", m, err
	}
	ok := false
	defer func() {
		if !ok {
			os.Remove(pending)
		}
	}()
	archive := zip.NewWriter(output)
	for _, name := range append([]string{"manifest.json"}, fileNames(m.Files)...) {
		file, e := archive.Create(name)
		if e != nil {
			archive.Close()
			output.Close()
			return "", m, e
		}
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			archive.Close()
			output.Close()
			return "", m, e
		}
		if _, e = file.Write(b); e != nil {
			archive.Close()
			output.Close()
			return "", m, e
		}
	}
	if err = archive.Close(); err == nil {
		err = output.Sync()
	}
	closeErr := output.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", m, err
	}
	if err = SyncDir(backupDir); err != nil {
		return "", m, err
	}
	stat, e := os.Stat(pending)
	if e != nil {
		return "", m, e
	}
	if stat.Size() > 128<<20 {
		return "", m, accounting.Fail("BACKUP_TOO_LARGE", "Gói vượt giới hạn128 MB. Liên hệ quản trị để sao lưu dữ liệu lớn.")
	}
	stage, err := s.Stage(ctx, pending)
	if err != nil {
		return "", m, err
	}
	os.RemoveAll(filepath.Join(s.Root, "staging", stage.ID))
	hash, size, e = checksumFile(pending)
	if e != nil {
		return "", m, e
	}
	info := BackupInfo{ID: id, Filename: filepath.Base(dest), Size: size, SHA256: hash, VerifiedAt: Now(), Kind: options.Kind, ActorID: options.ActorID, ActorName: options.ActorName, Note: options.Note, RequestHash: options.RequestHash, ScheduledFor: options.ScheduledFor, Manifest: m}
	if info.Kind == "" {
		info.Kind = "manual"
	}
	metadata, e := json.Marshal(backupMetadata{BackupInfo: info, RequestHash: options.RequestHash})
	if e != nil {
		return "", m, e
	}
	if err = WriteFile(dest+".json", metadata); err != nil {
		return "", m, err
	}
	if err = os.Rename(pending, dest); err != nil {
		return "", m, err
	}
	if err = SyncDir(backupDir); err != nil {
		return "", m, err
	}
	ok = true
	return dest, m, nil
}
func fileNames(files []File) []string {
	names := []string{}
	for _, f := range files {
		names = append(names, f.Path)
	}
	return names
}
func (s *Store) Stage(ctx context.Context, archivePath string) (Staged, error) {
	var staged Staged
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return staged, err
	}
	defer archive.Close()
	if len(archive.File) > 10002 {
		return staged, fmt.Errorf("too many files")
	}
	staged.ID = ID()
	dir := filepath.Join(s.Root, "staging", staged.ID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return staged, err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()
	seen := map[string]bool{}
	var total uint64
	for _, f := range archive.File {
		name := f.Name
		if f.UncompressedSize64 > (256<<20)-total {
			return staged, fmt.Errorf("backup exceeds extraction limit")
		}
		total += f.UncompressedSize64
		if total > 256<<20 || f.UncompressedSize64 > 128<<20 || f.Mode()&os.ModeSymlink != 0 || f.FileInfo().IsDir() || !filepath.IsLocal(name) || strings.Contains(name, "\\") || seen[name] || name != "manifest.json" && name != "accounting.sqlite" && !(strings.HasPrefix(name, "attachments/") && len(strings.TrimPrefix(name, "attachments/")) == 64 && !strings.Contains(strings.TrimPrefix(name, "attachments/"), "/")) {
			return staged, fmt.Errorf("invalid backup entry")
		}
		seen[name] = true
		if err = os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0700); err != nil {
			return staged, err
		}
		reader, e := f.Open()
		if e != nil {
			return staged, e
		}
		b, e := io.ReadAll(io.LimitReader(reader, 128<<20+1))
		reader.Close()
		if e != nil || len(b) > 128<<20 {
			return staged, fmt.Errorf("invalid backup file size")
		}
		if err = WriteFile(filepath.Join(dir, name), b); err != nil {
			return staged, err
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil || len(b) > 1<<20 {
		return staged, fmt.Errorf("invalid manifest")
	}
	if err = json.Unmarshal(b, &staged.Manifest); err != nil {
		return staged, err
	}
	m := staged.Manifest
	if m.Format != 1 || (m.SchemaHash != schemaHash(1) && m.SchemaHash != schemaHash(2) && m.SchemaHash != schemaHash(3) && m.SchemaHash != schemaHash(4) && m.SchemaHash != schemaHash(5) && m.SchemaHash != schemaHash(6) && m.SchemaHash != schemaHash(7) && m.SchemaHash != schemaHash(8)) || len(seen) != len(m.Files)+1 {
		return staged, fmt.Errorf("unsupported backup format or schema")
	}
	listed := map[string]bool{}
	for _, file := range m.Files {
		if !seen[file.Path] || listed[file.Path] {
			return staged, fmt.Errorf("manifest file mismatch")
		}
		h, n, e := checksumFile(filepath.Join(dir, file.Path))
		if e != nil || h != file.SHA256 || n != file.Size {
			return staged, fmt.Errorf("backup checksum mismatch")
		}
		listed[file.Path] = true
	}
	db, err := Connect(ctx, filepath.Join(dir, "accounting.sqlite"), true)
	if err != nil {
		return staged, err
	}
	actual, err := reconcile(ctx, db)
	if err == nil && (actual.SchemaHash != m.SchemaHash || actual.CompanyID != m.CompanyID || actual.CompanyName != m.CompanyName || actual.Epoch != m.Epoch || actual.Revision != m.Revision || actual.Documents != m.Documents || actual.Posted != m.Posted || actual.Debit != m.Debit || actual.Credit != m.Credit) {
		err = fmt.Errorf("backup reconciliation mismatch")
	}
	if err == nil {
		rows, e := db.QueryContext(ctx, "SELECT DISTINCT hash,size FROM attachments")
		if e != nil {
			err = e
		} else {
			for rows.Next() {
				var hash string
				var size int64
				if e = rows.Scan(&hash, &size); e != nil {
					err = e
					break
				}
				h, n, e := checksumFile(filepath.Join(dir, "attachments", hash))
				if e != nil || h != hash || n != size {
					err = fmt.Errorf("backup attachment missing")
					break
				}
			}
			if e = rows.Err(); err == nil {
				err = e
			}
			rows.Close()
		}
	}
	db.Close()
	if err != nil {
		return staged, err
	}
	staged.Manifest.SchemaVersion = actual.SchemaVersion
	staged.Manifest.Attachments = actual.Attachments
	staged.Manifest.FirstDate = actual.FirstDate
	staged.Manifest.LastDate = actual.LastDate
	ok = true
	return staged, nil
}
func (s *Store) Activate(ctx context.Context, stageID string, actors ...string) (Manifest, error) {
	var m Manifest
	if stageID == "" || strings.ContainsAny(stageID, "/\\.") {
		return m, fmt.Errorf("invalid staging id")
	}
	dir := filepath.Join(s.Root, "staging", stageID)
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	for _, f := range m.Files {
		if !filepath.IsLocal(f.Path) {
			return m, fmt.Errorf("invalid staged path")
		}
		h, n, e := checksumFile(filepath.Join(dir, f.Path))
		if e != nil || h != f.SHA256 || n != f.Size {
			return m, fmt.Errorf("staging checksum mismatch")
		}
	}
	actor := ""
	if len(actors) > 0 {
		actor = actors[0]
	}
	actorName := ""
	if len(actors) > 2 {
		actorName = actors[2]
	}
	backup, _, err := s.BackupWithOptions(ctx, BackupOptions{Kind: "before_restore", ActorID: actor, ActorName: actorName, Note: "Bản sao trước khi khôi phục dữ liệu"})
	if err != nil {
		return m, fmt.Errorf("pre-restore backup failed: %w", err)
	}
	next := ID()
	newDir := filepath.Join(s.Root, "datasets", next)
	if err = os.MkdirAll(filepath.Join(newDir, "attachments"), 0700); err != nil {
		return m, err
	}
	for _, f := range m.Files {
		content, e := os.ReadFile(filepath.Join(dir, f.Path))
		if e != nil {
			return m, e
		}
		if e = WriteFile(filepath.Join(newDir, f.Path), content); e != nil {
			return m, e
		}
	}
	db, err := Connect(ctx, filepath.Join(newDir, "accounting.sqlite"), false)
	if err != nil {
		return m, err
	}
	if err = migrate(ctx, db); err != nil {
		db.Close()
		return m, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		db.Close()
		return m, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE meta SET epoch=?,revision=revision+1 WHERE id=1", ID()); err == nil {
		_, err = tx.ExecContext(ctx, "DELETE FROM sessions")
	}
	if err == nil {
		err = tx.Commit()
	} else {
		tx.Rollback()
	}
	if err == nil {
		err = Check(ctx, db)
	}
	closeErr := db.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return m, err
	}
	previous, err := os.ReadFile(filepath.Join(s.Root, "active-dataset.json"))
	if err != nil {
		return m, err
	}
	event := map[string]string{"phase": "prepared", "actorId": actor, "stageId": stageID, "previousDataset": filepath.Base(s.Dataset), "nextDataset": next, "preRestoreBackup": filepath.Base(backup), "startedAt": Now(), "snapshotEpoch": m.Epoch, "companyId": m.CompanyID, "companyName": m.CompanyName, "snapshotCreatedAt": m.CreatedAt}
	if len(actors) > 1 {
		event["reason"] = actors[1]
	}
	if len(actors) > 2 {
		event["actorName"] = actors[2]
	}
	journal := filepath.Join(s.Root, "recovery", ID()+".json")
	if err = os.MkdirAll(filepath.Dir(journal), 0700); err != nil {
		return m, err
	}
	data, _ := json.Marshal(event)
	if err = WriteFile(journal, data); err != nil {
		return m, err
	}
	if err = ctx.Err(); err != nil {
		event["phase"] = "cancelled"
		event["finishedAt"] = Now()
		data, _ = json.Marshal(event)
		if e := WriteFile(journal, data); e != nil {
			slog.Error("cannot finalize cancelled restore journal")
		}
		return m, err
	}
	// Once connections close, finish the pointer switch even if the HTTP client disconnects.
	activationCtx, cancelActivation := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancelActivation()
	if err = s.Read.Close(); err != nil {
		return m, err
	}
	if err = s.Write.Close(); err != nil {
		s.openActive(activationCtx)
		return m, err
	}
	payload, _ := json.Marshal(pointer{Dataset: next})
	if err = WriteFile(filepath.Join(s.Root, "active-dataset.json"), payload); err == nil {
		err = s.openActive(activationCtx)
	}
	if err != nil {
		rollbackErr := WriteFile(filepath.Join(s.Root, "active-dataset.json"), previous)
		if rollbackErr == nil {
			rollbackCtx, cancelRollback := context.WithTimeout(context.Background(), 30*time.Second)
			rollbackErr = s.openActive(rollbackCtx)
			cancelRollback()
		}
		if rollbackErr != nil {
			return m, fmt.Errorf("restore failed and rollback requires maintenance: %w", rollbackErr)
		}
		event["phase"] = "rolled_back"
		event["finishedAt"] = Now()
		data, _ = json.Marshal(event)
		if e := WriteFile(journal, data); e != nil {
			slog.Error("cannot finalize restore journal")
		}
		return m, err
	}
	event["phase"] = "activated"
	event["finishedAt"] = Now()
	data, _ = json.Marshal(event)
	if e := WriteFile(journal, data); e != nil {
		slog.Error("restore activated; prepared journal remains available")
	}
	return m, nil
}
