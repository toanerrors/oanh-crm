package ledger

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"math/big"
	"slices"
	"strings"
	"time"
	"unicode"

	"oanh.local/accounting/internal/accounting"
)

type Metrics struct {
	BankBalance   string            `json:"bankBalance"`
	Revenue       string            `json:"revenue"`
	Expense       string            `json:"expense"`
	Receivable    string            `json:"receivable"`
	Balances      map[string]string `json:"balances"`
	DocumentCount int               `json:"documentCount"`
	PostedCount   int               `json:"postedCount"`
	DraftCount    int               `json:"draftCount"`
	Months        []Month           `json:"months"`
}
type Month struct {
	Key     string `json:"key"`
	Revenue string `json:"revenue"`
	Expense string `json:"expense"`
}
type Row struct {
	Key           string `json:"key"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Date          string `json:"date,omitempty"`
	DocumentID    string `json:"documentId,omitempty"`
	Description   string `json:"description,omitempty"`
	Account       string `json:"account,omitempty"`
	Debit         string `json:"debit"`
	Credit        string `json:"credit"`
	OpeningDebit  string `json:"openingDebit"`
	OpeningCredit string `json:"openingCredit"`
	ClosingDebit  string `json:"closingDebit"`
	ClosingCredit string `json:"closingCredit"`
}
type Report struct {
	Context
	Rows           []Row  `json:"rows"`
	Total          int    `json:"total"`
	Debit          string `json:"debit"`
	Credit         string `json:"credit"`
	OpeningDebit   string `json:"openingDebit"`
	OpeningCredit  string `json:"openingCredit"`
	ClosingDebit   string `json:"closingDebit"`
	ClosingCredit  string `json:"closingCredit"`
	AccountBalance *Row   `json:"accountBalance,omitempty"`
}
type Filter struct {
	Period, Kind, Account, Status, Type, Query, Sort, Source, BankAccount string
	Page, PageSize                                                        int
	Desc                                                                  bool
	Revision                                                              *int64
}
type DocumentList struct {
	Context
	Documents []Document     `json:"documents"`
	Total     int            `json:"total"`
	Amount    string         `json:"amount"`
	Counts    map[string]int `json:"counts"`
	Money     *MoneySummary  `json:"money,omitempty"`
}
type MoneySummary struct {
	Opening  string `json:"opening"`
	Receipts string `json:"receipts"`
	Payments string `json:"payments"`
	Closing  string `json:"closing"`
}

func search(value string) string {
	return strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		for _, pair := range [][2]string{{"àáạảãâầấậẩẫăằắặẳẵ", "a"}, {"èéẹẻẽêềếệểễ", "e"}, {"ìíịỉĩ", "i"}, {"òóọỏõôồốộổỗơờớợởỡ", "o"}, {"ùúụủũưừứựửữ", "u"}, {"ỳýỵỷỹ", "y"}, {"đ", "d"}} {
			if strings.ContainsRune(pair[0], r) {
				return rune(pair[1][0])
			}
		}
		return r
	}, strings.TrimSpace(value))
}
func add(m map[string]*big.Int, key string, value int64) {
	if m[key] == nil {
		m[key] = new(big.Int)
	}
	m[key].Add(m[key], big.NewInt(value))
}
func amount(m map[string]*big.Int, key string) string {
	if m[key] == nil {
		return "0"
	}
	return m[key].String()
}
func (s *Service) Metrics(ctx context.Context, q Queryer, period string) (*Metrics, error) {
	m := &Metrics{Balances: map[string]string{}, Months: []Month{}}
	rows, err := q.QueryContext(ctx, "SELECT id FROM accounts")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		m.Balances[id] = "0"
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	rows, err = q.QueryContext(ctx, "SELECT status,count(*) FROM documents WHERE source_type NOT IN ('opening','opening_adjustment','opening_adjustment_reversal') AND substr(date,1,7)=? GROUP BY status", period)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var status string
		var count int
		if err = rows.Scan(&status, &count); err != nil {
			rows.Close()
			return nil, err
		}
		counts[status] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	m.PostedCount = counts["posted"]
	m.DraftCount = counts["draft"]
	m.DocumentCount = m.PostedCount + m.DraftCount
	balances, months, partners := map[string]*big.Int{}, map[string]*big.Int{}, map[string]*big.Int{}
	rows, err = q.QueryContext(ctx, "SELECT a.id,a.account_group,COALESCE(l.partner_id,''),substr(d.date,1,7),d.source_type,l.debit,l.credit FROM document_lines l JOIN documents d ON d.id=l.document_id JOIN accounts a ON a.id=l.account_id WHERE d.status='posted' AND substr(d.date,1,7)<=?", period)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, group, partner, month, source string
		var d, c int64
		if err = rows.Scan(&id, &group, &partner, &month, &source, &d, &c); err != nil {
			rows.Close()
			return nil, err
		}
		add(balances, id, d)
		add(balances, id, -c)
		if group == "receivable" {
			add(partners, partner, d)
			add(partners, partner, -c)
		}
		if !IsOpening(source) && source != "closing" && source != "closing_reversal" {
			if group == "revenue" {
				add(months, month+":revenue", c)
				add(months, month+":revenue", -d)
			}
			if group == "expense" {
				add(months, month+":expense", d)
				add(months, month+":expense", -c)
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for id, n := range balances {
		m.Balances[id] = n.String()
	}
	bankTotal := new(big.Int)
	bankRows, e := q.QueryContext(ctx, "SELECT account_id FROM bank_accounts UNION SELECT id FROM accounts WHERE id IN ('112','1121')")
	if e != nil {
		return nil, e
	}
	for bankRows.Next() {
		var id string
		if e = bankRows.Scan(&id); e != nil {
			bankRows.Close()
			return nil, e
		}
		if balances[id] != nil {
			bankTotal.Add(bankTotal, balances[id])
		}
	}
	e = bankRows.Err()
	bankRows.Close()
	if e != nil {
		return nil, e
	}
	m.BankBalance = bankTotal.String()
	receivable := new(big.Int)
	for _, n := range partners {
		if n.Sign() > 0 {
			receivable.Add(receivable, n)
		}
	}
	m.Receivable = receivable.String()
	m.Revenue = amount(months, period+":revenue")
	m.Expense = amount(months, period+":expense")
	end, _ := time.Parse("2006-01", period)
	for i := -5; i <= 0; i++ {
		key := end.AddDate(0, i, 0).Format("2006-01")
		m.Months = append(m.Months, Month{Key: key, Revenue: amount(months, key+":revenue"), Expense: amount(months, key+":expense")})
	}
	return m, nil
}
func (s *Service) Documents(ctx context.Context, f Filter) (DocumentList, error) {
	result := DocumentList{Documents: []Document{}, Counts: map[string]int{"all": 0, "posted": 0, "draft": 0}}
	tx, err := s.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	result.Context, err = ReadContext(ctx, tx)
	result.Period = f.Period
	if err != nil {
		return result, err
	}
	if f.Revision != nil && *f.Revision != result.Revision {
		return result, accounting.Fail("VERSION_CONFLICT", "Danh sách đã thay đổi. Tải lại dữ liệu trước khi tiếp tục.")
	}
	bankDocuments := map[string]bool{}
	if f.BankAccount != "" {
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM bank_accounts WHERE id=?", f.BankAccount).Scan(&count); err != nil {
			return result, err
		}
		if count != 1 {
			return result, accounting.Fail("INVALID_BANK", "Không tìm thấy tài khoản ngân hàng.")
		}
		bankRows, e := tx.QueryContext(ctx, "SELECT DISTINCT document_id FROM bank_movements WHERE bank_id=?", f.BankAccount)
		if e != nil {
			return result, e
		}
		for bankRows.Next() {
			var id string
			if e = bankRows.Scan(&id); e != nil {
				bankRows.Close()
				return result, e
			}
			bankDocuments[id] = true
		}
		e = bankRows.Err()
		bankRows.Close()
		if e != nil {
			return result, e
		}
	}
	moneyAccounts := strings.Split(f.Account, ",")
	allMoney := len(moneyAccounts) > 0 && len(moneyAccounts) <= 100
	for _, id := range moneyAccounts {
		var group string
		if e := tx.QueryRowContext(ctx, "SELECT account_group FROM accounts WHERE id=?", id).Scan(&group); e != nil || group != "cash" {
			allMoney = false
			break
		}
	}
	if allMoney || f.BankAccount != "" {
		result.Money, err = moneySummary(ctx, tx, f.Account, f.Period, f.BankAccount)
		if err != nil {
			return result, err
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT id FROM documents WHERE source_type NOT IN ('opening','opening_adjustment','opening_adjustment_reversal') AND substr(date,1,7)=? ORDER BY date DESC,posted_order DESC,id", f.Period)
	if err != nil {
		return result, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	var docs []Document
	total := new(big.Int)
	query := search(f.Query)
	for _, id := range ids {
		d, e := ReadDocument(ctx, tx, id)
		if e != nil {
			return result, e
		}
		matchesAccount := f.Account == "" || f.Account == "all"
		for _, l := range d.Lines {
			if slices.Contains(strings.Split(f.Account, ","), l.AccountID) {
				matchesAccount = true
			}
		}
		if f.BankAccount != "" && !bankDocuments[d.ID] {
			continue
		}
		if f.Source != "" && d.SourceType != f.Source {
			continue
		}
		if !matchesAccount || f.Type != "" && f.Type != "all" && d.Type != f.Type {
			continue
		}
		result.Counts["all"]++
		result.Counts[d.Status]++
		if f.Status != "" && f.Status != "all" && d.Status != f.Status {
			continue
		}
		names := ""
		for _, l := range d.Lines {
			names += " " + l.PartnerName
		}
		if !strings.Contains(search(d.Reference+" "+d.Description+names), query) {
			continue
		}
		docs = append(docs, d)
		for _, line := range d.Lines {
			n, _ := accounting.Money(line.Debit)
			total.Add(total, big.NewInt(n))
		}
	}
	if f.Sort == "amount" {
		slices.SortStableFunc(docs, func(a, b Document) int {
			sum := func(d Document) *big.Int {
				n := new(big.Int)
				for _, l := range d.Lines {
					v, _ := new(big.Int).SetString(l.Debit, 10)
					n.Add(n, v)
				}
				return n
			}
			cmp := sum(a).Cmp(sum(b))
			if f.Desc {
				return -cmp
			}
			return cmp
		})
	} else if f.Sort == "date" && !f.Desc {
		slices.Reverse(docs)
	}
	result.Total = len(docs)
	result.Amount = total.String()
	from, to := bounds(f.Page, f.PageSize, len(docs))
	result.Documents = docs[from:to]
	if result.Documents == nil {
		result.Documents = []Document{}
	}
	return result, tx.Commit()
}
func moneySummary(ctx context.Context, tx *sql.Tx, account, period, bank string) (*MoneySummary, error) {
	start, err := time.Parse("2006-01", period)
	if err != nil {
		return nil, accounting.Fail("BAD_REQUEST", "Kỳ báo cáo tiền không hợp lệ.")
	}
	end := start.AddDate(0, 1, 0).Format("2006-01-02")
	accounts := strings.Split(account, ",")
	args := make([]any, 0, len(accounts)+1)
	for _, id := range accounts {
		args = append(args, id)
	}
	args = append(args, end)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(accounts)), ",")
	query := "SELECT d.date,d.source_type,l.debit,l.credit FROM document_lines l JOIN documents d ON d.id=l.document_id WHERE d.status='posted' AND l.account_id IN (" + placeholders + ") AND d.date<?"
	if bank != "" {
		query = "SELECT d.date,d.source_type,m.debit,m.credit FROM bank_movements m JOIN documents d ON d.id=m.document_id WHERE d.status='posted' AND m.bank_id=? AND d.date<?"
		args = []any{bank, end}
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	opening, receipts, payments := new(big.Int), new(big.Int), new(big.Int)
	for rows.Next() {
		var date, source string
		var debit, credit int64
		if err = rows.Scan(&date, &source, &debit, &credit); err != nil {
			return nil, err
		}
		if IsOpening(source) || date < start.Format("2006-01-02") {
			opening.Add(opening, big.NewInt(debit))
			opening.Sub(opening, big.NewInt(credit))
		} else {
			receipts.Add(receipts, big.NewInt(debit))
			payments.Add(payments, big.NewInt(credit))
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	closing := new(big.Int).Sub(new(big.Int).Add(opening, receipts), payments)
	return &MoneySummary{opening.String(), receipts.String(), payments.String(), closing.String()}, nil
}
func bounds(page, size, total int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	from := min((page-1)*size, total)
	return from, min(from+size, total)
}
func (s *Service) Report(ctx context.Context, f Filter) (Report, error) {
	report := Report{Rows: []Row{}}
	tx, err := s.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	report.Context, err = ReadContext(ctx, tx)
	report.Period = f.Period
	if err != nil {
		return report, err
	}
	if report.ConversionDate != "" && f.Period < report.ConversionDate[:7] {
		return report, accounting.Fail("BEFORE_CONVERSION", "Kỳ báo cáo trước mốc chuyển đổi; chưa có dữ liệu lịch sử trong bộ dữ liệu này.")
	}
	if f.Revision != nil && *f.Revision != report.Revision {
		return report, accounting.Fail("VERSION_CONFLICT", "Sổ đã thay đổi. Tải lại dữ liệu để đọc cùng phiên bản.")
	}
	accounts := map[string]Account{}
	order := []string{}
	rows, err := tx.QueryContext(ctx, "SELECT id,code,name,account_group,requires_partner FROM accounts ORDER BY code")
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var a Account
		if err = rows.Scan(&a.ID, &a.Code, &a.Name, &a.Group, &a.RequiresPartner); err != nil {
			rows.Close()
			return report, err
		}
		accounts[a.ID] = a
		order = append(order, a.ID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return report, err
	}
	opening, closing, debits, credits := map[string]*big.Int{}, map[string]*big.Int{}, map[string]*big.Int{}, map[string]*big.Int{}
	rows, err = tx.QueryContext(ctx, "SELECT d.id,d.reference,d.date,d.description,l.position,l.account_id,COALESCE(l.partner_id,''),l.partner_name,l.account_name,l.description,l.debit,l.credit,d.source_type FROM documents d JOIN document_lines l ON l.document_id=d.id WHERE d.status='posted' AND substr(d.date,1,7)<=? ORDER BY d.date,d.posted_order,l.position", f.Period)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var id, reference, date, description, account, partner, partnerName, accountName, lineDescription, source string
		var index int
		var d, c int64
		if err = rows.Scan(&id, &reference, &date, &description, &index, &account, &partner, &partnerName, &accountName, &lineDescription, &d, &c, &source); err != nil {
			rows.Close()
			return report, err
		}
		key := account
		if accounts[account].RequiresPartner {
			key += "|" + partner
		}
		add(closing, key, d)
		add(closing, key, -c)
		if IsOpening(source) || date < f.Period+"-01" {
			add(opening, key, d)
			add(opening, key, -c)
		} else {
			add(debits, account, d)
			add(credits, account, c)
		}
		if IsOpening(source) || date[:7] != f.Period {
			continue
		}
		if f.Kind == "journal" || f.Kind == "ledger" {
			if f.Kind == "ledger" && f.Account != "" && f.Account != "all" && account != f.Account {
				continue
			}
			if lineDescription == "" {
				lineDescription = description
			}
			report.Rows = append(report.Rows, Row{Key: fmt.Sprintf("%s-%d", id, index), Code: reference, Name: accountName, Date: date, DocumentID: id, Description: lineDescription, Account: account, Debit: accounting.Text(d), Credit: accounting.Text(c)})
		}
		if f.Kind == "income" && !IsOpening(source) && source != "closing" && source != "closing_reversal" && (accounts[account].Group == "revenue" || accounts[account].Group == "expense") {
			report.Rows = append(report.Rows, Row{Key: fmt.Sprintf("%s-%d", id, index), Code: reference, Name: accountName, Date: date, DocumentID: id, Description: lineDescription, Account: account, Debit: accounting.Text(d), Credit: accounting.Text(c)})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return report, err
	}
	trial := []Row{}
	for _, id := range order {
		od, oc, cd, cc := new(big.Int), new(big.Int), new(big.Int), new(big.Int)
		for key, value := range opening {
			if key == id || strings.HasPrefix(key, id+"|") {
				if value.Sign() > 0 {
					od.Add(od, value)
				} else {
					oc.Sub(oc, value)
				}
			}
		}
		for key, value := range closing {
			if key == id || strings.HasPrefix(key, id+"|") {
				if value.Sign() > 0 {
					cd.Add(cd, value)
				} else {
					cc.Sub(cc, value)
				}
			}
		}
		a := accounts[id]
		row := Row{Key: id, Code: a.Code, Name: a.Name, Debit: amount(debits, id), Credit: amount(credits, id), OpeningDebit: od.String(), OpeningCredit: oc.String(), ClosingDebit: cd.String(), ClosingCredit: cc.String()}
		trial = append(trial, row)
		if f.Account == id {
			copy := row
			report.AccountBalance = &copy
		}
	}
	if f.Kind == "trial" {
		report.Rows = trial
	}
	if f.Kind == "debt" {
		group := "receivable"
		if f.Account == "payable" {
			group = "payable"
		}
		rows, err = tx.QueryContext(ctx, "SELECT id,code,name FROM partners ORDER BY code")
		if err != nil {
			return report, err
		}
		for rows.Next() {
			var id, code, name string
			if err = rows.Scan(&id, &code, &name); err != nil {
				rows.Close()
				return report, err
			}
			balance := new(big.Int)
			for account, a := range accounts {
				if a.Group == group && closing[account+"|"+id] != nil {
					balance.Add(balance, closing[account+"|"+id])
				}
			}
			if balance.Sign() == 0 {
				continue
			}
			row := Row{Key: id, Code: code, Name: name, Debit: "0", Credit: "0"}
			if balance.Sign() > 0 {
				row.Debit = balance.String()
			} else {
				row.Credit = new(big.Int).Neg(balance).String()
			}
			report.Rows = append(report.Rows, row)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return report, err
		}
	}
	query := search(f.Query)
	report.Rows = slices.DeleteFunc(report.Rows, func(row Row) bool { return !strings.Contains(search(row.Code+" "+row.Name+" "+row.Description), query) })
	sums := []*big.Int{new(big.Int), new(big.Int), new(big.Int), new(big.Int), new(big.Int), new(big.Int)}
	for _, row := range report.Rows {
		for i, value := range []string{row.Debit, row.Credit, row.OpeningDebit, row.OpeningCredit, row.ClosingDebit, row.ClosingCredit} {
			if value != "" {
				n, ok := new(big.Int).SetString(value, 10)
				if !ok {
					return report, fmt.Errorf("invalid derived amount")
				}
				sums[i].Add(sums[i], n)
			}
		}
	}
	report.Debit = sums[0].String()
	report.Credit = sums[1].String()
	report.OpeningDebit = sums[2].String()
	report.OpeningCredit = sums[3].String()
	report.ClosingDebit = sums[4].String()
	report.ClosingCredit = sums[5].String()
	report.Total = len(report.Rows)
	from, to := bounds(f.Page, f.PageSize, report.Total)
	report.Rows = report.Rows[from:to]
	return report, tx.Commit()
}
func ExportRows(w io.Writer, metadata Context, header []string, rows [][]string) error {
	if _, err := io.WriteString(w, "\ufeff"); err != nil {
		return err
	}
	writer := csv.NewWriter(w)
	write := func(row []string) error {
		safe := append([]string(nil), row...)
		for i, cell := range safe {
			trim := strings.TrimSpace(cell)
			if trim != "" && strings.ContainsAny(trim[:1], "=+-@") {
				safe[i] = "'" + cell
			}
		}
		return writer.Write(safe)
	}
	if err := write([]string{metadata.CompanyName, "Kỳ " + metadata.Period + " · Epoch " + metadata.Epoch, fmt.Sprintf("Phiên bản %d", metadata.Revision), "VND"}); err != nil {
		return err
	}
	if err := write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
