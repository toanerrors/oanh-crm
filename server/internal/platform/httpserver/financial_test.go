package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestFinancialHTTPAndSafePrint(t *testing.T) {
	store, err := sqlite.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := httptest.NewServer(New(store, "", false))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	var epoch string
	store.Read.QueryRow("SELECT epoch FROM meta").Scan(&epoch)
	call := func(method, path string, body any, key string) (int, []byte) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			reader = bytes.NewReader(b)
		}
		r, e := http.NewRequest(method, server.URL+"/api/v1"+path, reader)
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
	for _, path := range []string{"/financial-reports/export?id=report&version=1&format=xlsx", "/financial-reports/template", "/financial-reports?year=2026", "/financial-reports/source?from=2026-01-01&to=2026-12-31", "/financial-reports/print?id=report&version=1"} {
		if status, _ := call("GET", path, nil, ""); status != 401 {
			t.Fatal(path, status)
		}
	}
	setup := identity.Setup{Username: "test", Password: "Financial-test-2026", Name: "Kiểm thử", CompanyName: "<script>alert(1)</script>", Regime: "Kiểm thử", ConfirmAccounts: true}
	if status, b := call("POST", "/setup", setup, ""); status != 201 {
		t.Fatal(status, string(b))
	}
	if status, b := call("POST", "/auth/login", map[string]string{"username": "test", "password": setup.Password}, ""); status != 200 {
		t.Fatal(status, string(b))
	}
	store.Read.QueryRow("SELECT epoch FROM meta").Scan(&epoch)
	input := ledger.FinancialInput{ID: "report", Template: "tt99-2026-v1", From: "2026-01-01", To: "2026-12-31", PreviousFrom: "2025-01-01", PreviousTo: "2025-12-31", PreparedDate: "2027-01-10", Sheets: []string{"B01-DN"}, Values: map[string]map[string]ledger.FinancialValue{"B01-DN": {"111": {Current: "9007199254740993", Note: "<b>test</b>"}}}}
	if status, b := call("POST", "/financial-reports/preview", input, ""); status != 200 || !strings.Contains(string(b), "9007199254740993") {
		t.Fatal(status, string(b))
	}
	status, b := call("POST", "/financial-reports", input, "save-report")
	if status != 200 {
		t.Fatal(status, string(b))
	}
	status, retry := call("POST", "/financial-reports", input, "save-report")
	if status != 200 || !bytes.Equal(b, retry) {
		t.Fatal(status, string(retry))
	}
	if status, b = call("GET", "/financial-reports/print?id=report&version=1", nil, ""); status != 200 || strings.Contains(string(b), "<script>") || strings.Contains(string(b), "<b>test</b>") || !strings.Contains(string(b), "9007199254740993") {
		t.Fatal(status, string(b))
	}
	status, b = call("GET", "/financial-reports/export?id=report&version=1&format=xlsx", nil, "")
	if status != 200 {
		t.Fatal(status, string(b))
	}
	workbook, e := excelize.OpenReader(bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	sheetRows, e := workbook.GetRows("B01-DN", excelize.Options{RawCellValue: true})
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, row := range sheetRows {
		if len(row) > 3 && row[1] == "111" && row[3] == "9007199254740993" {
			found = true
		}
	}
	if !found || len(workbook.GetSheetList()) != 3 {
		t.Fatal("missing financial values/sheets")
	}
	workbook.Close()
	if status, data := call("GET", "/financial-reports/export?id=report&version=1&format=csv", nil, ""); status != 200 || !strings.Contains(string(data), "BẢN NHÁP BÁO CÁO TÀI CHÍNH") || !strings.Contains(string(data), "bản v1") {
		t.Fatal(status, string(data))
	}
	if status, _ = call("GET", "/financial-reports/export?id=report&version=2&format=xlsx", nil, ""); status != 409 {
		t.Fatal(status)
	}
	if status, _ = call("GET", "/financial-reports/print?id=report&version=2", nil, ""); status != 409 {
		t.Fatal(status)
	}
	if status, b = call("GET", "/financial-reports?year=2026", nil, ""); status != 200 || !strings.Contains(string(b), `"version":1`) {
		t.Fatal(status, string(b))
	}
	if status, _ = call("GET", "/financial-reports/source?from=2026-02-30&to=2026-12-31", nil, ""); status != 422 {
		t.Fatal(status)
	}
}
