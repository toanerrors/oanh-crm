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
