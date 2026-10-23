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
	for _, path := range []string{"/backups", "/backups/schedule", "/backups/" + info.ID + "/verify", "/backups/" + info.ID + "/restore-preview", "/restore/activate"} {
		if status, _ = call("POST", path, map[string]any{}, "viewer"); status != 403 {
			t.Fatal(path, status)
		}
	}
}
