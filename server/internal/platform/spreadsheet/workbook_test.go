package spreadsheet

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestWorkbookPreservesExactRawValuesAndFormulaLocations(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellStr("Sheet1", "A1", "Mã")
	f.SetCellStr("Sheet1", "B1", "Tiền")
	f.SetCellStr("Sheet1", "C1", "Công thức")
	f.SetCellStr("Sheet1", "A3", "000001")
	f.SetCellStr("Sheet1", "B3", "9007199254740993")
	f.SetCellFormula("Sheet1", "C3", "1+2")
	f.SetCellDefault("Sheet1", "D3", "9007199254740993")
	f.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: ptr(true)})
	f.NewSheet("Khác")
	f.SetCellStr("Khác", "A1", "Thông tin")
	raw, e := f.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	book, e := Read("nguon.xlsx", raw.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	if !book.Date1904 || len(book.Sheets) != 2 || len(book.Sheets[0].Rows) != 3 {
		t.Fatal(book)
	}
	row := book.Sheets[0].Rows[2]
	if row[0].Value != "000001" || row[1].Value != "9007199254740993" || row[3].Value != "9007199254740993" || !row[2].Formula {
		t.Fatal(row)
	}
	if row[3].Kind != "number" || row[1].Kind != "text" {
		t.Fatal(row)
	}
}
func ptr[T any](value T) *T { return &value }
func TestExactNumberFormatsRejectAmbiguity(t *testing.T) {
	for _, c := range []struct {
		raw, format, want string
		numeric           bool
	}{{"9.007.199.254.740.993", "vi", "9007199254740993", false}, {"9,007,199,254,740,993", "en", "9007199254740993", false}, {"9007199254740993", "plain", "9007199254740993", false}, {"9.007199254740993e15", "vi", "9007199254740993", true}, {"1.234,50", "vi", "1234.5", false}, {"0.000001", "plain", "0.000001", false}, {"-0.00", "plain", "0", false}, {"1e-3", "plain", "0.001", true}} {
		got, e := Number(c.raw, c.format, c.numeric)
		if e != nil || got != c.want {
			t.Fatalf("%+v got=%s err=%v", c, got, e)
		}
	}
	for _, raw := range []string{"1.23.456", "1,2,3", "1.234,56.7", "NaN", "=1+2", "1e1000000000"} {
		if _, e := Number(raw, "vi", false); e == nil {
			t.Fatal(raw)
		}
	}
	if _, e := Number("1,234", "plain", false); e == nil {
		t.Fatal("guessed grouping")
	}
}
func TestCalendarSerialAndStrictDates(t *testing.T) {
	for _, c := range []struct {
		value, kind, format, want string
		date1904                  bool
	}{{"2026-10-22T00:00:00", "date", "dmy", "2026-10-22", false}, {"1", "number", "dmy", "1900-01-01", false}, {"59", "number", "dmy", "1900-02-28", false}, {"61", "number", "dmy", "1900-03-01", false}, {"0", "number", "dmy", "1904-01-01", true}, {"29/02/2024", "text", "dmy", "2024-02-29", false}, {"2026-10-22", "text", "iso", "2026-10-22", false}} {
		got, e := Date(Cell{Value: c.value, Kind: c.kind}, c.format, c.date1904)
		if e != nil || got != c.want {
			t.Fatalf("%+v got=%s err=%v", c, got, e)
		}
	}
	for _, c := range []Cell{{Value: "60", Kind: "number"}, {Value: "1.5", Kind: "number"}, {Value: "31/04/2026", Kind: "text"}, {Value: "29/02/2026", Kind: "text"}} {
		if _, e := Date(c, "dmy", false); e == nil {
			t.Fatal(c)
		}
	}
}
func TestWorkbookLimitsCSVAndTextOnlyExports(t *testing.T) {
	if _, e := Read("old.xls", []byte("legacy")); e == nil {
		t.Fatal("accepted xls")
	}
	if _, e := Read("fake.xlsx", []byte("broken")); e == nil {
		t.Fatal("accepted invalid zip")
	}
	if _, e := Read("bad.csv", []byte{255}); e == nil {
		t.Fatal("accepted encoding")
	}
	if _, e := Read("large.csv", bytes.Repeat([]byte("a"), MaxFileSize+1)); e == nil {
		t.Fatal("accepted oversized")
	}
	book, e := Read("test.csv", []byte("\ufeffMã,Tên\n000001,\"Tên, có phẩy\"\n"))
	if e != nil || book.Sheets[0].Rows[1][0].Value != "000001" {
		t.Fatal(book, e)
	}
	raw, e := Template("Mẫu", []string{"Mã", "Tiền"}, []string{"Chỉ nhập giá trị"})
	if e != nil {
		t.Fatal(e)
	}
	f, e := excelize.OpenReader(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if v, _ := f.GetCellValue("DuLieu", "B1"); v != "Tiền" {
		t.Fatal(v)
	}
	style, _ := f.GetCellStyle("DuLieu", "A2")
	options, _ := f.GetStyle(style)
	if options.NumFmt != 49 {
		t.Fatal(options)
	}
	report, e := ErrorReport([][]string{{"1", "Mã", "INVALID", "=HYPERLINK(\"bad\")"}})
	if e != nil {
		t.Fatal(e)
	}
	r, e := excelize.OpenReader(bytes.NewReader(report))
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if formula, _ := r.GetCellFormula("KiemTraLoi", "D2"); formula != "" {
		t.Fatal("export executed formula")
	}
}

func TestWorkbookRejectsDuplicatePhysicalCoordinates(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellStr("Sheet1", "A1", "Mã")
	f.SetCellStr("Sheet1", "A2", "0001")
	raw, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`<row r="2"><c r="A2" t="inlineStr"><is><t>Khác</t></is></c></row>`, `<row r="3"><c r="A2" t="inlineStr"><is><t>Khác</t></is></c></row>`} {
		source, _ := zip.NewReader(bytes.NewReader(raw.Bytes()), int64(raw.Len()))
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, entry := range source.File {
			reader, _ := entry.Open()
			data, _ := io.ReadAll(reader)
			reader.Close()
			if entry.Name == "xl/worksheets/sheet1.xml" {
				data = []byte(strings.Replace(string(data), "</sheetData>", extra+"</sheetData>", 1))
			}
			out, _ := writer.Create(entry.Name)
			out.Write(data)
		}
		writer.Close()
		if _, err = Read("duplicate.xlsx", buffer.Bytes()); err == nil {
			t.Fatal("accepted ambiguous coordinates")
		}
	}
}
