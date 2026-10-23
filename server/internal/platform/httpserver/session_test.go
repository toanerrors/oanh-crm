package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestRememberedSessionCookiesExpiryAndRevocation(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := identity.Service{Store: db}
	setup := identity.Setup{Username: "session-owner", Password: "Synthetic-owner-2026", Name: "Chủ thử", CompanyName: "Thử", Regime: "Kiểm thử", ConfirmAccounts: true}
	if err = service.Setup(ctx, setup); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(New(db, "", true))
	defer server.Close()
	newClient := func() *http.Client {
		c := *server.Client()
		jar, e := cookiejar.New(nil)
		if e != nil {
			t.Fatal(e)
		}
		c.Jar = jar
		return &c
	}
	owner := newClient()
	call := func(client *http.Client, method, path string, input any, headers map[string]string) *http.Response {
		t.Helper()
		var body io.Reader
		if input != nil {
			b, e := json.Marshal(input)
			if e != nil {
				t.Fatal(e)
			}
			body = bytes.NewReader(b)
		}
		req, e := http.NewRequest(method, server.URL+"/api/v1"+path, body)
		if e != nil {
			t.Fatal(e)
		}
		if input != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		_, e = io.Copy(io.Discard, res.Body)
		res.Body.Close()
		if e != nil {
			t.Fatal(e)
		}
		return res
	}
	login := func(client *http.Client, username, password string, remember any) *http.Response {
		t.Helper()
		input := map[string]any{"username": username, "password": password}
		if remember != nil {
			input["remember"] = remember
		}
		return call(client, "POST", "/auth/login", input, nil)
	}
	var revision int64
	if err = db.Read.QueryRowContext(ctx, "SELECT revision FROM meta WHERE id=1").Scan(&revision); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		remember any
		lifetime time.Duration
	}{{"legacy", nil, 12 * time.Hour}, {"short", false, 12 * time.Hour}, {"remembered", true, 30 * 24 * time.Hour}} {
		t.Run(test.name, func(t *testing.T) {
			res := login(owner, setup.Username, setup.Password, test.remember)
			if res.StatusCode != 200 {
				t.Fatal(res.StatusCode)
			}
			cookies := res.Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing session cookie")
			}
			cookie := cookies[0]
			if cookie.Name != "oanh_session" || cookie.Path != "/" || cookie.Domain != "" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != int(test.lifetime/time.Second) {
				t.Fatal("invalid session cookie attributes")
			}
			var expires string
			if err = db.Read.QueryRowContext(ctx, "SELECT expires_at FROM sessions WHERE token_hash=?", sqlite.Hash([]byte(cookie.Value))).Scan(&expires); err != nil {
				t.Fatal(err)
			}
			expiry, e := time.Parse(time.RFC3339, expires)
			if e != nil {
				t.Fatal(e)
			}
			if delta := cookie.Expires.Sub(expiry); delta < 0 || delta > time.Second {
				t.Fatal("cookie/server expiry mismatch")
			}
			if remaining := time.Until(expiry); remaining > test.lifetime || remaining < test.lifetime-5*time.Second {
				t.Fatal("unexpected session duration")
			}
			if res = call(owner, "GET", "/workspace?period=2026-10", nil, nil); res.StatusCode != 200 {
				t.Fatal(res.StatusCode)
			}
			res = call(owner, "POST", "/auth/logout", nil, nil)
			if res.StatusCode != 200 || len(res.Cookies()) != 1 || res.Cookies()[0].MaxAge >= 0 {
				t.Fatal("logout did not clear cookie")
			}
			replay := newClient()
			replay.Jar = nil
			if res = call(replay, "GET", "/workspace?period=2026-10", nil, map[string]string{"Cookie": "oanh_session=" + cookie.Value}); res.StatusCode != 401 {
				t.Fatal("revoked session accepted", res.StatusCode)
			}
		})
	}
	if res := login(owner, setup.Username, setup.Password, "true"); res.StatusCode != 400 {
		t.Fatal("invalid remember accepted", res.StatusCode)
	}
	if res := login(owner, setup.Username, "wrong", true); res.StatusCode != 401 || len(res.Cookies()) != 0 {
		t.Fatal("failed login issued cookie")
	}
	if res := login(owner, setup.Username, setup.Password, true); res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	var current int64
	if err = db.Read.QueryRowContext(ctx, "SELECT revision FROM meta WHERE id=1").Scan(&current); err != nil || current != revision {
		t.Fatal("authentication changed accounting revision", err)
	}
	dataset, err := ledger.ReadContext(ctx, db.Read)
	if err != nil {
		t.Fatal(err)
	}
	save := func(key, role string, active bool, password string) {
		t.Helper()
		res := call(owner, "POST", "/users", map[string]any{"id": "remember-user", "username": "remember-user", "name": "Người thử", "role": role, "active": active, "password": password}, map[string]string{"X-Dataset-Epoch": dataset.Epoch, "Idempotency-Key": key})
		if res.StatusCode != 200 {
			t.Fatal("save user", res.StatusCode)
		}
	}
	for i, test := range []struct {
		name, role, password string
		active               bool
	}{{"role", "viewer", "", true}, {"disabled", "accountant", "", false}, {"password", "accountant", "Synthetic-changed-2026", true}} {
		save("session-base-"+test.name, "accountant", true, "Synthetic-user-2026")
		user := newClient()
		res := login(user, "remember-user", "Synthetic-user-2026", true)
		if res.StatusCode != 200 {
			t.Fatal(i, res.StatusCode)
		}
		if res = call(user, "GET", "/workspace?period=2026-10", nil, nil); res.StatusCode != 200 {
			t.Fatal(res.StatusCode)
		}
		save("session-revoke-"+test.name, test.role, test.active, test.password)
		if res = call(user, "GET", "/workspace?period=2026-10", nil, nil); res.StatusCode != 401 {
			t.Fatal("remembered session survived user change", test.name, res.StatusCode)
		}
	}
	if _, err = db.Write.ExecContext(ctx, "UPDATE sessions SET expires_at='2000-01-01T00:00:00Z'"); err != nil {
		t.Fatal(err)
	}
	if res := call(owner, "GET", "/workspace?period=2026-10", nil, nil); res.StatusCode != 401 {
		t.Fatal("expired session accepted", res.StatusCode)
	}
	if res := call(owner, "POST", "/auth/login", map[string]any{"username": setup.Username, "password": setup.Password, "remember": true}, map[string]string{"Origin": "https://untrusted.invalid"}); res.StatusCode != 403 || len(res.Cookies()) != 0 {
		t.Fatal("cross-origin login accepted")
	}
}
