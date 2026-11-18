package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/modules/operations"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestRecoveryHTTPManagementScheduleAndSafeActivation(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	auth := identity.Service{Store: store}
	setup := identity.Setup{Username: "backup-owner", Password: "Synthetic-backup-only-2026", Name: "Người kiểm thử", CompanyName: "Doanh nghiệp phục hồi", Regime: "Kiểm thử", ConfirmAccounts: true}
	if err = auth.Setup(ctx, setup); err != nil {
		t.Fatal(err)
	}
	owner, token, err := auth.Login(ctx, setup.Username, setup.Password)
	if err != nil {
		t.Fatal(err)
	}
	current, err := ledger.ReadContext(ctx, store.Read)
	if err != nil {
		t.Fatal(err)
	}
	epoch := current.Epoch
	led := ledger.Service{Store: store}
	cmd := func(key string) ledger.Command { return ledger.Command{Actor: owner.ID, Epoch: epoch, Key: key} }
	if _, err = led.SavePeriod(ctx, cmd("period"), ledger.Period{Period: "2026-10"}); err != nil {
		t.Fatal(err)
	}
	draft := ledger.Document{ID: "posted", Date: "2026-10-01", Type: "journal", Description: "Tiền kiểm thử", Lines: []ledger.Line{{AccountID: "111", Debit: "9007199254740993", Credit: "0"}, {AccountID: "411", Debit: "0", Credit: "9007199254740993"}}}
	if _, err = led.Save(ctx, cmd("draft"), draft); err != nil {
		t.Fatal(err)
	}
	if _, err = led.Post(ctx, cmd("post"), ledger.Post{ID: draft.ID, Version: 1}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(store, "", false))
	defer server.Close()
	rawCall := func(method, path string, body []byte, media, key string) (int, []byte, http.Header) {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: "oanh_session", Value: token})
		req.Header.Set("X-Dataset-Epoch", epoch)
		req.Header.Set("Idempotency-Key", key)
		if media != "" {
			req.Header.Set("Content-Type", media)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, b, res.Header
	}
	call := func(method, path string, body any, key string) (int, []byte) {
		t.Helper()
		var b []byte
		if body != nil {
			b, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		status, b, _ := rawCall(method, path, b, "application/json", key)
		return status, b
	}
	status, b := call("POST", "/backups", map[string]string{"note": "Trước khôi phục"}, "backup-intent")
	if status != 200 {
		t.Fatal(status, string(b))
	}
	var info sqlite.BackupInfo
	if err = json.Unmarshal(b, &info); err != nil || info.ID == "" || info.SHA256 == "" || info.Manifest.Debit != "9007199254740993" {
		t.Fatal(info, err)
	}
	status, b = call("POST", "/backups", map[string]string{"note": "Trước khôi phục"}, "backup-intent")
	var retry sqlite.BackupInfo
	json.Unmarshal(b, &retry)
	if status != 200 || retry.ID != info.ID || retry.SHA256 != info.SHA256 {
		t.Fatal(status, retry)
	}
	if status, b = call("POST", "/backups", map[string]string{"note": "Nội dung khác"}, "backup-intent"); status != 409 {
		t.Fatal(status, string(b))
	}
	status, archive, headers := rawCall("GET", "/backups/"+info.ID+"/download", nil, "", "")
	if status != 200 || sqlite.Hash(archive) != info.SHA256 || headers.Get("X-Backup-SHA256") != info.SHA256 {
		t.Fatal(status, headers)
	}
	if status, b = call("POST", "/backups/"+info.ID+"/verify", map[string]any{}, ""); status != 200 {
		t.Fatal(status, string(b))
	}
	status, b = call("GET", "/backups?q=khong-co", nil, "")
	var overview struct {
		Total, Count int
		Latest       *sqlite.BackupInfo
	}
	json.Unmarshal(b, &overview)
	if status != 200 || overview.Total != 0 || overview.Count != 1 || overview.Latest == nil {
		t.Fatal(status, overview, string(b))
	}
	if status, b = call("GET", "/backups?pageSize=999", nil, ""); status != 400 {
		t.Fatal(status, string(b))
	}
	if status, b = call("POST", "/backups/schedule", map[string]any{"enabled": true, "version": 0}, ""); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, b = call("POST", "/backups/schedule", map[string]any{"enabled": false, "version": 0}, ""); status != 409 {
		t.Fatal(status, string(b))
	}
	status, b = call("POST", "/backups/"+info.ID+"/restore-preview", map[string]any{}, "")
	if status != 200 {
		t.Fatal(status, string(b))
	}
	var preview sqlite.RestorePreview
	json.Unmarshal(b, &preview)
	draft.ID = "after-preview"
	if _, err = led.Save(ctx, cmd("later-draft"), draft); err != nil {
		t.Fatal(err)
	}
	input := map[string]string{"stageId": preview.ID, "confirmCompanyId": current.CompanyID, "reason": "Kiểm thử khôi phục", "confirmSnapshotCompanyId": info.Manifest.CompanyID}
	if status, b = call("POST", "/restore/activate", input, ""); status != 409 {
		t.Fatal(status, string(b))
	}
	status, b, _ = rawCall("POST", "/restore/stage", archive, "application/zip", "")
	if status != 200 {
		t.Fatal(status, string(b))
	}
	json.Unmarshal(b, &preview)
	input["stageId"] = preview.ID
	if status, b = call("POST", "/restore/activate", input, ""); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, _ = call("GET", "/backups", nil, ""); status != 401 {
		t.Fatal("old session survived", status)
	}
	_, token, err = auth.Login(ctx, setup.Username, setup.Password)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := ledger.ReadContext(ctx, store.Read)
	if err != nil || restored.Epoch == epoch {
		t.Fatal(restored, err)
	}
	epoch = restored.Epoch
	status, b = call("GET", "/backups", nil, "")
	var result struct {
		Items          []sqlite.BackupInfo
		Current        sqlite.RecoveryCurrent
		Schedule       sqlite.BackupSchedule
		RecoveryEvents []sqlite.RecoveryEvent
	}
	json.Unmarshal(b, &result)
	if status != 200 || len(result.Items) != 2 || result.Current.Documents != 1 || !result.Schedule.Enabled || len(result.RecoveryEvents) != 1 || result.RecoveryEvents[0].Reason != "Kiểm thử khôi phục" {
		t.Fatal(status, result, string(b))
	}
	// An ordinary user cannot browse, download, verify or activate recovery packages.
	if status, b = call("POST", "/users", map[string]any{"id": "backup-viewer", "username": "backup-viewer", "name": "Chỉ xem", "role": "viewer", "active": true, "password": "Viewer-backup-only-2026"}, "viewer"); status != 200 {
		t.Fatal(status, string(b))
	}
	_, token, err = auth.Login(ctx, "backup-viewer", "Viewer-backup-only-2026")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/backups", "/backups/" + info.ID + "/download"} {
		if status, _ = call("GET", path, nil, ""); status != 403 {
			t.Fatal(path, status)
		}
	}
	for _, path := range []string{"/backups", "/backups/schedule", "/backups/" + info.ID + "/verify", "/backups/" + info.ID + "/restore-preview", "/restore/activate", "/restore/reset"} {
		if status, _ = call("POST", path, map[string]any{}, "viewer"); status != 403 {
			t.Fatal(path, status)
		}
	}
}

