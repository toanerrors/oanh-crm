package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"oanh.local/accounting/internal/accounting"
)

type ResetOptions struct {
	ActorID      string
	ActorName    string
	Reason       string
	KeepPolicies bool
	KeepSessions bool
}

type preservedUser struct {
	id, username, name, role, passwordHash string
	active                                 int
}

type preservedSession struct {
	tokenHash, userID, expiresAt string
}

type preservedPolicy struct {
	id            string
	version       int
	effectiveFrom string
	data          string
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func (s *Store) ResetKeepingUsers(ctx context.Context, options ResetOptions) (Manifest, error) {
	var m Manifest
	var companyID, companyName, currency, regime string
	if err := s.Read.QueryRowContext(ctx, "SELECT company_id,company_name,currency,regime FROM meta WHERE id=1").Scan(&companyID, &companyName, &currency, &regime); err != nil {
		return m, err
	}
	userRows, err := s.Read.QueryContext(ctx, "SELECT id,username,name,role,password_hash,active FROM users ORDER BY rowid")
	if err != nil {
		return m, err
	}
	users := []preservedUser{}
	for userRows.Next() {
		var u preservedUser
		if err = userRows.Scan(&u.id, &u.username, &u.name, &u.role, &u.passwordHash, &u.active); err != nil {
			userRows.Close()
			return m, err
		}
		users = append(users, u)
	}
	if err = userRows.Err(); err != nil {
		userRows.Close()
		return m, err
	}
	userRows.Close()
	if len(users) == 0 {
		return m, accounting.Fail("VALIDATION", "Không gian chưa khởi tạo người dùng.")
	}

	actorID := strings.TrimSpace(options.ActorID)
	actorName := strings.TrimSpace(options.ActorName)
	hasActiveOwner := false
	actorFound := false
	for _, u := range users {
		if u.role == "owner" && u.active == 1 {
			hasActiveOwner = true
			if actorID == "" {
				actorID = u.id
				actorName = u.name
				actorFound = true
			}
		}
		if actorID != "" && u.id == actorID {
			if u.role != "owner" || u.active != 1 {
				return m, accounting.Fail("FORBIDDEN", "Chỉ chủ sở hữu đang hoạt động được đặt lại dữ liệu.")
			}
			if actorName == "" {
				actorName = u.name
			}
			actorFound = true
		}
	}
	if !hasActiveOwner || !actorFound {
		return m, accounting.Fail("VALIDATION", "Cần ít nhất một tài khoản chủ sở hữu đang hoạt động.")
	}

	policies := []preservedPolicy{}
	if options.KeepPolicies {
		policyRows, e := s.Read.QueryContext(ctx, "SELECT id,version,effective_from,data FROM policies ORDER BY version")
		if e != nil {
			return m, e
		}
		for policyRows.Next() {
			var p preservedPolicy
			if e = policyRows.Scan(&p.id, &p.version, &p.effectiveFrom, &p.data); e != nil {
				policyRows.Close()
				return m, e
			}
			policies = append(policies, p)
		}
		if e = policyRows.Err(); e != nil {
			policyRows.Close()
			return m, e
		}
		policyRows.Close()
	}

	sessions := []preservedSession{}
	if options.KeepSessions {
		sessionRows, e := s.Read.QueryContext(ctx, "SELECT s.token_hash,s.user_id,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE u.active=1 AND s.expires_at>? ORDER BY s.token_hash", Now())
		if e != nil {
			return m, e
		}
		for sessionRows.Next() {
			var sess preservedSession
			if e = sessionRows.Scan(&sess.tokenHash, &sess.userID, &sess.expiresAt); e != nil {
				sessionRows.Close()
				return m, e
			}
			sessions = append(sessions, sess)
		}
		if e = sessionRows.Err(); e != nil {
			sessionRows.Close()
			return m, e
		}
		sessionRows.Close()
	}

	reason := strings.TrimSpace(options.Reason)
	note := "Bản sao tự động trước khi đặt lại dữ liệu ban đầu"
	if reason != "" {
		note = truncateRunes(note+": "+reason, 300)
	}
	backup, _, err := s.BackupWithOptions(ctx, BackupOptions{
		Kind:      "before_restore",
		ActorID:   actorID,
		ActorName: actorName,
		Note:      note,
	})
	if err != nil {
		return m, fmt.Errorf("pre-reset backup failed: %w", err)
	}

	next := ID()
	newDir := filepath.Join(s.Root, "datasets", next)
	if err = os.MkdirAll(filepath.Join(newDir, "attachments"), 0700); err != nil {
		return m, err
	}
	prepared := false
	defer func() {
		if !prepared {
			os.RemoveAll(newDir)
		}
	}()

	dbPath := filepath.Join(newDir, "accounting.sqlite")
	db, err := Connect(ctx, dbPath, false)
	if err != nil {
		return m, err
	}
	if _, err = db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
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
	if err = SeedInitialAccounts(ctx, tx); err == nil {
		for _, u := range users {
			if _, err = tx.ExecContext(ctx, "INSERT INTO users VALUES(?,?,?,?,?,?)", u.id, u.username, u.name, u.role, u.passwordHash, u.active); err != nil {
				break
			}
		}
	}
	if err == nil {
		for _, sess := range sessions {
			if _, err = tx.ExecContext(ctx, "INSERT INTO sessions VALUES(?,?,?)", sess.tokenHash, sess.userID, sess.expiresAt); err != nil {
				break
			}
		}
	}
	if err == nil {
		for _, p := range policies {
			if _, err = tx.ExecContext(ctx, "INSERT INTO policies VALUES(?,?,?,?)", p.id, p.version, p.effectiveFrom, p.data); err != nil {
				break
			}
		}
	}
	newEpoch := ID()
	if err == nil {
		_, err = tx.ExecContext(ctx, "UPDATE meta SET company_id=?,company_name=?,epoch=?,revision=revision+1,currency=?,regime=? WHERE id=1", companyID, companyName, newEpoch, currency, regime)
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, "INSERT INTO audit(actor_id,action,entity_id,occurred_at,revision) SELECT ?,'setup',company_id,?,revision FROM meta", actorID, Now())
	}
	if err == nil {
		err = tx.Commit()
	} else {
		tx.Rollback()
	}
	if err == nil {
		m, err = reconcile(ctx, db)
	}
	closeErr := db.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return m, err
	}
	m.CreatedAt = Now()

	previous, err := os.ReadFile(filepath.Join(s.Root, "active-dataset.json"))
	if err != nil {
		return m, err
	}
	eventReason := "Đặt lại dữ liệu ban đầu (giữ người dùng)"
	if reason != "" {
		eventReason = truncateRunes(eventReason+": "+reason, 500)
	}
	event := map[string]string{
		"phase":             "prepared",
		"actorId":           actorID,
		"actorName":         actorName,
		"reason":            eventReason,
		"previousDataset":   filepath.Base(s.Dataset),
		"nextDataset":       next,
		"preRestoreBackup":  filepath.Base(backup),
		"startedAt":         Now(),
		"snapshotEpoch":     m.Epoch,
		"companyId":         m.CompanyID,
		"companyName":       m.CompanyName,
		"snapshotCreatedAt": m.CreatedAt,
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
			slog.Error("cannot finalize cancelled reset journal")
		}
		return m, err
	}

	prepared = true
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
			return m, fmt.Errorf("reset failed and rollback requires maintenance: %w", rollbackErr)
		}
		event["phase"] = "rolled_back"
		event["finishedAt"] = Now()
		data, _ = json.Marshal(event)
		if e := WriteFile(journal, data); e != nil {
			slog.Error("cannot finalize reset journal")
		}
		return m, err
	}
	event["phase"] = "activated"
	event["finishedAt"] = Now()
	data, _ = json.Marshal(event)
	if e := WriteFile(journal, data); e != nil {
		slog.Error("reset activated; prepared journal remains available")
	}
	return m, nil
}
