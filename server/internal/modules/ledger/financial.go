package ledger

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
)

//go:embed financial_template.json
var financialTemplate []byte

type FinancialRow struct {
	Code    string   `json:"code"`
	Name    string   `json:"name"`
	Level   int      `json:"level"`
	Formula []string `json:"formula"`
}
type FinancialSheet struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Rows []FinancialRow `json:"rows"`
}
type FinancialTemplate struct {
	Version       string           `json:"version"`
	Regime        string           `json:"regime"`
	EffectiveFrom string           `json:"effectiveFrom"`
	Source        string           `json:"source"`
	Sheets        []FinancialSheet `json:"sheets"`
}

func FinancialForms() FinancialTemplate {
	var t FinancialTemplate
	if err := json.Unmarshal(financialTemplate, &t); err != nil {
		panic(err)
	}
	return t
}

type FinancialValue struct {
	Current  string `json:"current"`
	Previous string `json:"previous"`
	Note     string `json:"note"`
}
type FinancialInput struct {
	ID              string                               `json:"id"`
	Version         int                                  `json:"version"`
	Template        string                               `json:"template"`
	From            string                               `json:"from"`
	To              string                               `json:"to"`
	PreviousFrom    string                               `json:"previousFrom"`
	PreviousTo      string                               `json:"previousTo"`
	PreparedBy      string                               `json:"preparedBy"`
	ChiefAccountant string                               `json:"chiefAccountant"`
	Director        string                               `json:"director"`
	PreparedDate    string                               `json:"preparedDate"`
	Address         string                               `json:"address"`
	Sheets          []string                             `json:"sheets"`
	Values          map[string]map[string]FinancialValue `json:"values"`
	Notes           string                               `json:"notes"`
	SourceRevision  int64                                `json:"sourceRevision"`
}
type FinancialIssue struct {
	Sheet   string `json:"sheet"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type FinancialReport struct {
	Input       FinancialInput                       `json:"input"`
	Computed    map[string]map[string]FinancialValue `json:"computed"`
	Issues      []FinancialIssue                     `json:"issues"`
	UpdatedAt   string                               `json:"updatedAt"`
	CompanyName string                               `json:"companyName"`
}

var financialEPS = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,29})(\.[0-9]{1,2})?$`)