func TestResetDatasetKeepingUsersBackupRollbackAndReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := sqlite.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			store.Close()
		}
	}()
	auth := identity.Service{Store: store}
	setup := identity.Setup{Username: "reset-owner", Password: "Synthetic-reset-owner-2026", Name: "Chủ sở hữu", CompanyName: "CÔNG TY KIỂM THỬ ĐẶT LẠI", Regime: "TT99", ConfirmAccounts: true}
	if err = auth.Setup(ctx, setup); err != nil {
		t.Fatal(err)
	}
	_, ownerToken, err := auth.Login(ctx, setup.Username, setup.Password)
	if err != nil {
		t.Fatal(err)
	}
	current, err := ledger.ReadContext(ctx, store.Read)
	if err != nil {
		t.Fatal(err)
	}
	epoch := current.Epoch
	server := httptest.NewServer(New(store, "", false))
	defer server.Close()

	call := func(token, useEpoch, method, path string, body any, key string) (int, []byte) {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: "oanh_session", Value: token})
		req.Header.Set("X-Dataset-Epoch", useEpoch)
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, b
	}

	if status, b := call(ownerToken, epoch, "POST", "/users", map[string]any{"id": "reset-acc", "username": "reset-accountant", "name": "Kế toán viên", "role": "accountant", "active": true, "password": "Synthetic-reset-acc-2026"}, "u1"); status != 200 {
		t.Fatal(status, string(b))
	}
	_, accountantToken, err := auth.Login(ctx, "reset-accountant", "Synthetic-reset-acc-2026")
	if err != nil {
		t.Fatal(err)
	}

	policy := operations.Policy{
		Regime: "TT99", EffectiveFrom: "2026-07-01", Confirmed: true, Rounding: "half_up", VATMethod: "deduction", VATRates: []string{"0", "8", "10"},
		InventoryMethod: "fifo", QuantityScale: 0, StandardDays: "26", HoursPerDay: "8", PersonalDeduction: "11000000", DependentDeduction: "4400000", CorporateTaxRate: "20",
		Insurance:          []operations.Insurance{{Name: "BHXH", EmployeeRate: "8", EmployerRate: "17.5"}},
		PITBrackets:        []operations.Bracket{{UpTo: "5000000", Rate: "5"}, {UpTo: "", Rate: "10"}},
		DepreciationMethod: "straight_line", ResidualMethod: "last_period",
	}
	status, b := call(ownerToken, epoch, "POST", "/policies", policy, "p1")
	if status != 200 {
		t.Fatal(status, string(b))
	}
	var savedPolicy operations.Policy
	if err = json.Unmarshal(b, &savedPolicy); err != nil || savedPolicy.ID == "" {
		t.Fatal(string(b), err)
	}

	// Populate test catalogs, custom account, warehouse, bank, project, and posted + draft documents.
	if status, b = call(ownerToken, epoch, "POST", "/accounts", map[string]any{"id": "1122", "code": "1122", "name": "Tiền gửi Vietbank test", "group": "cash", "parentId": "112", "requiresPartner": false, "active": true}, "a1"); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, b = call(ownerToken, epoch, "POST", "/partners", map[string]any{"partners": []map[string]any{{"id": "part-test", "code": "KH-TEST", "name": "Khách test", "role": "both", "taxCode": "0312345678", "address": "Q1", "email": "", "phone": "", "active": true}}}, "pt1"); status != 200 {
		t.Fatal(status, string(b))
	}
	var savedPartners []ledger.Partner
	if err = json.Unmarshal(b, &savedPartners); err != nil || len(savedPartners) != 1 {
		t.Fatal(string(b), err)
	}
	if status, b = call(ownerToken, epoch, "POST", "/projects", map[string]any{"projects": []map[string]any{{"id": "proj-test", "code": "CT-TEST", "name": "Công trình test", "description": "Thử", "active": true}}}, "pr1"); status != 200 {
		t.Fatal(status, string(b))
	}
	var savedProjects []ledger.Project
	if err = json.Unmarshal(b, &savedProjects); err != nil || len(savedProjects) != 1 {
		t.Fatal(string(b), err)
	}
	if status, b = call(ownerToken, epoch, "POST", "/items", map[string]any{"id": "item-test", "code": "DV-TEST", "name": "Dịch vụ test", "unit": "gói", "tracked": false, "active": true}, "it1"); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, b = call(ownerToken, epoch, "POST", "/warehouses", map[string]any{"id": "wh-test", "code": "KHO_PHU", "name": "Kho phụ test", "active": true}, "wh1"); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, b = call(ownerToken, epoch, "POST", "/bank-accounts", map[string]any{"id": "bank-test", "number": "1900123456", "bankName": "Vietbank", "accountId": "1122", "active": true}, "bk1"); status != 200 {
		t.Fatal(status, string(b))
	}
	salePayload := map[string]any{"kind": "sale", "tradeMode": "service", "priceMode": "exclusive", "sourceId": "sale-test-1", "date": "2026-11-18", "description": "Doanh thu test", "partnerId": savedPartners[0].ID, "lines": []map[string]any{{"itemId": "item-test", "projectId": savedProjects[0].ID, "quantity": "1", "unitPrice": "90071992547410", "vatRate": "8"}}}
	status, b = call(ownerToken, epoch, "POST", "/operations/preview", salePayload, "op1-preview")
	if status != 200 {
		t.Fatal(status, string(b))
	}
	var calc operations.Result
	if err = json.Unmarshal(b, &calc); err != nil {
		t.Fatal(err)
	}
	salePayload["expectedPolicyId"] = calc.PolicyID
	salePayload["expectedRevision"] = calc.Revision
	if status, b = call(ownerToken, epoch, "POST", "/operations", salePayload, "op1"); status != 200 {
		t.Fatal(status, string(b))
	}
	if status, b = call(ownerToken, epoch, "POST", "/voucher-drafts", map[string]any{"kind": "purchase", "tradeMode": "service", "priceMode": "exclusive", "sourceId": "draft-test-1", "date": "2026-11-18", "description": "Nháp test", "partnerId": savedPartners[0].ID, "lines": []map[string]any{{"itemId": "item-test", "quantity": "1", "unitPrice": "100000", "vatRate": "8", "deductible": true}}}, "dr1"); status != 200 {
		t.Fatal(status, string(b))
	}

	// Accountant cannot reset; wrong company name or wrong epoch is rejected.
	if status, _ = call(accountantToken, epoch, "POST", "/restore/reset", map[string]any{"confirmCompanyId": current.CompanyID, "confirmCompanyName": setup.CompanyName, "reason": "Thử quyền"}, ""); status != 403 {
		t.Fatal("accountant allowed:", status)
	}
	if status, _ = call(ownerToken, epoch, "POST", "/restore/reset", map[string]any{"confirmCompanyId": current.CompanyID, "confirmCompanyName": "Sai tên", "reason": "Đặt lại"}, ""); status != 422 {
		t.Fatal("wrong company name allowed:", status)
	}
	if status, _ = call(ownerToken, "stale-epoch", "POST", "/restore/reset", map[string]any{"confirmCompanyId": current.CompanyID, "confirmCompanyName": setup.CompanyName, "reason": "Đặt lại"}, ""); status != 409 {
		t.Fatal("stale epoch allowed:", status)
	}

	// Execute reset keeping users & policies.
	status, b = call(ownerToken, epoch, "POST", "/restore/reset", map[string]any{"confirmCompanyId": current.CompanyID, "confirmCompanyName": setup.CompanyName, "reason": "Xóa toàn bộ dữ liệu test, giữ người dùng"}, "")
	if status != 200 {
		t.Fatal(status, string(b))
	}

	// Old epoch must be rejected on commands.
	if status, _ = call(ownerToken, epoch, "POST", "/projects", map[string]any{"projects": []map[string]any{{"id": "stale-proj", "code": "STALE", "name": "Cũ", "description": "", "active": true}}}, "stale-cmd"); status != 409 {
		t.Fatal("old epoch command should fail:", status)
	}

	after, err := ledger.ReadContext(ctx, store.Read)
	if err != nil {
		t.Fatal(err)
	}
	if after.Epoch == epoch || after.CompanyID != current.CompanyID || after.CompanyName != setup.CompanyName || after.Regime != setup.Regime {
		t.Fatalf("unexpected meta after reset: %+v", after)
	}

	// Both owner and accountant sessions and credentials remain valid; NeedsSetup is false.
	needs, err := auth.NeedsSetup(ctx)
	if err != nil || needs {
		t.Fatal("needsSetup should remain false:", needs, err)
	}
	if _, err = auth.Authenticate(ctx, ownerToken); err != nil {
		t.Fatal("owner session should survive:", err)
	}
	if _, err = auth.Authenticate(ctx, accountantToken); err != nil {
		t.Fatal("accountant session should survive:", err)
	}
	if _, _, err = auth.Login(ctx, "reset-accountant", "Synthetic-reset-acc-2026"); err != nil {
		t.Fatal("accountant password login should succeed:", err)
	}

	// All test data is wiped; standard accounts (186), default warehouse (1), users (2), and policy (1) remain.
	for query, want := range map[string]int{
		"SELECT count(*) FROM users":                    2,
		"SELECT count(*) FROM documents":                0,
		"SELECT count(*) FROM partners":                 0,
		"SELECT count(*) FROM projects":                 0,
		"SELECT count(*) FROM items":                    0,
		"SELECT count(*) FROM bank_accounts":            0,
		"SELECT count(*) FROM warehouses":               1,
		"SELECT count(*) FROM policies":                 1,
		"SELECT count(*) FROM accounts WHERE id='1122'": 0,
		"SELECT count(*) FROM accounts WHERE id='1121'": 1,
	} {
		var got int
		if err = store.Read.QueryRowContext(ctx, query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s = %d, want %d (err=%v)", query, got, want, err)
		}
	}

	// Unused preserved policy can be updated in place (e.g. effectiveFrom moved earlier to 2026-01-01).
	savedPolicy.EffectiveFrom = "2026-01-01"
	if status, b = call(ownerToken, after.Epoch, "POST", "/policies", savedPolicy, "p-edit"); status != 200 {
		t.Fatal("expected unused policy update to succeed:", status, string(b))
	}

	// Pre-reset backup exists, verifies cleanly, and retains the test posted document for rollback safety.
	backups, unreadable, err := store.ListBackups(ctx, after.CompanyID)
	if err != nil || unreadable != 0 || len(backups) != 1 || backups[0].Kind != "before_restore" || backups[0].Manifest.Documents != 2 || backups[0].Manifest.Posted != 1 {
		t.Fatalf("unexpected pre-reset backups: %+v unreadable=%d err=%v", backups, unreadable, err)
	}
	if _, _, err = store.ReadBackup(ctx, backups[0].ID, after.CompanyID, true); err != nil {
		t.Fatal("pre-reset backup failed verification:", err)
	}

	// Reopen store and run full sqlite.Check.
	closed = true
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = sqlite.Check(ctx, reopened.Read); err != nil {
		t.Fatal(err)
	}
}
