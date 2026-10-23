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
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/modules/operations"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestOpeningHTTPPermissionsRetryAndVersion(t *testing.T) {
	ctx := context.Background()
	store, e := sqlite.Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	auth := identity.Service{Store: store}
	if e = auth.Setup(ctx, identity.Setup{Username: "owner", Password: "Opening-test-2026", Name: "Giả lập", CompanyName: "Kiểm thử", Regime: "Kiểm thử", ConfirmAccounts: true}); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(New(store, "", false))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	var epoch string
	store.Read.QueryRow("SELECT epoch FROM meta").Scan(&epoch)
	call := func(method, path string, input any, key string) (int, []byte) {
		t.Helper()
		var body io.Reader
		if input != nil {
			b, _ := json.Marshal(input)
			body = bytes.NewReader(b)
		}
		r, e := http.NewRequest(method, server.URL+"/api/v1"+path, body)
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Content-Type", "application/json")
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
			r.Header.Set("X-Dataset-Epoch", epoch)
		}
		response, e := client.Do(r)
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
	if status, _ := call("GET", "/opening-balances", nil, ""); status != 401 {
		t.Fatal(status)
	}
	if status, b := call("POST", "/auth/login", map[string]string{"username": "owner", "password": "Opening-test-2026"}, ""); status != 200 {
		t.Fatal(status, string(b))
	}
	if _, e = store.Write.Exec("INSERT INTO periods VALUES('2026-10',0)"); e != nil {
		t.Fatal(e)
	}
	input := operations.OpeningInput{ID: "opening-http", Cutoff: "2026-10-01", Source: "Giả lập", Lines: []operations.OpeningLine{{Line: ledger.Line{AccountID: "111", Debit: "9007199254740993", Credit: "0"}}, {Line: ledger.Line{AccountID: "411", Debit: "0", Credit: "9007199254740993"}}}}
	if status, _ := call("POST", "/opening-balances", input, ""); status != 422 {
		t.Fatal(status)
	}
	status, b := call("POST", "/opening-balances", input, "save-opening")
	if status != 200 {
		t.Fatal(status, string(b))
	}
	status, retry := call("POST", "/opening-balances", input, "save-opening")
	if status != 200 || !bytes.Equal(b, retry) {
		t.Fatal(status, string(retry))
	}
	input.Source = "Khác"
	if status, _ := call("POST", "/opening-balances", input, "save-opening"); status != 409 {
		t.Fatal(status)
	}
	if status, _ := call("POST", "/opening-balances/confirm", ledger.Post{ID: input.ID, Version: 0}, "stale-post"); status != 409 {
		t.Fatal(status)
	}
	if status, b = call("POST", "/opening-balances/discard", ledger.Post{ID: input.ID, Version: 1}, "discard"); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, b = call("POST", "/opening-balances", input, "save-again"); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, b = call("POST", "/opening-balances/confirm", ledger.Post{ID: input.ID, Version: 1}, "confirm"); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, _ := call("POST", "/opening-balances/discard", ledger.Post{ID: input.ID, Version: 2}, "discard-posted"); status != 409 {
		t.Fatal(status)
	}
	if status, _ := call("GET", "/reports?kind=trial&period=2026-09", nil, ""); status != 422 {
		t.Fatal(status)
	}
	if status, _ := call("GET", "/financial-reports/source?from=2026-01-01&to=2026-12-31", nil, ""); status != 422 {
		t.Fatal(status)
	}

	adjustment := operations.OpeningAdjustmentInput{Input: input, BaseVersion: 0, Reason: "Đối chiếu lại"}
	adjustment.Input.ID = "http-adjustment"
	adjustment.Input.Version = 0
	adjustment.Input.Lines = append([]operations.OpeningLine(nil), input.Lines...)
	adjustment.Input.Lines[0].Debit = "5000000"
	adjustment.Input.Lines[1].Credit = "5000000"
	if status, b = call("POST", "/opening-balances/adjustments", adjustment, "adjust-save"); status != 200 {
		t.Fatal(status, string(b))
	}
	status, b = call("GET", "/opening-balances/adjustments/http-adjustment", nil, "")
	if status != 200 {
		t.Fatal(status, string(b))
	}
	var preview operations.OpeningAdjustment
	if e = json.Unmarshal(b, &preview); e != nil {
		t.Fatal(e)
	}
	confirm := operations.OpeningAdjustmentPost{ID: "http-adjustment", Version: preview.Document.Version, Revision: preview.Revision}
	if status, b = call("POST", "/opening-balances/adjustments/confirm", confirm, "adjust-confirm"); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, b = call("GET", "/opening-balances/adjustments?page=1", nil, ""); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, b = call("GET", "/opening-balances/adjustments/"+input.ID, nil, ""); status != 200 {
		t.Fatal(status, string(b))
	}
	if _, e = store.Write.Exec("UPDATE users SET role='accountant'"); e != nil {
		t.Fatal(e)
	}

	if status, _ = call("POST", "/opening-balances/adjustments", adjustment, "accountant-adjust"); status != 403 {
		t.Fatal(status)
	}
	if status, _ = call("GET", "/opening-balances/adjustments/http-adjustment", nil, ""); status != 200 {
		t.Fatal(status)
	}
	input.ID = "other"
	if status, _ := call("POST", "/opening-balances", input, "accountant-save"); status != 403 {
		t.Fatal(status)
	}
	if status, _ := call("GET", "/opening-balances", nil, ""); status != 200 {
		t.Fatal(status)
	}
	if _, e = store.Write.Exec("UPDATE users SET role='viewer'"); e != nil {
		t.Fatal(e)
	}
	if status, _ := call("POST", "/opening-balances/confirm", ledger.Post{ID: input.ID, Version: 1}, "viewer-post"); status != 403 {
		t.Fatal(status)
	}
	if e = sqlite.Check(ctx, store.Read); e != nil {
		t.Fatal(e)
	}
}
