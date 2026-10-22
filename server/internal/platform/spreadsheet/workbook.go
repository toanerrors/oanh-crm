package spreadsheet

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math/big"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"oanh.local/accounting/internal/accounting"
)

const MaxFileSize = 20 << 20
const MaxWorkbookSize = 4 << 20
const MaxRows = 5050
const MaxColumns = 128

type Cell struct {
	Value   string `json:"value"`
	Kind    string `json:"kind,omitempty"`
	Formula bool   `json:"formula,omitempty"`
}
type Sheet struct {
	Name string   `json:"name"`
	Rows [][]Cell `json:"rows"`
}
type Workbook struct {
	Sheets   []Sheet `json:"sheets"`
	Date1904 bool    `json:"date1904"`
}

func invalid(message string) error { return accounting.Fail("INVALID_WORKBOOK", message) }

func Read(name string, data []byte) (Workbook, error) {
	book := Workbook{Sheets: []Sheet{}}
	if len(data) == 0 || len(data) > MaxFileSize {
		return book, invalid("Chọn tệp từ 1 byte đến 20 MB.")
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".csv":
		if !utf8.Valid(data) {
			return book, invalid("CSV phải dùng mã hóa UTF-8.")
		}
		r := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff")))
		r.FieldsPerRecord = -1
		sheet := Sheet{Name: "CSV", Rows: [][]Cell{}}
		for {
			values, e := r.Read()
			if e == io.EOF {
				break
			}
			if e != nil {
				return book, invalid("CSV có dòng sai cấu trúc hoặc dấu nháy.")
			}
			if len(sheet.Rows) >= MaxRows || len(values) > MaxColumns {
				return book, invalid("Tệp vượt 5.050 dòng hoặc 128 cột.")
			}
			row := make([]Cell, len(values))
			for i, value := range values {
				row[i] = Cell{Value: value, Kind: "text"}
			}
			sheet.Rows = append(sheet.Rows, row)
		}
		book.Sheets = append(book.Sheets, sheet)
	case ".xlsx":
		z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if e != nil {
			return book, invalid("Không đọc được cấu trúc tệp XLSX.")
		}
		var expanded uint64
		for _, entry := range z.File {
			expanded += entry.UncompressedSize64
			if expanded > 64<<20 || entry.UncompressedSize64 > 32<<20 {
				return book, invalid("Tệp Excel vượt giới hạn giải nén 64 MB.")
			}
			p := strings.ToLower(entry.Name)
			if strings.HasPrefix(p, "xl/worksheets/") && strings.HasSuffix(p, ".xml") {
				if e := validateSheetCoordinates(entry); e != nil {
					return book, e
				}
			}
			if strings.Contains(p, "vbaproject") || strings.HasPrefix(p, "xl/externallinks/") {
				return book, invalid("Tệp có macro hoặc liên kết ngoài. Hãy lưu bản XLSX chỉ chứa dữ liệu cần nhập.")
			}
		}
		f, e := excelize.OpenReader(bytes.NewReader(data), excelize.Options{RawCellValue: true, UnzipSizeLimit: 64 << 20, UnzipXMLSizeLimit: 8 << 20})
		if e != nil {
			return book, invalid("Không mở được XLSX. Kiểm tra định dạng và mật khẩu của tệp.")
		}
		defer f.Close()
		props, e := f.GetWorkbookProps()
		if e != nil {
			return book, invalid("Không đọc được hệ ngày Excel.")
		}
		book.Date1904 = props.Date1904 != nil && *props.Date1904
		names := f.GetSheetList()
		if len(names) == 0 || len(names) > 20 {
			return book, invalid("Tệp Excel phải có từ 1 đến 20 sheet.")
		}
		cells := 0
		size := 0
		for _, name := range names {
			sheet := Sheet{Name: name, Rows: [][]Cell{}}
			rows, e := f.Rows(name)
			if e != nil {
				return book, invalid("Không đọc được sheet.")
			}
			for rows.Next() {
				if len(sheet.Rows) >= MaxRows {
					rows.Close()
					return book, invalid("Mỗi sheet tối đa 5.050 dòng, gồm dòng tiêu đề.")
				}
				values, e := rows.Columns(excelize.Options{RawCellValue: true})
				if e != nil {
					rows.Close()
					return book, invalid("Có ô Excel không đọc được.")
				}
				if len(values) > MaxColumns {
					rows.Close()
					return book, invalid("Mỗi sheet tối đa 128 cột.")
				}
				row := make([]Cell, len(values))
				for col, value := range values {
					axis, _ := excelize.CoordinatesToCellName(col+1, len(sheet.Rows)+1)
					kind, e := f.GetCellType(name, axis)
					if e != nil {
						rows.Close()
						return book, invalid("Không đọc được kiểu dữ liệu ô.")
					}
					formula, e := f.GetCellFormula(name, axis)
					if e != nil {
						rows.Close()
						return book, invalid("Không đọc được thông tin công thức.")
					}
					cell := Cell{Value: value, Kind: "text", Formula: formula != ""}
					switch kind {
					case excelize.CellTypeNumber, excelize.CellTypeUnset:
						cell.Kind = "number"
					case excelize.CellTypeDate:
						cell.Kind = "date"
					case excelize.CellTypeError:
						cell.Kind = "error"
					case excelize.CellTypeBool:
						cell.Kind = "bool"
					}
					if len(value) > 5000 {
						rows.Close()
						return book, invalid("Một ô dữ liệu vượt 5.000 byte.")
					}
					cells++
					size += len(value)
					if cells > 100000 || size > MaxWorkbookSize {
						rows.Close()
						return book, invalid("Tệp vượt 100.000 ô hoặc 4 MB dữ liệu đọc được.")
					}
					row[col] = cell
				}
				sheet.Rows = append(sheet.Rows, row)
			}
			e = rows.Error()
			rows.Close()
			if e != nil {
				return book, invalid("Không đọc hết được dữ liệu sheet.")
			}
			book.Sheets = append(book.Sheets, sheet)
		}
	default:
		return book, invalid("Hỗ trợ XLSX và CSV UTF-8. Với XLS cũ, hãy lưu lại thành XLSX trong Excel.")
	}
	raw, e := json.Marshal(book)
	if e != nil {
		return book, e
	}
	if len(raw) > MaxWorkbookSize {
		return book, invalid("Dữ liệu sau khi đọc vượt 4 MB. Hãy chia tệp nhỏ hơn.")
	}
	return book, nil
}

