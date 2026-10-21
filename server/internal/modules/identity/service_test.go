package identity

import (
	"context"
	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
	"testing"
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
