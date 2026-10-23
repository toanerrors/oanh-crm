package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/modules/operations"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestAuditHTTPFiltersDetailPreviewRestoreAndPermissions(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	auth := identity.Service{Store: store}
	input := identity.Setup{Username: "audit-owner", Password: "Synthetic-audit-2026", Name: "Người kiểm thử", CompanyName: "Thử", Regime: "Thử", ConfirmAccounts: true}
	if err = auth.Setup(ctx, input); err != nil {
		t.Fatal(err)
	}
	user, token, err := auth.Login(ctx, input.Username, input.Password)
	if err != nil {
		t.Fatal(err)
	}
	session, err := ledger.ReadContext(ctx, store.Read)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(store, "", false))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	call := func(method, path string, payload any, key string, authorized bool) (int, []byte) {
		t.Helper()
		var body io.Reader
		if payload != nil {
			b, e := json.Marshal(payload)
			if e != nil {
				t.Fatal(e)
			}
			body = bytes.NewReader(b)
		}
		req, e := http.NewRequest(method, server.URL+"/api/v1"+path, body)
		if e != nil {
			t.Fatal(e)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if authorized {
			req.AddCookie(&http.Cookie{Name: "oanh_session", Value: token})
		}
		req.Header.Set("X-Dataset-Epoch", session.Epoch)
		req.Header.Set("Idempotency-Key", key)
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		b, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, b
	}
	for _, path := range []string{"/audit/events", "/audit/detail?id=1"} {
		if status, _ := call("GET", path, nil, "", false); status != 401 {
			t.Fatal(path, status)
		}
	}
	l := ledger.Service{Store: store}
	cmd := func(key string) ledger.Command { return ledger.Command{Actor: user.ID, Epoch: session.Epoch, Key: key} }
	if _, err = l.SavePartners(ctx, cmd("audit-customer-create"), []ledger.Partner{{ID: "audit-customer", Code: "AUD", Name: "Đối tượng thử", Role: "customer", Email: "old@example.invalid", Active: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err = l.SavePartners(ctx, cmd("audit-customer-edit"), []ledger.Partner{{ID: "audit-customer", Code: "AUD", Name: "Đối tượng thử", Role: "customer", Email: "new@example.invalid", Active: true}}); err != nil {
		t.Fatal(err)
	}
	status, b := call("GET", "/audit/events?q=cap+nhat+khach&page=1&pageSize=1", nil, "", true)
	if status != 200 {
		t.Fatal(status, string(b))
	}
	var list ledger.AuditPage
	if err = json.Unmarshal(b, &list); err != nil || len(list.Items) != 1 || list.Total != 2 {
		t.Fatal(string(b), err)
	}
	id := list.Items[0].ID
	if list.Items[0].Title == list.Items[0].Action || list.Items[0].EntityName == "" {
		t.Fatal("technical journal")
	}
	status, b = call("GET", "/audit/detail?id="+id+"&page=1&pageSize=1", nil, "", true)
	if status != 200 || !bytes.Contains(b, []byte("old@example.invalid")) || !bytes.Contains(b, []byte(`"canRestore":true`)) {
		t.Fatal(status, string(b))
	}
	in := operations.AuditRestoreInput{AuditID: id, Reason: "Sửa nhầm email"}
	status, b = call("POST", "/audit/restore/preview", in, "audit-http-preview", true)
	if status != 200 {
		t.Fatal(status, string(b))
	}
	var p operations.AuditRestorePreview
	if err = json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	var email string
	store.Read.QueryRow("SELECT email FROM partners WHERE id='audit-customer'").Scan(&email)
	if email != "new@example.invalid" {
		t.Fatal("preview applied data")
	}
	if status, _ = call("POST", "/audit/restore", in, "without-preview", true); status != 422 {
		t.Fatal(status)
	}
	in.ExpectedRevision = &p.Revision
	in.Fingerprint = p.Fingerprint
	status, b = call("POST", "/audit/restore", in, "audit-http-confirm", true)
	if status != 200 {
		t.Fatal(status, string(b))
	}
	status, retry := call("POST", "/audit/restore", in, "audit-http-confirm", true)
	if status != 200 || !bytes.Equal(b, retry) {
		t.Fatal("retry changed result", status)
	}
	store.Read.QueryRow("SELECT email FROM partners WHERE id='audit-customer'").Scan(&email)
	if email != "old@example.invalid" {
		t.Fatal(email)
	}
	status, b = call("GET", "/audit/detail?id="+id, nil, "", true)
	if status != 200 || !bytes.Contains(b, []byte(`"canRestore":false`)) || !bytes.Contains(b, []byte(`"restoredBy"`)) {
		t.Fatal(status, string(b))
	}
	if _, err = store.Write.Exec("INSERT INTO audit(actor_id,action,entity_id,occurred_at,revision) VALUES(?,'legacy.action','old','2026-10-22T17:00:00Z',0)", user.ID); err != nil {
		t.Fatal(err)
	}
	var legacy int64
	store.Read.QueryRow("SELECT MAX(id) FROM audit").Scan(&legacy)
	status, b = call("GET", "/audit/detail?id="+strconv.FormatInt(legacy, 10), nil, "", true)
	if status != 200 || !bytes.Contains(b, []byte(`"legacy":true`)) || !bytes.Contains(b, []byte(`"canRestore":false`)) {
		t.Fatal(status, string(b))
	}
	status, b = call("GET", "/audit/events?from=2026-10-23&to=2026-10-23&action=legacy.action", nil, "", true)
	if status != 200 || !bytes.Contains(b, []byte(`"total":1`)) {
		t.Fatal(status, string(b))
	}
	for _, query := range []string{"page=0", "pageSize=101", "from=2026-02-30", "from=2026-10-23&to=2026-10-01"} {
		if status, _ = call("GET", "/audit/events?"+query, nil, "", true); status != 400 {
			t.Fatal(query, status)
		}
	}
	const secret = "Synthetic-audit-child-2026"
	if status, b = call("POST", "/users", map[string]any{"id": "audit-child", "username": "audit-child", "name": "Người thử mới", "role": "viewer", "active": true, "password": secret}, "audit-user-create", true); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, b = call("POST", "/users", map[string]any{"id": "audit-child", "username": "audit-child", "name": "Người thử mới", "role": "viewer", "active": true, "password": "Synthetic-audit-child-2027"}, "audit-password-change", true); status != 200 {
		t.Fatal(status, string(b))
	}
	var passwordEvent string
	if err = store.Read.QueryRow("SELECT CAST(MAX(id) AS TEXT) FROM audit WHERE action='admin.user.save'").Scan(&passwordEvent); err != nil {
		t.Fatal(err)
	}
	status, b = call("GET", "/audit/detail?id="+passwordEvent, nil, "", true)
	if status != 200 || !bytes.Contains(b, []byte(`"credential_changed":"1"`)) || !bytes.Contains(b, []byte(`"entityName":"Người thử mới · audit-child"`)) {
		t.Fatal(status, string(b))
	}
	if _, err = store.Write.Exec("UPDATE users SET role='viewer' WHERE id=?", user.ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/audit/events", "/audit/detail?id=" + id} {
		if status, _ = call("GET", path, nil, "", true); status != 403 {
			t.Fatal(path, status)
		}
	}
	if status, _ = call("POST", "/audit/restore/preview", in, "viewer-preview", true); status != 403 {
		t.Fatal(status)
	}
	if status, _ = call("POST", "/audit/restore", in, "viewer-restore", true); status != 403 {
		t.Fatal(status)
	}
	if err = sqlite.Check(ctx, store.Read); err != nil {
		t.Fatal(err)
	}
	var snapshots string
	rows, err := store.Read.Query("SELECT COALESCE(before_state,'')||COALESCE(after_state,'') FROM audit_changes")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		rows.Scan(&s)
		snapshots += s
	}
	rows.Close()
	if strings.Contains(snapshots, "password_hash") || strings.Contains(snapshots, input.Password) || strings.Contains(snapshots, secret) || strings.Contains(snapshots, "Synthetic-audit-child-2027") {
		t.Fatal("credential leaked into journal")
	}
}
