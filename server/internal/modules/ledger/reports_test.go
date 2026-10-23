package ledger

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func TestExportProtectsMetadataAndDoesNotMutateRows(t *testing.T) {
	var output bytes.Buffer
	rows := [][]string{{"\t=SUM(A1)", "9007199254740993 VND", "Tên"}}
	err := ExportRows(&output, Context{CompanyName: " =HYPERLINK(1)", Period: "2026-10", Revision: 1}, []string{"@header", "Tiền", "Tên"}, rows)
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(output.String(), "\ufeff")))
	reader.FieldsPerRecord = -1
	result, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if result[0][0] != "' =HYPERLINK(1)" || result[1][0] != "'@header" || result[2][0] != "'\t=SUM(A1)" || result[2][1] != "9007199254740993 VND" || rows[0][0] != "\t=SUM(A1)" {
		t.Fatal(result, rows)
	}
}

func TestCatalogExportFilterMatchesVietnameseAndRoles(t *testing.T) {
	accounts := []Account{{ID: "parent", Code: "112", Name: "Tiền gửi"}, {ID: "child", Code: "00001", Name: "Tiền mặt"}}
	if rows := CatalogAccounts(accounts, "tien mat"); len(rows) != 1 || rows[0].ID != "child" {
		t.Fatal(rows)
	}
	partners := []Partner{{ID: "customer", Name: "Khách thử", Role: "customer"}, {ID: "supplier", Name: "Nhà cung cấp", Role: "supplier"}, {ID: "both", Code: "00002", Name: "Đối tác", Role: "both"}}
	if rows := CatalogPartners(partners, "", "supplier"); len(rows) != 2 || rows[1].Code != "00002" {
		t.Fatal(rows)
	}
	if rows := CatalogPartners(partners, "doi tac", "customer"); len(rows) != 1 || rows[0].ID != "both" {
		t.Fatal(rows)
	}
}
