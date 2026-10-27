package operations

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestRecoveryCatalogReplayVerificationAndReopen(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	doc := f.save(t, "backup-source")
	if _, err := f.ledger.Post(ctx, f.cmd("post"), ledger.Post{ID: doc.ID, Version: doc.Version}); err != nil {
		t.Fatal(err)
	}
	content := []byte("Tệp chứng minh giả lập")
	hash := sqlite.Hash(content)
	if err := sqlite.WriteFile(filepath.Join(f.store.Dataset, "attachments", hash), content); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Write.Exec("INSERT INTO attachments VALUES('evidence',?,?,?,?)", doc.ID, hash, "evidence.txt", len(content)); err != nil {
		t.Fatal(err)
	}
	current, err := f.store.RecoveryCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	options := sqlite.BackupOptions{ID: "managed-backup", Kind: "manual", ActorName: "Chủ sở hữu", Note: "Trước chốt sổ", RequestHash: "same-request"}
	path, m, err := f.store.BackupWithOptions(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if m.Debit != "9007199254740993" || (m.Attachments == nil || *m.Attachments != 1) || m.SchemaVersion != 9 || m.FirstDate != "2026-10-21" {
		t.Fatal(m)
	}
	again, _, err := f.store.BackupWithOptions(ctx, options)
	if err != nil || again != path {
		t.Fatal(again, err)
	}
	options.RequestHash = "different-request"
	if _, _, err = f.store.BackupWithOptions(ctx, options); accounting.Code(err) != "IDEMPOTENCY_CONFLICT" {
		t.Fatal(err)
	}
	list, unreadable, err := f.store.ListBackups(ctx, current.CompanyID)
	if err != nil || unreadable != 0 || len(list) != 1 || list[0].SHA256 == "" || list[0].VerifiedAt == "" {
		t.Fatal(list, unreadable, err)
	}
	if list, _, err = f.store.ListBackups(ctx, "other-company"); err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
	if _, _, err = f.store.ReadBackup(ctx, "managed-backup", "other-company", false); accounting.Code(err) != "NOT_FOUND" {
		t.Fatal(err)
	}
	if _, _, err = f.store.ReadBackup(ctx, "../../file", current.CompanyID, false); accounting.Code(err) != "NOT_FOUND" {
		t.Fatal(err)
	}
	for _, id := range []string{"a/../../file", "a\\..\\file", "a.b", "a:b"} {
		if _, _, err = f.store.ReadBackup(ctx, id, current.CompanyID, false); accounting.Code(err) != "NOT_FOUND" {
			t.Fatal(id, err)
		}
		if _, err = f.store.CheckRestoreApproval(ctx, id, f.actor); accounting.Code(err) != "PREVIEW_REQUIRED" {
			t.Fatal(id, err)
		}
	}

	if err = os.Remove(path + ".json"); err != nil {
		t.Fatal(err)
	}
	list, _, err = f.store.ListBackups(ctx, current.CompanyID)
	if err != nil || list[0].VerifiedAt != "" {
		t.Fatal(list, err)
	}
	if _, info, err := f.store.ReadBackup(ctx, "managed-backup", current.CompanyID, true); err != nil || info.VerifiedAt == "" {
		t.Fatal(info, err)
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(ctx, f.root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	list, _, err = reopened.ListBackups(ctx, current.CompanyID)
	if err != nil || len(list) != 1 || list[0].Manifest.Debit != "9007199254740993" {
		t.Fatal(list, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 1
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = reopened.ReadBackup(ctx, "managed-backup", current.CompanyID, false); accounting.Code(err) != "INVALID_BACKUP" {
		t.Fatal(err)
	}
	if err = sqlite.Check(ctx, reopened.Read); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryPreviewRevisionExpiryAndSafetyBackup(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	f.save(t, "before")
	path, _, err := f.store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareRestore(ctx, path, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CheckRestoreApproval(ctx, preview.ID, "other-owner"); accounting.Code(err) != "PREVIEW_REQUIRED" {
		t.Fatal(err)
	}
	f.save(t, "after-preview")
	if _, err = f.store.CheckRestoreApproval(ctx, preview.ID, f.actor); accounting.Code(err) != "VERSION_CONFLICT" {
		t.Fatal(err)
	}
	preview, err = f.store.PrepareRestore(ctx, path, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	approvalPath := filepath.Join(f.root, "staging", preview.ID, "approval.json")
	raw, err := os.ReadFile(approvalPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err = json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata["expiresAt"] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	expired, _ := json.Marshal(metadata)
	if err = sqlite.WriteFile(approvalPath, expired); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CheckRestoreApproval(ctx, preview.ID, f.actor); accounting.Code(err) != "PREVIEW_REQUIRED" {
		t.Fatal(err)
	}
	if err = sqlite.WriteFile(approvalPath, raw); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CheckRestoreApproval(ctx, preview.ID, f.actor); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Activate(ctx, preview.ID, f.actor, "Đối chiếu gói kiểm thử", "Người kiểm thử"); err != nil {
		t.Fatal(err)
	}
	current, err := f.store.RecoveryCurrent(ctx)
	if err != nil || current.Documents != 1 || current.Epoch == f.epoch {
		t.Fatal(current, err)
	}
	list, _, err := f.store.ListBackups(ctx, current.CompanyID)
	if err != nil || len(list) != 2 {
		t.Fatal(list, err)
	}
	var safety sqlite.BackupInfo
	for _, row := range list {
		if row.Kind == "before_restore" {
			safety = row
		}
	}
	if safety.ID == "" || safety.Manifest.Documents != 2 || safety.ActorName != "Người kiểm thử" {
		t.Fatal(safety)
	}
	events := f.store.RecoveryEvents(current.CompanyID)
	if len(events) != 1 || events[0].Phase != "activated" || events[0].Reason != "Đối chiếu gói kiểm thử" || events[0].ActorName != "Người kiểm thử" {
		t.Fatal(events)
	}
	// The retained safety package can restore the state that existed before activation.
	safetyPath, _, err := f.store.ReadBackup(ctx, safety.ID, current.CompanyID, true)
	if err != nil {
		t.Fatal(err)
	}
	undo, err := f.store.PrepareRestore(ctx, safetyPath, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CheckRestoreApproval(ctx, undo.ID, f.actor); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Activate(ctx, undo.ID, f.actor, "Quay lại trước khôi phục"); err != nil {
		t.Fatal(err)
	}
	current, err = f.store.RecoveryCurrent(ctx)
	if err != nil || current.Documents != 2 {
		t.Fatal(current, err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryRejectsForgedCompanyAndCancelledPreparation(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	path, _, err := f.store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	original, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	forged := filepath.Join(t.TempDir(), "forged.zip")
	out, err := os.Create(forged)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(out)
	for _, entry := range original.File {
		r, e := entry.Open()
		if e != nil {
			t.Fatal(e)
		}
		w, e := writer.Create(entry.Name)
		if e != nil {
			t.Fatal(e)
		}
		if entry.Name == "manifest.json" {
			var m sqlite.Manifest
			if e = json.NewDecoder(r).Decode(&m); e != nil {
				t.Fatal(e)
			}
			m.CompanyName = "Tên công ty giả mạo"
			if e = json.NewEncoder(w).Encode(m); e != nil {
				t.Fatal(e)
			}
		} else {
			if _, e = io.Copy(w, r); e != nil {
				t.Fatal(e)
			}
		}
		r.Close()
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.PrepareRestore(ctx, forged, f.actor); accounting.Code(err) != "INVALID_BACKUP" {
		t.Fatal(err)
	}
	staged, err := f.store.PrepareRestore(ctx, path, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = f.store.Activate(cancelled, staged.ID); err == nil {
		t.Fatal("accepted canceled activation")
	}
	current, err := f.store.RecoveryCurrent(ctx)
	if err != nil || current.Epoch != f.epoch {
		t.Fatal(current, err)
	}
}

func TestRecoveryDailyScheduleDurabilityCatchupAndConcurrentBackup(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	current, err := f.store.RecoveryCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config, err := f.store.ReadBackupSchedule(current.CompanyID)
	if err != nil || config.Enabled {
		t.Fatal(config, err)
	}
	config, err = f.store.SaveBackupSchedule(current.CompanyID, "Người kiểm thử", true, config.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SaveBackupSchedule(current.CompanyID, "tab-cũ", false, 0); accounting.Code(err) != "VERSION_CONFLICT" {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(48 * time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.store.RunScheduledBackup(ctx, now) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	list, _, err := f.store.ListBackups(ctx, current.CompanyID)
	if err != nil || len(list) != 1 || list[0].Kind != "scheduled" {
		t.Fatal(list, err)
	}
	due, err := time.Parse(time.RFC3339, list[0].ScheduledFor)
	if err != nil || due.Hour() != 16 || due.Minute() != 0 {
		t.Fatal(due, err)
	}
	job, err := f.store.ReadBackupJob(current, now)
	if err != nil || job.Status != "completed" || job.Attempts != 1 {
		t.Fatal(job, err)
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(ctx, f.root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	config, err = reopened.ReadBackupSchedule(current.CompanyID)
	if err != nil || !config.Enabled || config.Version != 1 {
		t.Fatal(config, err)
	}
	if err = reopened.RunScheduledBackup(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err = reopened.RunScheduledBackup(ctx, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	list, _, err = reopened.ListBackups(ctx, current.CompanyID)
	if err != nil || len(list) != 2 {
		t.Fatal(list, err)
	}
	if _, err = reopened.SaveBackupSchedule(current.CompanyID, "Người kiểm thử", false, config.Version); err != nil {
		t.Fatal(err)
	}
	if err = reopened.RunScheduledBackup(ctx, now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	list, _, err = reopened.ListBackups(ctx, current.CompanyID)
	if err != nil || len(list) != 2 {
		t.Fatal(list, err)
	}
}

func TestRecoveryScheduledRetryLimitsAndInterruptedCompletion(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	doc := f.save(t, "scheduled-attachment")
	content := []byte("Bằng chứng cho tác vụ tự động")
	hash := sqlite.Hash(content)
	if _, err := f.store.Write.Exec("INSERT INTO attachments VALUES('scheduled',?,?,?,?)", doc.ID, hash, "scheduled.txt", len(content)); err != nil {
		t.Fatal(err)
	}
	current, err := f.store.RecoveryCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SaveBackupSchedule(current.CompanyID, "Chủ sở hữu", true, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(48 * time.Hour)
	for i := 0; i < 4; i++ {
		err = f.store.RunScheduledBackup(ctx, now.Add(time.Duration(i)*6*time.Minute))
		if i < 3 && err == nil {
			t.Fatal("accepted missing attachment")
		}
	}
	job, err := f.store.ReadBackupJob(current, now.Add(18*time.Minute))
	if err != nil || job.Status != "failed" || job.Attempts != 3 {
		t.Fatal(job, err)
	}
	if err = sqlite.WriteFile(filepath.Join(f.store.Dataset, "attachments", hash), content); err != nil {
		t.Fatal(err)
	}
	tomorrow := now.Add(24 * time.Hour)
	if err = f.store.RunScheduledBackup(ctx, tomorrow); err != nil {
		t.Fatal(err)
	}
	list, _, err := f.store.ListBackups(ctx, current.CompanyID)
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	job, err = f.store.ReadBackupJob(current, tomorrow)
	if err != nil {
		t.Fatal(err)
	}
	job.Status = "running"
	job.Attempts = 3
	job.BackupID = ""
	job.FinishedAt = ""
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if err = sqlite.WriteFile(filepath.Join(f.root, "backup-jobs", list[0].ID+".json"), raw); err != nil {
		t.Fatal(err)
	}
	if err = f.store.RunScheduledBackup(ctx, tomorrow.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	job, err = f.store.ReadBackupJob(current, tomorrow.Add(10*time.Minute))
	if err != nil || job.Status != "completed" || job.BackupID != list[0].ID || job.Attempts != 3 {
		t.Fatal(job, err)
	}
	list, _, err = f.store.ListBackups(ctx, current.CompanyID)
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
}

func TestRecoveryPreviewCleanupLeavesActiveAndBackups(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	path, _, err := f.store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareRestore(ctx, path, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.CleanupRestorePreviews(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CheckRestoreApproval(ctx, preview.ID, f.actor); err != nil {
		t.Fatal(err)
	}
	if err = f.store.CleanupRestorePreviews(time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CheckRestoreApproval(ctx, preview.ID, f.actor); accounting.Code(err) != "PREVIEW_REQUIRED" {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("removed backup", err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverySnapshotDuringConcurrentPosting(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	start := make(chan struct{})
	errs := make(chan error, 2)
	var path string
	var manifest sqlite.Manifest
	go func() { <-start; var err error; path, manifest, err = f.store.Backup(ctx); errs <- err }()
	go func() {
		<-start
		for i := 0; i < 10; i++ {
			id := "during-backup-" + strconv.Itoa(i)
			doc := f.draft(id)
			if _, err := f.ledger.Save(ctx, f.cmd("save-"+id), doc); err != nil {
				errs <- err
				return
			}
			if _, err := f.ledger.Post(ctx, f.cmd("post-"+id), ledger.Post{ID: id, Version: 1}); err != nil {
				errs <- err
				return
			}
		}
		errs <- nil
	}()
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if manifest.Posted < 0 || manifest.Posted > 10 || manifest.Documents < manifest.Posted {
		t.Fatal(manifest)
	}
	expected := new(big.Int).Mul(big.NewInt(int64(manifest.Posted)), big.NewInt(9007199254740993)).String()
	if manifest.Debit != expected || manifest.Credit != expected {
		t.Fatal(manifest, expected)
	}
	if _, err := f.store.Stage(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryLatePreparationFailureLeavesActiveDatasetWritable(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	path, _, err := f.store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareRestore(ctx, path, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(f.root, "staging", preview.ID)
	database := filepath.Join(dir, "accounting.sqlite")
	staged, err := sqlite.Connect(ctx, database, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = staged.Exec("CREATE TRIGGER force_activation_failure BEFORE UPDATE OF epoch ON meta BEGIN SELECT RAISE(ABORT,'forced activation failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err = staged.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest sqlite.Manifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	dbBytes, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}
	for i := range manifest.Files {
		if manifest.Files[i].Path == "accounting.sqlite" {
			manifest.Files[i].SHA256 = sqlite.Hash(dbBytes)
			manifest.Files[i].Size = int64(len(dbBytes))
		}
	}
	raw, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = sqlite.WriteFile(filepath.Join(dir, "manifest.json"), raw); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Activate(ctx, preview.ID, f.actor); err == nil {
		t.Fatal("accepted failing activation")
	}
	current, err := f.store.RecoveryCurrent(ctx)
	if err != nil || current.Epoch != f.epoch {
		t.Fatal(current, err)
	}
	if _, err = f.ledger.Save(ctx, f.cmd("after-failure"), f.draft("after-failure")); err != nil {
		t.Fatal("active dataset no longer writable", err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}
