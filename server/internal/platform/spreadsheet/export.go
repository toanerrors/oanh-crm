package spreadsheet

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"oanh.local/accounting/internal/accounting"
)

const MaxExportRows = 10000

type ExportTable struct {
	Name         string
	Headers      []string
	Rows         [][]string
	MoneyColumns map[int]bool
	Total        []string
}

type ExportBook struct {
	Title, Company, Period string
	Revision               int64
	Metadata               [][]string
	Tables                 []ExportTable
}

var exportInteger = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

// Excel numbers retain at most 15 significant decimal digits. Larger values stay text.
func exportAmount(value string) any {
	digits := strings.TrimPrefix(value, "-")
	if exportInteger.MatchString(value) && len(digits) <= 15 {
		n, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return n
		}
	}
	return value
}

func ExportXLSX(ctx context.Context, book ExportBook) ([]byte, error) {
	if len(book.Tables) == 0 || len(book.Tables) > 16 {
		return nil, accounting.Fail("EXPORT_LIMIT", "Số bảng xuất không hợp lệ.")
	}
	f := excelize.NewFile()
	defer f.Close()
	title, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 16, Color: "176B57"}})
	if err != nil {
		return nil, err
	}
	header, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"176B57"}}, Alignment: &excelize.Alignment{WrapText: true, Vertical: "center"}})
	if err != nil {
		return nil, err
	}
	text, err := f.NewStyle(&excelize.Style{NumFmt: 49, Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	if err != nil {
		return nil, err
	}
	number, err := f.NewStyle(&excelize.Style{NumFmt: 3, Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "top"}})
	if err != nil {
		return nil, err
	}
	exact, err := f.NewStyle(&excelize.Style{NumFmt: 49, Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "top", WrapText: true}, Font: &excelize.Font{Color: "8A5418"}})
	if err != nil {
		return nil, err
	}
	total, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E1F1EA"}}, Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	if err != nil {
		return nil, err
	}
	totalNumber, err := f.NewStyle(&excelize.Style{NumFmt: 3, Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E1F1EA"}}, Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "top"}})
	if err != nil {
		return nil, err
	}
	totalExact, err := f.NewStyle(&excelize.Style{NumFmt: 49, Font: &excelize.Font{Bold: true, Color: "8A5418"}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E1F1EA"}}, Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "top", WrapText: true}})
	if err != nil {
		return nil, err
	}
	if err = f.SetSheetName("Sheet1", "Thông tin"); err != nil {
		return nil, err
	}
	metadata := [][]string{{book.Title}, {book.Company}, {"Kỳ / phạm vi", book.Period}, {"Phiên bản dữ liệu", strconv.FormatInt(book.Revision, 10)}, {"Đơn vị tiền", "VND"}, {"Độ chính xác", "Mã luôn là văn bản. Số nguyên tối đa 15 chữ số là ô số; số lớn hơn và số thập phân giữ dạng văn bản (màu nâu) để không làm sai số. Dòng tổng do máy chủ cung cấp; không phải công thức Excel."}}
	metadata = append(metadata, book.Metadata...)
	for i, row := range metadata {
		for j, value := range row {
			if utf8.RuneCountInString(value) > 32767 {
				return nil, accounting.Fail("EXPORT_LIMIT", "Nội dung ô vượt giới hạn Excel.")
			}
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			if err = f.SetCellStr("Thông tin", cell, value); err != nil {
				return nil, err
			}
			if err = f.SetCellStyle("Thông tin", cell, cell, text); err != nil {
				return nil, err
			}
		}
	}
	if err = f.SetCellStyle("Thông tin", "A1", "B1", title); err != nil {
		return nil, err
	}
	if err = f.SetColWidth("Thông tin", "A", "A", 28); err != nil {
		return nil, err
	}
	if err = f.SetColWidth("Thông tin", "B", "B", 110); err != nil {
		return nil, err
	}
	if err = f.SetRowHeight("Thông tin", 6, 54); err != nil {
		return nil, err
	}
	for index, table := range book.Tables {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if len(table.Headers) < 1 || len(table.Headers) > MaxColumns || len(table.Rows) > MaxExportRows {
			return nil, accounting.Fail("EXPORT_LIMIT", "Excel hỗ trợ tối đa 10.000 dòng mỗi bảng; thu hẹp bộ lọc hoặc chọn CSV.")
		}
		if _, err = f.NewSheet(table.Name); err != nil {
			return nil, err
		}
		landscape, paper, width, height := "landscape", 9, 1, 0
		if err = f.SetPageLayout(table.Name, &excelize.PageLayoutOptions{Orientation: &landscape, Size: &paper, FitToWidth: &width, FitToHeight: &height}); err != nil {
			return nil, err
		}
		if err = f.SetHeaderFooter(table.Name, &excelize.HeaderFooterOptions{OddFooter: "&LOanh · VND&CTrang &P / &N"}); err != nil {
			return nil, err
		}
		sw, e := f.NewStreamWriter(table.Name)
		if e != nil {
			return nil, e
		}
		if err = sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 6, TopLeftCell: "A7", ActivePane: "bottomLeft"}); err != nil {
			return nil, err
		}
		for col, h := range table.Headers {
			size := 22
			lower := strings.ToLower(h)
			if strings.Contains(lower, "diễn giải") || strings.Contains(lower, "tên") || strings.Contains(lower, "chỉ tiêu") || strings.Contains(lower, "thuyết minh") {
				size = 48
			}
			if err = sw.SetColWidth(col+1, col+1, float64(size)); err != nil {
				return nil, err
			}
		}
		last, _ := excelize.ColumnNumberToName(len(table.Headers))
		for i, value := range []string{book.Title, book.Company, "Phạm vi: " + book.Period + " · Phiên bản " + strconv.FormatInt(book.Revision, 10), fmt.Sprintf("Toàn bộ tập lọc · %d dòng · VND", len(table.Rows))} {
			style := text
			if i == 0 {
				style = title
			}
			if err = sw.SetRow(fmt.Sprintf("A%d", i+1), []any{excelize.Cell{StyleID: style, Value: value}}, excelize.RowOpts{Height: 24}); err != nil {
				return nil, err
			}
			if len(table.Headers) > 1 {
				if err = sw.MergeCell(fmt.Sprintf("A%d", i+1), fmt.Sprintf("%s%d", last, i+1)); err != nil {
					return nil, err
				}
			}
		}
		headers := make([]any, len(table.Headers))
		for i, h := range table.Headers {
			headers[i] = excelize.Cell{StyleID: header, Value: h}
		}
		if err = sw.SetRow("A6", headers, excelize.RowOpts{Height: 32}); err != nil {
			return nil, err
		}
		writeRow := func(rowNumber int, row []string, isTotal bool) error {
			if len(row) > len(table.Headers) {
				return accounting.Fail("EXPORT_LIMIT", "Số cột dữ liệu xuất không khớp.")
			}
			cells := make([]any, len(row))
			for col, value := range row {
				if utf8.RuneCountInString(value) > 32767 {
					return accounting.Fail("EXPORT_LIMIT", "Nội dung ô vượt giới hạn Excel.")
				}
				var v any = value
				style := text
				if table.MoneyColumns[col] {
					v = exportAmount(value)
					style = number
					if _, ok := v.(string); ok {
						style = exact
					}
				}
				if isTotal {
					style = total
					if table.MoneyColumns[col] {
						style = totalNumber
						if _, ok := v.(string); ok {
							style = totalExact
						}
					}
				}
				cells[col] = excelize.Cell{StyleID: style, Value: v}
			}
			return sw.SetRow(fmt.Sprintf("A%d", rowNumber), cells)
		}
		for i, row := range table.Rows {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			if err = writeRow(i+7, row, false); err != nil {
				return nil, err
			}
		}
		if len(table.Total) > 0 {
			if err = writeRow(len(table.Rows)+7, table.Total, true); err != nil {
				return nil, err
			}
		}
		if len(table.Rows) > 0 {
			stripes := true
			if err = sw.AddTable(&excelize.Table{Range: fmt.Sprintf("A6:%s%d", last, len(table.Rows)+6), Name: fmt.Sprintf("OanhTable%d", index+1), StyleName: "TableStyleMedium4", ShowRowStripes: &stripes}); err != nil {
				return nil, err
			}
		}
		if err = sw.Flush(); err != nil {
			return nil, err
		}
		if err = f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Titles", RefersTo: fmt.Sprintf("'%s'!$1:$6", table.Name), Scope: table.Name}); err != nil {
			return nil, err
		}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	f.SetActiveSheet(1)
	var output bytes.Buffer
	if _, err = f.WriteTo(&output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
