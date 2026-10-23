package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
	_ "time/tzdata"

	"oanh.local/accounting/internal/accounting"
)

type BackupSchedule struct {
	Enabled   bool   `json:"enabled"`
	Version   int64  `json:"version"`
	Time      string `json:"time"`
	TimeZone  string `json:"timeZone"`
	Retention string `json:"retention"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	UpdatedBy string `json:"updatedBy,omitempty"`
}
type BackupJob struct {
	Status       string `json:"status"`
	ScheduledFor string `json:"scheduledFor"`
	StartedAt    string `json:"startedAt"`
	FinishedAt   string `json:"finishedAt,omitempty"`
	Attempts     int    `json:"attempts"`
	BackupID     string `json:"backupId,omitempty"`
	Message      string `json:"message,omitempty"`
}

func backupLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		panic(err)
	}
	return location
}
func backupDue(now time.Time) time.Time {
	local := now.In(backupLocation())
	due := time.Date(local.Year(), local.Month(), local.Day(), 23, 0, 0, 0, local.Location())
	if local.Before(due) {
		due = due.AddDate(0, 0, -1)
	}
	return due
}
func (s *Store) schedulePath(company string) string {
	return filepath.Join(s.Root, "backup-settings", Hash([]byte(company))+".json")
}
func (s *Store) ReadBackupSchedule(company string) (BackupSchedule, error) {
	result := BackupSchedule{Time: "23:00", TimeZone: "Asia/Ho_Chi_Minh", Retention: "all"}
	raw, err := os.ReadFile(s.schedulePath(company))
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	if result.Time != "23:00" || result.TimeZone != "Asia/Ho_Chi_Minh" || result.Retention != "all" || result.Version < 0 {
		return result, fmt.Errorf("invalid backup schedule")
	}
	return result, nil
}
func (s *Store) SaveBackupSchedule(company, actor string, enabled bool, version int64) (BackupSchedule, error) {
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	current, err := s.ReadBackupSchedule(company)
	if err != nil {
		return current, err
	}
	if version != current.Version {
		return current, accounting.Fail("VERSION_CONFLICT", "Lịch sao lưu đã thay đổi. Tải lại trước khi lưu.")
	}
	current.Enabled = enabled
	current.Version++
	current.UpdatedAt = Now()
	current.UpdatedBy = actor
	if err = os.MkdirAll(filepath.Dir(s.schedulePath(company)), 0700); err != nil {
		return current, err
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return current, err
	}
	return current, WriteFile(s.schedulePath(company), raw)
}
func scheduledBackupID(current RecoveryCurrent, due time.Time) string {
	return Hash([]byte("daily:" + current.CompanyID + ":" + current.Epoch + ":" + due.Format(time.RFC3339)))
}
func (s *Store) backupJobPath(id string) string {
	return filepath.Join(s.Root, "backup-jobs", id+".json")
}
func (s *Store) ReadBackupJob(current RecoveryCurrent, now time.Time) (BackupJob, error) {
	var result BackupJob
	raw, err := os.ReadFile(s.backupJobPath(scheduledBackupID(current, backupDue(now))))
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(raw, &result)
	return result, err
}
func (s *Store) writeBackupJob(id string, job BackupJob) error {
	if err := os.MkdirAll(filepath.Join(s.Root, "backup-jobs"), 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return WriteFile(s.backupJobPath(id), raw)
}
func (s *Store) RunScheduledBackup(ctx context.Context, now time.Time) error {
	s.Gate.RLock()
	defer s.Gate.RUnlock()
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	current, err := s.RecoveryCurrent(ctx)
	if err != nil {
		return err
	}
	if current.CompanyID == "" {
		return nil
	}
	schedule, err := s.ReadBackupSchedule(current.CompanyID)
	if err != nil || !schedule.Enabled {
		return err
	}
	due := backupDue(now)
	// Enabling a schedule does not retroactively create jobs from before that decision.
	enabledAt, err := time.Parse(time.RFC3339Nano, schedule.UpdatedAt)
	if err != nil {
		return err
	}
	if due.Before(enabledAt) {
		return nil
	}
	job, err := s.ReadBackupJob(current, now)
	if err != nil {
		return err
	}
	if job.Status == "completed" {
		return nil
	}
	id := scheduledBackupID(current, due)
	path, _ := backupPath(s.Root, id)
	if _, e := os.Stat(path); e == nil {
		_, info, e := s.ReadBackup(ctx, id, current.CompanyID, true)
		if e != nil {
			return e
		}
		if info.RequestHash != id {
			return fmt.Errorf("scheduled backup intent mismatch")
		}
		job.Status = "completed"
		job.BackupID = id
		job.ScheduledFor = due.UTC().Format(time.RFC3339)
		job.FinishedAt = Now()
		job.Message = ""
		return s.writeBackupJob(id, job)
	} else if !os.IsNotExist(e) {
		return e
	}
	if job.Attempts >= 3 {
		if job.Status == "running" {
			job.Status = "failed"
			job.Message = "Tác vụ trước bị gián đoạn. Hãy tạo bản sao thủ công và kiểm tra máy chủ."
			return s.writeBackupJob(id, job)
		}
		return nil
	}
	if job.StartedAt != "" {
		started, e := time.Parse(time.RFC3339Nano, job.StartedAt)
		if e != nil {
			return e
		}
		if now.Before(started.Add(5 * time.Minute)) {
			return nil
		}
	}
	job = BackupJob{Status: "running", ScheduledFor: due.UTC().Format(time.RFC3339), StartedAt: now.UTC().Format(time.RFC3339Nano), Attempts: job.Attempts + 1}
	if err = s.writeBackupJob(id, job); err != nil {
		return err
	}
	timeout, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	_, _, err = s.BackupWithOptions(timeout, BackupOptions{ID: id, Kind: "scheduled", ActorName: "Lịch sao lưu tự động", RequestHash: id, ScheduledFor: job.ScheduledFor})
	job.FinishedAt = Now()
	if err != nil {
		job.Status = "failed"
		job.Message = "Không tạo được bản sao. Kiểm tra dung lượng máy chủ hoặc tạo sao lưu thủ công."
	} else {
		job.Status = "completed"
		job.BackupID = id
	}
	if saveErr := s.writeBackupJob(id, job); saveErr != nil {
		return errors.Join(err, saveErr)
	}
	return err
}
func (s *Store) RunBackupScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	lastError := ""
	for {
		err := s.CleanupRestorePreviews(time.Now())
		if err == nil {
			err = s.RunScheduledBackup(ctx, time.Now())
		}
		if err != nil && ctx.Err() == nil && err.Error() != lastError {
			slog.Error("scheduled backup failed", "error", err)
			lastError = err.Error()
		} else if err == nil {
			lastError = ""
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func NextBackupAt(now time.Time) string {
	return backupDue(now).AddDate(0, 0, 1).UTC().Format(time.RFC3339)
}
