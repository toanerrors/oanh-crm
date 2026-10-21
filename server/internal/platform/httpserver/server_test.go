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

	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestHTTPAuthenticationPermissionsAndContext(t *testing.T) {
	db, e := sqlite.Open(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	handler := New(db, "", false)
	server := httptest.NewServer(handler)
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	call := func(method, path string, input any, headers map[string]string) (int, []byte) {
		t.Helper()
		var body io.Reader
		if input != nil {
			b, e := json.Marshal(input)
			if e != nil {
				t.Fatal(e)
			}
			body = bytes.NewReader(b)
		}
		request, e := http.NewRequest(method, server.URL+path, body)
		if e != nil {
			t.Fatal(e)
		}
		if input != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		for k, v := range headers {
			request.Header.Set(k, v)
		}
		response, e := client.Do(request)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		b, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, b
	}
	if status, _ := call("GET", "/api/v1/workspace?period=2026-10", nil, nil); status != 401 {
		t.Fatal(status)
	}
	if status, _ := call("GET", "/api/v1/trade-invoices?asOf=2026-10-21&kind=sale", nil, nil); status != 401 {
		t.Fatal(status)
	}
	setup := identity.Setup{Username: "admin", Password: "Integration-test-2026", Name: "Quản trị thử", CompanyName: "Doanh nghiệp thử", Regime: "Chính sách thử", ConfirmAccounts: true}
	if status, _ := call("POST", "/api/v1/setup", setup, map[string]string{"Origin": "https://untrusted.invalid"}); status != 403 {
		t.Fatal(status)
	}
	if status, b := call("POST", "/api/v1/setup", setup, nil); status != 201 {
		t.Fatal(status, string(b))
	}
	if status, _ := call("POST", "/api/v1/setup", setup, nil); status != 409 {
		t.Fatal(status)
	}
	if status, b := call("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": setup.Password}, nil); status != 200 {
		t.Fatal(status, string(b))
	}
	for _, query := range []string{"asOf=2026-02-30&kind=sale", "asOf=2026-10-21&kind=unknown", "asOf=2026-10-21&kind=sale&pageSize=101", "asOf=2026-10-21&kind=sale&page=wrong"} {
		if status, b := call("GET", "/api/v1/trade-invoices?"+query, nil, nil); status != 400 {
			t.Fatal(status, string(b))
		}
	}
	if status, b := call("GET", "/api/v1/trade-invoices?asOf=2026-10-21&kind=sale", nil, nil); status != 200 {
		t.Fatal(status, string(b))
	}
	status, b := call("GET", "/api/v1/session", nil, nil)
	if status != 200 {
		t.Fatal(status, string(b))
	}
	var session struct {
		Context struct {
			Epoch string `json:"epoch"`
		} `json:"context"`
	}
	if e = json.Unmarshal(b, &session); e != nil {
		t.Fatal(e)
	}
	headers := map[string]string{"X-Dataset-Epoch": session.Context.Epoch, "Idempotency-Key": "open-period"}
	if status, b = call("POST", "/api/v1/periods", map[string]any{"period": "2026-10", "closed": false}, headers); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, _ = call("POST", "/api/v1/periods", map[string]any{"period": "2026-11", "closed": false}, headers); status != 409 {
		t.Fatal(status)
	}
	if status, _ = call("POST", "/api/v1/periods", map[string]any{"period": "2026-11", "closed": false}, map[string]string{"X-Dataset-Epoch": "old", "Idempotency-Key": "old-epoch"}); status != 409 {
		t.Fatal(status)
	}
	if status, _ = call("GET", "/api/v1/missing", nil, map[string]string{"Accept": "text/html"}); status != 404 {
		t.Fatal(status)
	}
	if status, _ = call("POST", "/api/v1/documents", map[string]any{"unknown": true}, headers); status != 400 {
		t.Fatal(status)
	}
	if status, _ = call("GET", "/api/v1/documents?period=2026-10&pageSize=1000", nil, nil); status != 400 {
		t.Fatal(status)
	}
	if status, b = call("POST", "/api/v1/users", map[string]any{"id": "viewer", "username": "viewer", "name": "Chỉ xem", "role": "viewer", "active": true, "password": "Viewer-test-only-2026"}, map[string]string{"X-Dataset-Epoch": session.Context.Epoch, "Idempotency-Key": "viewer-create"}); status != 200 {
		t.Fatal(status, string(b))
	}
	call("POST", "/api/v1/auth/logout", nil, nil)
	if status, _ = call("GET", "/api/v1/workspace?period=2026-10", nil, nil); status != 401 {
		t.Fatal(status)
	}
	if status, _ = call("POST", "/api/v1/auth/login", map[string]string{"username": "viewer", "password": "Viewer-test-only-2026"}, nil); status != 200 {
		t.Fatal(status)
	}
	if status, _ = call("GET", "/api/v1/workspace?period=2026-10", nil, nil); status != 200 {
		t.Fatal(status)
	}
	if status, _ = call("GET", "/api/v1/automation", nil, nil); status != 403 {
		t.Fatal(status)
	}
	if status, _ = call("POST", "/api/v1/periods", map[string]any{"period": "2026-11", "closed": false}, map[string]string{"X-Dataset-Epoch": session.Context.Epoch, "Idempotency-Key": "viewer-denied"}); status != 403 {
		t.Fatal(status)
	}
}
