package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/xuri/excelize/v2"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/operations"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestHTTPExcelUploadMappingErrorsCommitAndPermissions(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := httptest.NewServer(New(store, "", false))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	request := func(method, path string, body []byte, contentType, epoch, key string) (int, []byte) {
		t.Helper()
		r, e := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		r.Header.Set("X-Dataset-Epoch", epoch)
		r.Header.Set("Idempotency-Key", key)
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, raw
	}
	post := func(path string, input any, epoch, key string) (int, []byte) {
		t.Helper()
		b, e := json.Marshal(input)
		if e != nil {
			t.Fatal(e)
		}
		return request("POST", path, b, "application/json", epoch, key)
	}
	if status, _ := request("GET", "/excel-imports/schemas", nil, "", "", ""); status != 401 {
		t.Fatal(status)
	}
	setup := identity.Setup{Username: "admin", Password: "Excel-integration-test-2026", Name: "Kiểm thử", CompanyName: "Doanh nghiệp giả lập", Regime: "test", ConfirmAccounts: true}
	if status, raw := post("/setup", setup, "", ""); status != 201 {
		t.Fatal(status, string(raw))
	}
	if status, raw := post("/auth/login", map[string]string{"username": "admin", "password": setup.Password}, "", ""); status != 200 {
		t.Fatal(status, string(raw))
	}
	_, raw := request("GET", "/session", nil, "", "", "")
	var session struct {
		Context struct {
			Epoch string `json:"epoch"`
		} `json:"context"`
	}
	if err = json.Unmarshal(raw, &session); err != nil {
		t.Fatal(err)
	}
	epoch := session.Context.Epoch
	status, raw := request("GET", "/excel-imports/template?kind=partners&advanced=true", nil, "", epoch, "")
	if status != 200 || !bytes.HasPrefix(raw, []byte("PK")) {
		t.Fatal(status, string(raw))
	}
	book := excelize.NewFile()
	defer book.Close()
	for i, value := range []string{"code", "name", "role"} {
		axis, _ := excelize.CoordinatesToCellName(i+1, 1)
		book.SetCellStr("Sheet1", axis, value)
	}
	for i, value := range []string{"0000123", "Đối tượng kiểm thử", "customer"} {
		axis, _ := excelize.CoordinatesToCellName(i+1, 2)
		book.SetCellStr("Sheet1", axis, value)
	}
	data, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "kiem-thu.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	part.Write(data.Bytes())
	form.Close()
	status, raw = request("POST", "/excel-imports/files", body.Bytes(), form.FormDataContentType(), epoch, "upload")
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	var batch operations.ExcelBatch
	if err = json.Unmarshal(raw, &batch); err != nil {
		t.Fatal(err)
	}
	if status, raw = request("GET", "/excel-imports/"+batch.ID+"?sheet=Sheet1&headerRow=1", nil, "", "stale", ""); status != 409 {
		t.Fatal(status, string(raw))
	}
	config := operations.ExcelConfiguration{SchemaVersion: 1, Kind: "partners", Sheet: "Sheet1", HeaderRow: 1, Mapping: map[string]int{"code": 0, "name": 1, "role": 2}, DateFormat: "dmy", NumberFormat: "plain", Mode: "add"}
	status, raw = post("/excel-imports/"+batch.ID+"/configuration", config, epoch, "configuration")
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	status, raw = post("/excel-imports/"+batch.ID+"/preview", config, epoch, "")
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	var preview operations.ExcelPreview
	if err = json.Unmarshal(raw, &preview); err != nil || preview.ErrorCount != 0 {
		t.Fatal(preview, err)
	}
	config.Mapping["name"] = 0
	status, raw = post("/excel-imports/"+batch.ID+"/errors", config, epoch, "")
	if status != 200 || !bytes.HasPrefix(raw, []byte("PK")) {
		t.Fatal(status, string(raw))
	}
	config.Mapping["name"] = 1
	status, raw = post("/excel-imports/"+batch.ID+"/commit", operations.ExcelCommit{Configuration: config, ExpectedRevision: preview.Revision, Fingerprint: preview.Fingerprint}, epoch, "commit")
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	var code string
	if err = store.Read.QueryRow("SELECT code FROM partners WHERE code='0000123'").Scan(&code); err != nil || code != "0000123" {
		t.Fatal(code, err)
	}
	if status, raw = post("/users", map[string]any{"id": "viewer", "username": "viewer", "name": "Chỉ xem", "role": "viewer", "active": true, "password": "Excel-viewer-test-2026"}, epoch, "create-viewer"); status != 200 {
		t.Fatal(status, string(raw))
	}
	post("/auth/logout", nil, "", "")
	post("/auth/login", map[string]string{"username": "viewer", "password": "Excel-viewer-test-2026"}, "", "")
	for _, path := range []string{"/excel-imports/schemas", "/excel-imports", "/excel-imports/" + batch.ID, "/excel-imports/template?kind=partners"} {
		if status, raw = request("GET", path, nil, "", epoch, ""); status != 403 {
			t.Fatal(path, status, string(raw))
		}
	}
}