var financialAmount = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,29})$`)

func financialDates(from, to string) bool {
	if !accounting.Date(from) || !accounting.Date(to) || from > to {
		return false
	}
	a, _ := time.Parse("2006-01-02", from)
	b, _ := time.Parse("2006-01-02", to)
	return b.Before(a.AddDate(1, 0, 0))
}
func CalculateFinancial(input FinancialInput) (FinancialReport, error) {
	out := FinancialReport{Input: input, Computed: map[string]map[string]FinancialValue{}, Issues: []FinancialIssue{}}
	t := FinancialForms()
	if !accounting.ID(input.ID) || input.Version < 0 || input.Template != t.Version || input.From < t.EffectiveFrom || !financialDates(input.From, input.To) || !financialDates(input.PreviousFrom, input.PreviousTo) || input.PreviousTo >= input.From || !accounting.Date(input.PreparedDate) || input.SourceRevision < 0 || input.From[5:] != "01-01" || input.To != input.From[:4]+"-12-31" || input.PreviousTo != input.PreviousFrom[:4]+"-12-31" || input.PreviousFrom[5:] != "01-01" {
		return out, accounting.Fail("VALIDATION", "Mẫu TT99 áp dụng từ năm tài chính 2026; ngày/kỳ/phiên bản báo cáo không hợp lệ (mỗi kỳ tối đa một năm).")
	}
	start, _ := time.Parse("2006-01-02", input.From)
	if input.PreviousFrom != start.AddDate(-1, 0, 0).Format("2006-01-02") {
		return out, accounting.Fail("VALIDATION", "Kỳ so sánh phải là năm liền trước.")
	}
	for _, text := range []string{input.PreparedBy, input.ChiefAccountant, input.Director, input.Address} {
		if len(text) > 500 {
			return out, accounting.Fail("VALIDATION", "Thông tin người ký hoặc địa chỉ quá dài.")
		}
	}
	if len(input.Notes) > 60000 {
		return out, accounting.Fail("VALIDATION", "Thuyết minh vượt 60.000 byte.")
	}
	selected := map[string]bool{}
	for _, id := range input.Sheets {
		if selected[id] {
			return out, accounting.Fail("VALIDATION", "Báo cáo được chọn trùng.")
		}
		selected[id] = true
	}
	if !selected["B01-DN"] || len(selected) > 4 || selected["B03-DN"] && selected["B03-DN-GT"] {
		return out, accounting.Fail("VALIDATION", "Phải chọn B01-DN; chỉ chọn một phương pháp lưu chuyển tiền tệ.")
	}
	known := map[string]bool{}
	for _, sheet := range t.Sheets {
		known[sheet.ID] = true
	}
	for id := range selected {
		if !known[id] {
			return out, accounting.Fail("VALIDATION", "Mẫu báo cáo không hợp lệ.")
		}
	}
	for id, values := range input.Values {
		if !selected[id] {
			return out, accounting.Fail("VALIDATION", "Dữ liệu thuộc báo cáo chưa chọn.")
		}
		var rows []FinancialRow
		for _, s := range t.Sheets {
			if s.ID == id {
				rows = s.Rows
			}
		}
		allowed := map[string]bool{}
		for _, row := range rows {
			allowed[row.Code] = len(row.Formula) == 0
		}
		for code, v := range values {
			if !allowed[code] || len(v.Note) > 300 {
				return out, accounting.Fail("DERIVED_FIELD", "Chỉ nhập chỉ tiêu chi tiết và thuyết minh tối đa 300 byte; tổng do server tính.")
			}
			pattern := financialAmount
			if id == "B02-DN" && (code == "70" || code == "71") {
				pattern = financialEPS
			}
			for _, value := range []string{v.Current, v.Previous} {
				if value != "" && (!pattern.MatchString(value) || value == "-0") {
					return out, accounting.Fail("VALIDATION", "Số báo cáo tối đa 30 chữ số nguyên; riêng lãi trên cổ phiếu tối đa 2 chữ số lẻ. Ô trống là chưa có dữ liệu.")
				}
			}
		}
	}
	issue := func(sheet, code, message string) {
		out.Issues = append(out.Issues, FinancialIssue{sheet, code, message})
	}
	for _, sheet := range t.Sheets {
		if !selected[sheet.ID] {
			continue
		}
		values := map[string]FinancialValue{}
		definitions := map[string]FinancialRow{}
		for _, row := range sheet.Rows {
			definitions[row.Code] = row
		}
		var compute func(string) FinancialValue
		compute = func(code string) FinancialValue {
			if v, ok := values[code]; ok {
				return v
			}
			row := definitions[code]
			v := input.Values[sheet.ID][code]
			if len(row.Formula) == 0 {
				if v.Current == "" || v.Previous == "" {
					issue(sheet.ID, code, "Chưa nhập đủ hai cột số liệu; nhập 0 nếu đã xác nhận không phát sinh.")
				}
			} else {
				sums := [2]*big.Int{new(big.Int), new(big.Int)}
				complete := [2]bool{true, true}
				for _, term := range row.Formula {
					child := compute(strings.TrimPrefix(term, "-"))
					for i, value := range []string{child.Current, child.Previous} {
						if value == "" {
							complete[i] = false
							continue
						}
						n, _ := new(big.Int).SetString(value, 10)
						if strings.HasPrefix(term, "-") {
							sums[i].Sub(sums[i], n)
						} else {
							sums[i].Add(sums[i], n)
						}
					}
				}
				v = FinancialValue{}
				if complete[0] {
					v.Current = sums[0].String()
				}
				if complete[1] {
					v.Previous = sums[1].String()
				}
			}
			negative := sheet.ID == "B01-DN" && strings.Contains(row.Name, "(*)")
			if sheet.ID == "B03-DN" {
				negative = strings.Contains("|02|03|04|05|07|21|23|25|32|34|35|36|", "|"+code+"|")
			}
			if sheet.ID == "B03-DN-GT" {
				negative = strings.Contains("|14|15|17|21|23|25|32|34|35|36|", "|"+code+"|")
			}
			if negative {
				for _, value := range []string{v.Current, v.Previous} {
					if value != "" {
						n, _ := new(big.Int).SetString(value, 10)
						if n.Sign() > 0 {
							issue(sheet.ID, code, "Chỉ tiêu giảm trừ hoặc dòng tiền chi phải trình bày số âm.")
							break
						}
					}
				}
			}
			values[code] = v
			return v
		}
		for _, row := range sheet.Rows {
			compute(row.Code)
		}
		out.Computed[sheet.ID] = values
	}
	if strings.TrimSpace(input.PreparedBy) == "" || strings.TrimSpace(input.ChiefAccountant) == "" || strings.TrimSpace(input.Director) == "" {
		issue("", "", "Chưa đủ người lập, kế toán trưởng và người đại diện theo pháp luật.")
	}
	if strings.TrimSpace(input.Address) == "" {
		issue("", "", "Chưa có địa chỉ đơn vị báo cáo.")
	}
	if strings.TrimSpace(input.Notes) == "" {
		issue("B09-DN", "", "Chưa bổ sung bản thuyết minh và các bảng chi tiết liên quan.")
	}
	compare := func(a, b FinancialValue, sheet, code, message string) {
		for i, pair := range [][2]string{{a.Current, b.Current}, {a.Previous, b.Previous}} {
			if pair[0] != "" && pair[1] != "" && pair[0] != pair[1] {
				issue(sheet, code, fmt.Sprintf("%s (%s).", message, []string{"cột kỳ này", "cột so sánh"}[i]))
			}
		}
	}
	compare(out.Computed["B01-DN"]["280"], out.Computed["B01-DN"]["440"], "B01-DN", "280", "Tổng tài sản khác tổng nguồn vốn")
	for _, id := range []string{"B03-DN", "B03-DN-GT"} {
		if !selected[id] {
			continue
		}
		compare(out.Computed[id]["70"], out.Computed["B01-DN"]["110"], id, "70", "Tiền cuối kỳ LCTT khác tiền và tương đương tiền B01")
	}
	if selected["B03-DN-GT"] && selected["B02-DN"] {
		compare(out.Computed["B03-DN-GT"]["01"], out.Computed["B02-DN"]["50"], "B03-DN-GT", "01", "Lợi nhuận trước thuế khác B02")
	}
	return out, nil
}
func (s *Service) SaveFinancial(ctx context.Context, cmd Command, input FinancialInput) (json.RawMessage, error) {
	if _, err := CalculateFinancial(input); err != nil {
		return nil, err
	}
	return s.Write(ctx, cmd, "financial.save", input, func(tx *sql.Tx) (any, error) {
		var old int
		err := tx.QueryRowContext(ctx, "SELECT version FROM financial_reports WHERE id=?", input.ID).Scan(&old)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if old != input.Version {
			return nil, accounting.Fail("VERSION_CONFLICT", "BCTC đã thay đổi. Mở lại bản đã lưu trước khi sửa.")
		}
		var revision int64
		var company string
		if err = tx.QueryRowContext(ctx, "SELECT revision,company_name FROM meta").Scan(&revision, &company); err != nil {
			return nil, err
		}
		if input.SourceRevision > revision {
			return nil, accounting.Fail("VERSION_CONFLICT", "Phiên bản nguồn không hợp lệ.")
		}
		if input.SourceRevision == revision {
			input.SourceRevision++
		}
		input.Version++
		out, err := CalculateFinancial(input)
		if err != nil {
			return nil, err
		}
		out.CompanyName = company
		out.UpdatedAt = sqlite.Now()
		b, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO financial_reports VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET version=excluded.version,from_date=excluded.from_date,to_date=excluded.to_date,updated_at=excluded.updated_at,actor_id=excluded.actor_id,data=excluded.data`, input.ID, input.Version, input.From, input.To, out.UpdatedAt, cmd.Actor, string(b))
		return out, err
	})
}
func (s *Service) FinancialReports(ctx context.Context, year, id string) ([]FinancialReport, error) {
	if id != "" && !accounting.ID(id) || id == "" && (!accounting.Period(year+"-01")) {
		return nil, accounting.Fail("VALIDATION", "Năm hoặc mã báo cáo không hợp lệ.")
	}
	tx, err := s.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var company string
	if err = tx.QueryRowContext(ctx, "SELECT company_name FROM meta").Scan(&company); err != nil {
		return nil, err
	}
	query := "SELECT data,updated_at FROM financial_reports WHERE from_date>=? AND from_date<? ORDER BY from_date DESC,updated_at DESC,id LIMIT 101"
	args := []any{year + "-01-01", year + "-12-32"}
	if id != "" {
		query = "SELECT data,updated_at FROM financial_reports WHERE id=?"
		args = []any{id}
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FinancialReport{}
	for rows.Next() {
		var raw, updated string
		if err = rows.Scan(&raw, &updated); err != nil {
			return nil, err
		}
		var input FinancialInput
		if err = json.Unmarshal([]byte(raw), &input); err != nil {
			return nil, err
		}
		report, e := CalculateFinancial(input)
		if e != nil {
			return nil, e
		}
		report.CompanyName = company
		report.UpdatedAt = updated
		out = append(out, report)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 100 {
		return nil, accounting.Fail("REPORT_LIMIT", "Có hơn 100 BCTC trong năm; giới hạn tra cứu đã đạt.")
	}
	return out, tx.Commit()
}

type FinancialSource struct {
	Context
	From string `json:"from"`
	To   string `json:"to"`
	Rows []Row  `json:"rows"`
}

func (s *Service) FinancialSource(ctx context.Context, from, to string) (FinancialSource, error) {
	out := FinancialSource{From: from, To: to, Rows: []Row{}}
	if !financialDates(from, to) {
		return out, accounting.Fail("VALIDATION", "Khoảng đối chiếu không hợp lệ.")
	}
	tx, err := s.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	out.Context, err = ReadContext(ctx, tx)
	if err != nil {
		return out, err
	}
	if out.ConversionDate != "" && from < out.ConversionDate {
		return out, accounting.Fail("BEFORE_CONVERSION", "Khoảng đối chiếu bắt đầu trước mốc chuyển đổi; chưa có đủ lịch sử cho khoảng này.")
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.code,a.name,CASE WHEN a.requires_partner=1 THEN COALESCE(l.partner_id,'') ELSE '' END,d.date,d.source_type,l.debit,l.credit FROM document_lines l JOIN documents d ON d.id=l.document_id JOIN accounts a ON a.id=l.account_id WHERE d.status='posted' AND d.date<=? ORDER BY a.code,l.partner_id,d.date,d.posted_order,l.position`, to)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	type balances struct {
		name             string
		opening, closing map[string]*big.Int
		debit, credit    *big.Int
	}
	all := map[string]*balances{}
	order := []string{}
	for rows.Next() {
		var code, name, partner, date, source string
		var d, c int64
		if err = rows.Scan(&code, &name, &partner, &date, &source, &d, &c); err != nil {
			return out, err
		}
		a := all[code]
		if a == nil {
			a = &balances{name, map[string]*big.Int{}, map[string]*big.Int{}, new(big.Int), new(big.Int)}
			all[code] = a
			order = append(order, code)
		}
		delta := new(big.Int).Sub(big.NewInt(d), big.NewInt(c))
		if a.closing[partner] == nil {
			a.closing[partner] = new(big.Int)
		}
		a.closing[partner].Add(a.closing[partner], delta)
		if IsOpening(source) || date < from {
			if a.opening[partner] == nil {
				a.opening[partner] = new(big.Int)
			}
			a.opening[partner].Add(a.opening[partner], delta)
		} else {
			a.debit.Add(a.debit, big.NewInt(d))
			a.credit.Add(a.credit, big.NewInt(c))
		}
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	sides := func(values map[string]*big.Int) (string, string) {
		d, c := new(big.Int), new(big.Int)
		for _, v := range values {
			if v.Sign() > 0 {
				d.Add(d, v)
			} else {
				c.Sub(c, v)
			}
		}
		return d.String(), c.String()
	}
	for _, code := range order {
		a := all[code]
		od, oc := sides(a.opening)
		cd, cc := sides(a.closing)
		out.Rows = append(out.Rows, Row{Key: code, Code: code, Name: a.name, Debit: a.debit.String(), Credit: a.credit.String(), OpeningDebit: od, OpeningCredit: oc, ClosingDebit: cd, ClosingCredit: cc})
	}
	return out, tx.Commit()
}
