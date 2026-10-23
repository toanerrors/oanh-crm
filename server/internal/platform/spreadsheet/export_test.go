package spreadsheet

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExportXLSXExactTypedCellsAndLayout(t *testing.T) {
	headers := []string{"Mã", "Diễn giải", "Tiền", "Số lớn", "Âm", "Giới hạn", "Thập phân", "Chưa nhập", "Không", "Dài", "30 chữ số"}
	row := []string{"00009876", "=HYPERLINK(\"https://example.invalid\",\"test\")", "5000000", "9007199254740993", "-999999999999999", "-9223372036854775807", "123.45", "", "0", "1000000000000000", "999999999999999999999999999999"}
	money := map[int]bool{}
	for col := 2; col < len(headers); col++ {
		money[col] = true
	}
	book := ExportBook{Title: "Sổ thử", Company: "=2+2", Period: "2026-10", Revision: 7, Tables: []ExportTable{{Name: "Chứng từ", Headers: headers, Rows: [][]string{row}, MoneyColumns: money, Total: row}}}
	raw, err := ExportXLSX(context.Background(), book)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "export.xlsx")
	if err = f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	f.Close()
	f, err = excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for col, want := range row {
		cell, _ := excelize.CoordinatesToCellName(col+1, 7)
		value, e := f.GetCellValue("Chứng từ", cell, excelize.Options{RawCellValue: true})
		if e != nil || value != want {
			t.Fatalf("%s: %q != %q (%v)", cell, value, want, e)
		}
		formula, e := f.GetCellFormula("Chứng từ", cell)
		if e != nil || formula != "" {
			t.Fatalf("formula %s: %q %v", cell, formula, e)
		}
		kind, e := f.GetCellType("Chứng từ", cell)
		if e != nil {
			t.Fatal(e)
		}
		numeric := col == 2 || col == 4 || col == 8
		if want != "" && (kind == excelize.CellTypeNumber || kind == excelize.CellTypeUnset) != numeric {
			t.Fatalf("type %s: %v, numeric=%v", cell, kind, numeric)
		}
	}
	pane, err := f.GetPanes("Chứng từ")
	if err != nil || !pane.Freeze || pane.YSplit != 6 || pane.TopLeftCell != "A7" {
		t.Fatal(pane, err)
	}
	tables, err := f.GetTables("Chứng từ")
	if err != nil || len(tables) != 1 || tables[0].Range != "A6:K7" {
		t.Fatal(tables, err)
	}
	layout, err := f.GetPageLayout("Chứng từ")
	if err != nil || layout.Orientation == nil || *layout.Orientation != "landscape" {
		t.Fatal(layout, err)
	}
	value, err := f.GetCellValue("Thông tin", "A2")
	if err != nil || value != "=2+2" {
		t.Fatal(value, err)
	}
	if formula, _ := f.GetCellFormula("Thông tin", "A2"); formula != "" {
		t.Fatal(formula)
	}
	for _, name := range f.GetDefinedName() {
		if name.Name == "_xlnm.Print_Titles" {
			return
		}
	}
	t.Fatal("missing repeated print headers")
}
func TestExportXLSXLimitsCancellationAndEmptyData(t *testing.T) {
	valid := ExportBook{Tables: []ExportTable{{Name: "Trống", Headers: []string{"Mã", "Số tiền"}}}}
	if _, err := ExportXLSX(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	for _, book := range []ExportBook{{}, {Tables: []ExportTable{{Name: "Quá nhiều", Headers: []string{"Mã"}, Rows: make([][]string, MaxExportRows+1)}}}, {Tables: []ExportTable{{Name: "Dài", Headers: []string{"Mã"}, Rows: [][]string{{strings.Repeat("á", 32768)}}}}}} {
		if _, err := ExportXLSX(context.Background(), book); err == nil {
			t.Fatal("accepted invalid export")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExportXLSX(ctx, valid); err != context.Canceled {
		t.Fatal(err)
	}
}