func validateSheetCoordinates(entry *zip.File) error {
	reader, err := entry.Open()
	if err != nil {
		return invalid("Không đọc được sheet Excel.")
	}
	defer reader.Close()
	decoder := xml.NewDecoder(io.LimitReader(reader, 32<<20))
	row, column := 0, 0
	inData, inRow := false, false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return invalid("Cấu trúc XML của sheet không hợp lệ.")
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "sheetData" {
				inData = true
			}
			if !inData {
				continue
			}
			if value.Name.Local == "row" {
				next := row + 1
				for _, attr := range value.Attr {
					if attr.Name.Local == "r" {
						next, err = strconv.Atoi(attr.Value)
						if err != nil {
							return invalid("Số dòng Excel không hợp lệ.")
						}
					}
				}
				if inRow || next <= row || next > MaxRows {
					return invalid("Dòng Excel trùng, sai thứ tự hoặc vượt 5.050 dòng.")
				}
				row, column, inRow = next, 0, true
			}
			if value.Name.Local == "c" {
				if !inRow {
					return invalid("Ô Excel không nằm trong dòng dữ liệu.")
				}
				next := column + 1
				for _, attr := range value.Attr {
					if attr.Name.Local == "r" {
						var cellRow int
						next, cellRow, err = excelize.CellNameToCoordinates(attr.Value)
						if err != nil || cellRow != row {
							return invalid("Địa chỉ ô Excel không khớp dòng dữ liệu.")
						}
					}
				}
				if next <= column || next > MaxColumns {
					return invalid("Ô Excel trùng, sai thứ tự hoặc vượt 128 cột.")
				}
				column = next
			}
		case xml.EndElement:
			if value.Name.Local == "row" {
				inRow = false
			}
			if value.Name.Local == "sheetData" {
				inData = false
			}
		}
	}
	return nil
}

var numberPattern = regexp.MustCompile(`^([+-]?)([0-9]+)(?:\.([0-9]+))?(?:[eE]([+-]?[0-9]+))?$`)

func Number(value, format string, numeric bool) (string, error) {
	value = strings.TrimSpace(value)
	if !numeric && format != "plain" {
		group, decimal := ",", "."
		if format == "vi" {
			group, decimal = ".", ","
		} else if format != "en" {
			return "", invalid("Chọn định dạng số hợp lệ.")
		}
		pieces := strings.Split(value, decimal)
		if len(pieces) > 2 {
			return "", invalid("Số không đúng định dạng đã chọn.")
		}
		integer := pieces[0]
		sign := ""
		if strings.HasPrefix(integer, "-") || strings.HasPrefix(integer, "+") {
			sign, integer = integer[:1], integer[1:]
		}
		if strings.Contains(integer, group) {
			chunks := strings.Split(integer, group)
			if len(chunks[0]) < 1 || len(chunks[0]) > 3 {
				return "", invalid("Nhóm phân cách hàng nghìn không hợp lệ.")
			}
			for _, p := range chunks[1:] {
				if len(p) != 3 {
					return "", invalid("Nhóm phân cách hàng nghìn phải có 3 chữ số.")
				}
			}
			integer = strings.Join(chunks, "")
		}
		value = sign + integer
		if len(pieces) == 2 {
			value += "." + pieces[1]
		}
	}
	match := numberPattern.FindStringSubmatch(value)
	if match == nil || len(value) > 100 {
		return "", invalid("Nhập số đúng định dạng; không tự bỏ dấu phân cách không rõ nghĩa.")
	}
	exponent := 0
	if match[4] != "" {
		var e error
		exponent, e = strconv.Atoi(match[4])
		if e != nil || exponent < -30 || exponent > 30 {
			return "", invalid("Số mũ vượt giới hạn.")
		}
	}
	digits := strings.TrimLeft(match[2]+match[3], "0")
	if digits == "" {
		return "0", nil
	}
	scale := len(match[3]) - exponent
	if scale < 0 {
		digits += strings.Repeat("0", -scale)
		scale = 0
	}
	if scale > 0 {
		if len(digits) <= scale {
			digits = strings.Repeat("0", scale-len(digits)+1) + digits
		}
		at := len(digits) - scale
		digits = digits[:at] + "." + digits[at:]
		digits = strings.TrimRight(strings.TrimRight(digits, "0"), ".")
	}
	if len(digits) > 60 {
		return "", invalid("Số vượt giới hạn độ dài.")
	}
	if match[1] == "-" {
		digits = "-" + digits
	}
	return digits, nil
}

