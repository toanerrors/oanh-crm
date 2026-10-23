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

	"github.com/xuri/excelize/v2"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestExportFormatsSnapshotsPermissionsAndPrecision(t *testing.T) {
	db, err := sqlite.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := httptest.NewServer(New(db, "", false))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	epoch := ""
	sequence := 0
	call := func(method, path string, input any) (int, []byte, http.Header) {
		t.Helper()
		var reader io.Reader
		if input != nil {
			data, _ := json.Marshal(input)
			reader = bytes.NewReader(data)
		}
		request, _ := http.NewRequest(method, server.URL+"/api/v1"+path, reader)
		request.Header.Set("Content-Type", "application/json")
		if method != "GET" {
			request.Header.Set("X-Dataset-Epoch", epoch)
			sequence++
			request.Header.Set("Idempotency-Key", "export-test-"+strconv.Itoa(sequence))
		}
		response, e := client.Do(request)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, data, response.Header
	}
	for _, path := range []string{"/documents/export?period=2026-10&format=xlsx", "/reports/export?period=2026-10&format=print", "/catalog/export?period=2026-10&catalog=accounts&format=xlsx", "/financial-reports/export?id=report&version=1&format=xlsx"} {
		if status, _, _ := call("GET", path, nil); status != 401 {
			t.Fatal(path, status)
		}
	}
	setup := identity.Setup{Username: "export-owner", Password: "Export-test-only-2026", Name: "Kiểm thử", CompanyName: "<script>test</script>", Regime: "Kiểm thử", ConfirmAccounts: true}
	if status, b, _ := call("POST", "/setup", setup); status != 201 {
		t.Fatal(status, string(b))
	}
	if status, b, _ := call("POST", "/auth/login", map[string]string{"username": setup.Username, "password": setup.Password}); status != 200 {
		t.Fatal(status, string(b))
	}
	db.Read.QueryRow("SELECT epoch FROM meta").Scan(&epoch)
	if status, b, _ := call("POST", "/periods", map[string]any{"period": "2026-10", "closed": false}); status != 200 {
		t.Fatal(status, string(b))
	}
	for index, amount := range []string{"9007199254740993", "5000000"} {
		id := "export-" + strconv.Itoa(index)
		document := ledger.Document{ID: id, Type: "journal", Date: "2026-10-20", Description: "<script>alert(1)</script> Xuất thử", Status: "draft", Lines: []ledger.Line{{AccountID: "111", Debit: amount, Credit: "0"}, {AccountID: "4111", Debit: "0", Credit: amount}}}
		if status, b, _ := call("POST", "/documents", document); status != 200 {
			t.Fatal(status, string(b))
		}
		if status, b, _ := call("POST", "/documents/"+id+"/post", map[string]int{"version": 1}); status != 200 {
			t.Fatal(status, string(b))
		}
	}
	var revision int64
	db.Read.QueryRow("SELECT revision FROM meta").Scan(&revision)
	for _, kind := range []string{"documents", "reports"} {
		path := "/" + kind + "/export?period=2026-10&page=2&pageSize=1&kind=ledger&account=111&q=xuat&format=xlsx&revision=" + strconv.FormatInt(revision, 10)
		status, raw, headers := call("GET", path, nil)
		if status != 200 || !strings.Contains(headers.Get("Content-Type"), "spreadsheetml") || !strings.Contains(headers.Get("Content-Disposition"), ".xlsx") {
			t.Fatal(status, headers, string(raw))
		}
		f, e := excelize.OpenReader(bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		sheet := "Chứng từ"
		if kind == "reports" {
			sheet = "Sổ kế toán"
		}
		rows, e := f.GetRows(sheet, excelize.Options{RawCellValue: true})
		if e != nil || len(rows) != 9 {
			t.Fatalf("%s %d rows %v", kind, len(rows), e)
		}
		found := false
		for _, row := range rows {
			for _, value := range row {
				if value == "9007199254740993" {
					found = true
				}
			}
		}
		f.Close()
		if !found {
			t.Fatal("large amount missing")
		}
		status, raw, headers = call("GET", strings.Replace(path, "format=xlsx", "format=csv", 1), nil)
		if status != 200 || !strings.HasPrefix(string(raw), "\ufeff") || !strings.Contains(headers.Get("Content-Type"), "text/csv") {
			t.Fatal(status, headers)
		}
		status, raw, headers = call("GET", strings.Replace(path, "format=xlsx", "format=print", 1), nil)
		if status != 200 || strings.Contains(string(raw), "<script>alert(1)") || !strings.Contains(string(raw), "9.007.199.254.740.993") || !strings.Contains(string(raw), "@page") {
			t.Fatal(status, string(raw))
		}
	}
	if status, b, _ := call("GET", "/documents/export?period=2026-10&format=pdf", nil); status != 400 {
		t.Fatal(status, string(b))
	}
	if status, _, _ := call("GET", "/reports/export?period=2026-10&format=xlsx&revision=0", nil); status != 409 {
		t.Fatal(status)
	}
	if status, _, _ := call("GET", "/catalog/export?period=2026-10&catalog=accounts&format=xlsx&revision=0", nil); status != 409 {
		t.Fatal(status)
	}
	var after int64
	db.Read.QueryRow("SELECT revision FROM meta").Scan(&after)
	if after != revision {
		t.Fatal("exports changed revision")
	}
	if status, b, _ := call("POST", "/users", map[string]any{"id": "export-viewer", "username": "export-viewer", "password": "Viewer-export-only-2026", "name": "Chỉ xem", "role": "viewer", "active": true}); status != 200 {
		t.Fatal(status, string(b))
	}
	call("POST", "/auth/logout", nil)
	call("POST", "/auth/login", map[string]string{"username": "export-viewer", "password": "Viewer-export-only-2026"})
	for _, path := range []string{"/documents/export?period=2026-10&format=xlsx", "/reports/export?period=2026-10&format=xlsx", "/catalog/export?period=2026-10&catalog=accounts&format=xlsx"} {
		if status, b, _ := call("GET", path, nil); status != 200 {
			t.Fatal(path, status, string(b))
		}
	}
}
