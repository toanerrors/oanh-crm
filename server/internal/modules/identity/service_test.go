package identity

import (
	"context"
	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
	"sync"
	"testing"
	"time"
)

func TestLoginLockExpiresWithoutImmediatelyRelocking(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Service{Store: db}
	setup := Setup{Username: "owner", Password: "Synthetic-test-2026", Name: "Chủ thử", CompanyName: "Thử", Regime: "Kiểm thử", ConfirmAccounts: true}
	if err = s.Setup(ctx, setup); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, _, err = s.Login(ctx, setup.Username, "wrong"); accounting.Code(err) != "UNAUTHENTICATED" {
			t.Fatal(i, err)
		}
	}
	if _, _, err = s.Login(ctx, setup.Username, setup.Password); accounting.Code(err) != "RATE_LIMITED" {
		t.Fatal(err)
	}
	if _, err = db.Write.Exec("UPDATE login_limits SET blocked_until='2000-01-01T00:00:00Z'"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Login(ctx, setup.Username, "wrong"); accounting.Code(err) != "UNAUTHENTICATED" {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, setup.Username, setup.Password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err = s.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, token); accounting.Code(err) != "UNAUTHENTICATED" {
		t.Fatal(err)
	}
}

func TestSessionLifetimeRememberedAndLegacyAfterReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, err := sqlite.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	now := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	start := now
	s := Service{Store: db, now: func() time.Time { return now }}
	setup := Setup{Username: "remember-owner", Password: "Synthetic-session-2026", Name: "Chủ thử", CompanyName: "Thử", Regime: "Kiểm thử", ConfirmAccounts: true}
	if err = s.Setup(ctx, setup); err != nil {
		t.Fatal(err)
	}
	_, legacy, err := s.Login(ctx, setup.Username, setup.Password)
	if err != nil {
		t.Fatal(err)
	}
	_, short, err := s.LoginWithRemember(ctx, setup.Username, setup.Password, false)
	if err != nil {
		t.Fatal(err)
	}
	_, remembered, err := s.LoginWithRemember(ctx, setup.Username, setup.Password, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		token    string
		lifetime time.Duration
	}{{legacy, 12 * time.Hour}, {short, 12 * time.Hour}, {remembered, 30 * 24 * time.Hour}} {
		var hash, expires string
		if err = db.Read.QueryRowContext(ctx, "SELECT token_hash,expires_at FROM sessions WHERE token_hash=?", sqlite.Hash([]byte(test.token))).Scan(&hash, &expires); err != nil {
			t.Fatal(err)
		}
		if hash == test.token || expires != start.Add(test.lifetime).Format(time.RFC3339) {
			t.Fatal("session hash/expiry mismatch")
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	s.Store = db
	now = start.Add(24 * time.Hour)
	for _, token := range []string{legacy, short} {
		if _, err = s.Authenticate(ctx, token); accounting.Code(err) != "UNAUTHENTICATED" {
			t.Fatal(err)
		}
	}
	if _, err = s.Authenticate(ctx, remembered); err != nil {
		t.Fatal(err)
	}
	var expires string
	if err = db.Read.QueryRowContext(ctx, "SELECT expires_at FROM sessions WHERE token_hash=?", sqlite.Hash([]byte(remembered))).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	if expires != start.Add(30*24*time.Hour).Format(time.RFC3339) {
		t.Fatal("read extended absolute expiry")
	}
	now = start.Add(30*24*time.Hour - time.Second)
	if _, err = s.Authenticate(ctx, remembered); err != nil {
		t.Fatal(err)
	}
	now = start.Add(30 * 24 * time.Hour)
	if _, err = s.Authenticate(ctx, remembered); accounting.Code(err) != "UNAUTHENTICATED" {
		t.Fatal(err)
	}
	now = start.Add(24 * time.Hour)
	if err = s.Logout(ctx, remembered); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, remembered); accounting.Code(err) != "UNAUTHENTICATED" {
		t.Fatal(err)
	}
}

func TestRememberedSessionsRollbackAndConcurrentLogout(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Service{Store: db}
	setup := Setup{Username: "rollback-owner", Password: "Synthetic-session-2026", Name: "Chủ thử", CompanyName: "Thử", Regime: "Kiểm thử", ConfirmAccounts: true}
	if err = s.Setup(ctx, setup); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Write.ExecContext(ctx, "CREATE TRIGGER reject_session BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT,'synthetic failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, token, e := s.LoginWithRemember(ctx, setup.Username, setup.Password, true); e == nil || token != "" {
		t.Fatal("failed insert issued a token")
	}
	var count int
	if err = db.Read.QueryRowContext(ctx, "SELECT count(*) FROM sessions").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if _, err = db.Write.ExecContext(ctx, "DROP TRIGGER reject_session"); err != nil {
		t.Fatal(err)
	}
	_, token, err := s.LoginWithRemember(ctx, setup.Username, setup.Password, true)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 6; i++ {
		workers.Go(func() {
			for n := 0; n < 8; n++ {
				_, e := s.Authenticate(ctx, token)
				if e != nil && accounting.Code(e) != "UNAUTHENTICATED" {
					t.Error(e)
				}
			}
		})
	}
	if err = s.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	if _, err = s.Authenticate(ctx, token); accounting.Code(err) != "UNAUTHENTICATED" {
		t.Fatal(err)
	}
}