func Date(cell Cell, format string, date1904 bool) (string, error) {
	value := strings.TrimSpace(cell.Value)
	if cell.Kind == "number" {
		raw, e := Number(value, "plain", true)
		if e != nil {
			return "", e
		}
		serial, ok := new(big.Int).SetString(raw, 10)
		if !ok || !serial.IsInt64() {
			return "", invalid("Ngày Excel phải là ngày nguyên, không có phần giờ.")
		}
		days := serial.Int64()
		if days < 0 || days > 2958465 || !date1904 && days == 60 {
			return "", invalid("Số ngày Excel không hợp lệ.")
		}
		base := time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)
		if date1904 {
			base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
		} else if days > 60 {
			days--
		}
		value = base.AddDate(0, 0, int(days)).Format("2006-01-02")
	} else {
		layout := "2006-01-02"
		if format == "dmy" {
			layout = "02/01/2006"
		} else if format != "iso" {
			return "", invalid("Chọn định dạng ngày hợp lệ.")
		}
		if cell.Kind == "date" {
			layout = "2006-01-02"
			value = strings.TrimSuffix(strings.TrimSuffix(value, "T00:00:00Z"), "T00:00:00")
		}
		date, e := time.Parse(layout, value)
		if e != nil {
			return "", invalid("Ngày không đúng định dạng đã chọn.")
		}
		value = date.Format("2006-01-02")
	}
	if !accounting.Date(value) {
		return "", invalid("Ngày ngoài phạm vi cho phép.")
	}
	return value, nil
}

func Template(title string, headers, notes []string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", "DuLieu")
	f.NewSheet("HuongDan")
	style, e := f.NewStyle(&excelize.Style{NumFmt: 49, Font: &excelize.Font{Size: 13}, Alignment: &excelize.Alignment{Vertical: "center"}})
	if e != nil {
		return nil, e
	}
	last, _ := excelize.ColumnNumberToName(len(headers))
	if e = f.SetColStyle("DuLieu", "A:"+last, style); e != nil {
		return nil, e
	}
	f.SetColWidth("DuLieu", "A", last, 24)
	for i, value := range headers {
		axis, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellStr("DuLieu", axis, value)
	}
	heading, e := f.NewStyle(&excelize.Style{NumFmt: 49, Font: &excelize.Font{Bold: true, Color: "FFFFFF", Size: 13}, Fill: excelize.Fill{Type: "pattern", Color: []string{"167D6A"}, Pattern: 1}})
	if e != nil {
		return nil, e
	}
	f.SetCellStyle("DuLieu", "A1", last+"1", heading)
	f.SetPanes("DuLieu", &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	f.SetColWidth("HuongDan", "A", "A", 115)
	f.SetCellStr("HuongDan", "A1", title)
	for i, note := range notes {
		f.SetCellStr("HuongDan", fmt.Sprintf("A%d", i+3), note)
	}
	b, e := f.WriteToBuffer()
	if e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}

func ErrorReport(rows [][]string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", "KiemTraLoi")
	style, err := f.NewStyle(&excelize.Style{NumFmt: 49, Font: &excelize.Font{Size: 13}})
	if err != nil {
		return nil, err
	}
	if err = f.SetColStyle("KiemTraLoi", "A:D", style); err != nil {
		return nil, err
	}
	f.SetColWidth("KiemTraLoi", "A", "A", 12)
	f.SetColWidth("KiemTraLoi", "B", "C", 28)
	f.SetColWidth("KiemTraLoi", "D", "D", 100)
	all := append([][]string{{"Dòng Excel", "Cột", "Mã lỗi", "Lý do cần sửa trong tệp gốc"}}, rows...)
	for r, row := range all {
		for c, value := range row {
			axis, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if err = f.SetCellStr("KiemTraLoi", axis, value); err != nil {
				return nil, err
			}
		}
	}
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
