package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

type Service struct{ Ledger *ledger.Service }

func (s *Service) SavePolicy(ctx context.Context, cmd ledger.Command, p Policy) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "admin.policy.save", p, func(tx *sql.Tx) (any, error) {
		if strings.TrimSpace(p.Regime) == "" || len(p.Regime) > 200 {
			return nil, accounting.Fail("VALIDATION", "Nhập chế độ kế toán và phiên bản áp dụng cho chính sách.")
		}
		if !p.Confirmed || !accounting.Date(p.EffectiveFrom) || !strings.Contains("|half_up|half_even|down|", "|"+p.Rounding+"|") || !strings.Contains("|deduction|direct|", "|"+p.VATMethod+"|") || !strings.Contains("|fifo|moving_average|", "|"+p.InventoryMethod+"|") || p.QuantityScale < 0 || p.QuantityScale > 6 || p.DepreciationMethod != "straight_line" || p.ResidualMethod != "last_period" {
			return nil, accounting.Fail("POLICY_REQUIRED", "Xác nhận ngày hiệu lực, VAT, giá kho, scale, làm tròn và phân bổ phần dư tháng cuối.")
		}
		if _, err := accounting.Percent(accounting.Rat(100), p.CorporateTaxRate, p.Rounding); err != nil {
			return nil, err
		}
		if len(p.VATRates) == 0 || len(p.VATRates) > 30 || len(p.Insurance) > 10 || len(p.PITBrackets) == 0 || len(p.PITBrackets) > 20 {
			return nil, accounting.Fail("VALIDATION", "Cấu hình danh sách thuế suất, bảo hiểm và biểu thuế TNCN hợp lệ.")
		}
		for _, rate := range p.VATRates {
			if _, e := accounting.Percent(accounting.Rat(100), rate, p.Rounding); e != nil {
				return nil, e
			}
		}
		for _, value := range []string{p.StandardDays, p.HoursPerDay} {
			r, e := accounting.Decimal(value)
			if e != nil || r.Sign() <= 0 || r.Cmp(accounting.Rat(31)) > 0 {
				return nil, accounting.Fail("VALIDATION", "Ngày công chuẩn và giờ/ngày phải dương, không quá 31.")
			}
		}
		for _, value := range []string{p.PersonalDeduction, p.DependentDeduction} {
			if _, e := accounting.Money(value); e != nil {
				return nil, e
			}
		}
		for _, rate := range p.Insurance {
			if strings.TrimSpace(rate.Name) == "" {
				return nil, accounting.Fail("VALIDATION", "Nhập tên khoản bảo hiểm.")
			}
			for _, value := range []string{rate.EmployeeRate, rate.EmployerRate} {
				if _, e := accounting.Percent(accounting.Rat(100), value, p.Rounding); e != nil {
					return nil, e
				}
			}
		}
		previous := int64(0)
		for index, b := range p.PITBrackets {
			if _, e := accounting.Percent(accounting.Rat(100), b.Rate, p.Rounding); e != nil {
				return nil, e
			}
			if b.UpTo == "" {
				if index != len(p.PITBrackets)-1 {
					return nil, accounting.Fail("VALIDATION", "Bậc không giới hạn phải ở cuối biểu thuế.")
				}
			} else {
				n, e := accounting.Money(b.UpTo)
				if e != nil || n <= previous {
					return nil, accounting.Fail("VALIDATION", "Giới hạn bậc thuế phải tăng dần.")
				}
				previous = n
			}
		}
		if p.PITBrackets[len(p.PITBrackets)-1].UpTo != "" {
			return nil, accounting.Fail("VALIDATION", "Biểu thuế phải có bậc cuối không giới hạn.")
		}
		var affected int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM operations WHERE date>=?", p.EffectiveFrom).Scan(&affected); err != nil {
			return nil, err
		}
		if affected > 0 {
			return nil, accounting.Fail("HISTORY_LOCKED", "Ngày hiệu lực mới không được thay chính sách của nghiệp vụ đã ghi nhận.")
		}
		var latest string
		err := tx.QueryRowContext(ctx, "SELECT data FROM policies ORDER BY effective_from DESC LIMIT 1").Scan(&latest)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == nil {
			var old Policy
			if err = json.Unmarshal([]byte(latest), &old); err != nil {
				return nil, err
			}
			if p.EffectiveFrom <= old.EffectiveFrom {
				return nil, accounting.Fail("HISTORY_LOCKED", "Phiên bản mới phải có ngày hiệu lực sau phiên bản gần nhất.")
			}
			if p.InventoryMethod != old.InventoryMethod || p.QuantityScale != old.QuantityScale {
				if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM stock_lots WHERE quantity>0").Scan(&affected); err != nil {
					return nil, err
				}
				if affected > 0 {
					return nil, accounting.Fail("STOCK_POLICY_LOCKED", "Chỉ đổi phương pháp/scale kho sau khi xử lý hết tồn theo chính sách cũ.")
				}
			}
		}
		if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0)+1 FROM policies").Scan(&p.Version); err != nil {
			return nil, err
		}
		p.ID = sqlite.ID()
		b, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO policies VALUES(?,?,?,?)", p.ID, p.Version, p.EffectiveFrom, string(b))
		return p, err
	})
}
func PolicyAt(ctx context.Context, q ledger.Queryer, date string) (Policy, error) {
	var p Policy
	var b string
	err := q.QueryRowContext(ctx, "SELECT data FROM policies WHERE effective_from<=? ORDER BY effective_from DESC LIMIT 1", date).Scan(&b)
	if err == sql.ErrNoRows {
		return p, accounting.Fail("POLICY_REQUIRED", "Chưa có chính sách được xác nhận cho ngày nghiệp vụ. Cấu hình trong Cài đặt.")
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(b), &p)
	return p, err
}
func (s *Service) SaveItem(ctx context.Context, cmd ledger.Command, item Item) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "item.save", item, func(tx *sql.Tx) (any, error) {
		return s.saveItemTx(ctx, tx, item)
	})
}
func (s *Service) saveItemTx(ctx context.Context, tx *sql.Tx, item Item) (any, error) {
	if !accounting.ID(item.ID) || !accounting.ID(item.Code) || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Unit) == "" {
		return nil, accounting.Fail("VALIDATION", "Nhập mã, tên và đơn vị hàng hóa/dịch vụ.")
	}
	var code, unit string
	var tracked bool
	err := tx.QueryRowContext(ctx, "SELECT code,tracked,unit FROM items WHERE id=?", item.ID).Scan(&code, &tracked, &unit)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil && (code != item.Code || unit != item.Unit) {
		return nil, accounting.Fail("HISTORY_LOCKED", "Giữ mã và đơn vị hàng hóa/dịch vụ đã tạo.")
	}
	if err == nil && tracked != item.Tracked {
		var used bool
		err = tx.QueryRowContext(ctx, `SELECT
            EXISTS(SELECT 1 FROM stock_movements WHERE item_id=?) OR
            EXISTS(SELECT 1 FROM stock_lots WHERE item_id=?) OR
            EXISTS(SELECT 1 FROM operations o,json_each(o.payload,'$.lines') l WHERE json_extract(l.value,'$.itemId')=?) OR
            EXISTS(SELECT 1 FROM voucher_inputs v,json_each(v.payload,'$.lines') l WHERE json_extract(l.value,'$.itemId')=?) OR
            EXISTS(SELECT 1 FROM opening_balances o,json_each(o.payload,'$.stocks') l WHERE json_extract(l.value,'$.itemId')=?) OR
            EXISTS(SELECT 1 FROM opening_adjustments o,json_each(o.after_payload,'$.stocks') l WHERE json_extract(l.value,'$.itemId')=?)`, item.ID, item.ID, item.ID, item.ID, item.ID, item.ID).Scan(&used)
		if err != nil {
			return nil, err
		}
		if used {
			return nil, accounting.Fail("HISTORY_LOCKED", "Mã đã có chứng từ hoặc số dư kho, không thể đổi giữa hàng hóa và dịch vụ. Giữ loại hiện tại và tạo mã khác nếu cần.")
		}
	}
	var duplicate int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM items WHERE code=? COLLATE NOCASE AND id<>?", item.Code, item.ID).Scan(&duplicate); err != nil {
		return nil, err
	}
	if duplicate > 0 {
		return nil, accounting.Fail("DUPLICATE_CODE", "Mã hàng hóa/dịch vụ đã tồn tại.")
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO items VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,unit=excluded.unit,tracked=excluded.tracked,active=excluded.active", item.ID, item.Code, item.Name, item.Unit, item.Tracked, item.Active)
	return item, err
}
func (s *Service) SaveEmployee(ctx context.Context, cmd ledger.Command, e Employee) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "admin.employee.save", e, func(tx *sql.Tx) (any, error) {
		return s.saveEmployeeTx(ctx, tx, e)
	})
}
func (s *Service) saveEmployeeTx(ctx context.Context, tx *sql.Tx, e Employee) (any, error) {
	base, err := accounting.Money(e.BaseSalary)
	if err != nil {
		return nil, err
	}
	insurance, err := accounting.Money(e.InsuranceBase)
	if err != nil {
		return nil, err
	}
	if !accounting.ID(e.ID) || !accounting.ID(e.Code) || strings.TrimSpace(e.Name) == "" || e.Dependents < 0 || e.Dependents > 100 {
		return nil, accounting.Fail("VALIDATION", "Thông tin nhân viên không hợp lệ.")
	}
	var code string
	err = tx.QueryRowContext(ctx, "SELECT code FROM employees WHERE id=?", e.ID).Scan(&code)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil && code != e.Code {
		return nil, accounting.Fail("HISTORY_LOCKED", "Giữ nguyên mã nhân viên.")
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO employees VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,base_salary=excluded.base_salary,insurance_base=excluded.insurance_base,dependents=excluded.dependents,active=excluded.active", e.ID, e.Code, e.Name, base, insurance, e.Dependents, e.Active)
	return e, err
}
func (s *Service) Data(ctx context.Context) (Data, error) {
	d := Data{Policies: []Policy{}, Items: []Item{}, Employees: []Employee{}, Assets: []Asset{}}
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return d, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT data FROM policies ORDER BY effective_from DESC")
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var b string
		var p Policy
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return d, err
		}
		if err = json.Unmarshal([]byte(b), &p); err != nil {
			rows.Close()
			return d, err
		}
		d.Policies = append(d.Policies, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT id,code,name,unit,tracked,active FROM items ORDER BY code")
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var i Item
		if err = rows.Scan(&i.ID, &i.Code, &i.Name, &i.Unit, &i.Tracked, &i.Active); err != nil {
			rows.Close()
			return d, err
		}
		d.Items = append(d.Items, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	for index := range d.Items {
		i := &d.Items[index]
		r, e := tx.QueryContext(ctx, "SELECT quantity,value,scale FROM stock_lots WHERE item_id=?", i.ID)
		if e != nil {
			return d, e
		}
		qty, val := new(big.Int), new(big.Int)
		for r.Next() {
			var q, v int64
			if e = r.Scan(&q, &v, &i.Scale); e != nil {
				r.Close()
				return d, e
			}
			qty.Add(qty, big.NewInt(q))
			val.Add(val, big.NewInt(v))
		}
		e = r.Err()
		r.Close()
		if e != nil {
			return d, e
		}
		i.Quantity = qty.String()
		i.Value = val.String()
	}
	rows, err = tx.QueryContext(ctx, "SELECT id,code,name,base_salary,insurance_base,dependents,active FROM employees ORDER BY code")
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var e Employee
		var base, insurance int64
		if err = rows.Scan(&e.ID, &e.Code, &e.Name, &base, &insurance, &e.Dependents, &e.Active); err != nil {
			rows.Close()
			return d, err
		}
		e.BaseSalary = accounting.Text(base)
		e.InsuranceBase = accounting.Text(insurance)
		d.Employees = append(d.Employees, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT id,code,name,kind,cost,residual,months,start_period,allocated FROM current_assets ORDER BY code")
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var a Asset
		var c, r, v int64
		if err = rows.Scan(&a.ID, &a.Code, &a.Name, &a.Kind, &c, &r, &a.Months, &a.StartPeriod, &v); err != nil {
			rows.Close()
			return d, err
		}
		a.Cost = accounting.Text(c)
		a.Residual = accounting.Text(r)
		a.Allocated = accounting.Text(v)
		d.Assets = append(d.Assets, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	return d, tx.Commit()
}
func (s *Service) Preview(ctx context.Context, user identity.User, epoch string, input Input) (Result, error) {
	var empty Result
	if user.Role == "viewer" {
		return empty, accounting.Fail("FORBIDDEN", "Không có quyền lập nghiệp vụ tự động.")
	}
	tx, err := s.Ledger.Store.Write.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	c, err := ledger.ReadContext(ctx, tx)
	if err != nil {
		return empty, err
	}
	if c.Epoch != epoch {
		return empty, accounting.Fail("DATASET_CHANGED", "Phiên dữ liệu đã thay đổi.")
	}
	return s.run(ctx, tx, input)
}
func (s *Service) Execute(ctx context.Context, cmd ledger.Command, input Input) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "operation."+input.Kind, input, func(tx *sql.Tx) (any, error) { return s.run(ctx, tx, input) })
}
func (s *Service) run(ctx context.Context, tx *sql.Tx, input Input) (Result, error) {
	r := Result{Amounts: map[string]string{}, Notes: []string{}}
	if !accounting.Date(input.Date) || !accounting.ID(input.SourceID) || strings.TrimSpace(input.Description) == "" {
		return r, accounting.Fail("VALIDATION", "Nhập ngày, mã nguồn nghiệp vụ và diễn giải.")
	}
	if err := normalizeBanks(ctx, tx, &input); err != nil {
		return r, err
	}
	if err := validateVoucher(input); err != nil {
		return r, err
	}
	p, err := PolicyAt(ctx, tx, input.Date)
	if err != nil {
		return r, err
	}
	metadata, err := ledger.ReadContext(ctx, tx)
	if err != nil {
		return r, err
	}
	if input.ExpectedPolicyID != "" && input.ExpectedPolicyID != p.ID || input.ExpectedRevision != nil && *input.ExpectedRevision != metadata.Revision {
		return r, accounting.Fail("VERSION_CONFLICT", "Dữ liệu hoặc chính sách đã thay đổi sau khi xem trước. Tính lại trước khi ghi nhận.")
	}
	r.Revision = metadata.Revision
	r.PolicyID = p.ID
	r.PolicyVersion = p.Version
	if input.ReplacementOf != "" {
		if input.Kind != "receipt" && input.Kind != "payment" && input.Kind != "transfer" {
			return r, accounting.Fail("INVALID_SOURCE", "Thay thế tại đây áp dụng cho phiếu thu, chi và chuyển tiền.")
		}
		if strings.TrimSpace(input.ReplacementReason) == "" || utf8.RuneCountInString(input.ReplacementReason) > 900 {
			return r, accounting.Fail("VALIDATION", "Nhập lý do sửa phiếu (tối đa 900 ký tự).")
		}
		original, e := ledger.ReadDocument(ctx, tx, input.ReplacementOf)
		if e != nil {
			return r, e
		}
		var kind string
		if e = tx.QueryRowContext(ctx, "SELECT kind FROM operations WHERE document_id=?", original.ID).Scan(&kind); e != nil || kind != input.Kind || input.DraftID == original.ID || input.SourceID == original.SourceID {
			return r, accounting.Fail("INVALID_SOURCE", "Bản thay thế phải cùng loại phiếu, dùng nháp và mã nguồn mới.")
		}
		reversal, e := s.run(ctx, tx, Input{Kind: "treasury_reversal", SourceID: original.ID, OriginalDocumentID: original.ID, Date: input.Date, Description: "Đảo để thay thế " + original.Reference})
		if e != nil {
			return r, e
		}
		r.Notes = append(r.Notes, "Thay thế "+original.Reference+"; chứng từ đảo "+reversal.Document.Reference+" và phiếu mới được ghi trong cùng lần xác nhận.")
	}
	doc := ledger.Document{ID: sqlite.ID(), Type: "journal", Date: input.Date, Description: input.Description, PartnerID: input.PartnerID, SourceType: input.Kind, SourceID: input.SourceID, PolicyID: p.ID, Lines: []ledger.Line{}}
	if input.DraftID != "" {
		saved, e := ledger.ReadDocument(ctx, tx, input.DraftID)
		if e != nil {
			return r, e
		}
		var original string
		if e = tx.QueryRowContext(ctx, "SELECT payload FROM voucher_inputs WHERE document_id=?", input.DraftID).Scan(&original); e != nil {
			return r, accounting.Fail("INVALID_SOURCE", "Không tìm thấy nháp nghiệp vụ.")
		}
		var previous Input
		if e = json.Unmarshal([]byte(original), &previous); e != nil {
			return r, e
		}
		if saved.Status != "draft" || saved.Version != input.DraftVersion {
			return r, accounting.Fail("VERSION_CONFLICT", "Nháp đã thay đổi hoặc đã ghi sổ. Tải lại trước khi tiếp tục.")
		}
		if previous.Kind != input.Kind {
			return r, accounting.Fail("INVALID_SOURCE", "Không đổi loại nghiệp vụ của nháp đã lưu.")
		}
		doc.ID, doc.Version = saved.ID, saved.Version
	}
	switch input.Kind {
	case "treasury_reversal":
		doc.SourceID = input.OriginalDocumentID
		err = s.reverseTreasury(ctx, tx, input, &doc, &r)
	case "receipt", "payment", "transfer":
		err = s.treasury(ctx, tx, input, &doc, &r)
	case "sale", "purchase":
		err = s.trade(ctx, tx, input, p, &doc, &r)
	case "payroll":
		doc.SourceID = input.EmployeeID + ":" + input.Date[:7]
		err = s.payroll(ctx, tx, input, p, &doc, &r)
	case "asset":
		err = s.registerAsset(ctx, tx, input, &doc, &r)
	case "depreciation":
		doc.SourceID = input.AssetID + ":" + input.Date[:7]
		err = s.depreciate(ctx, tx, input, p, &doc, &r)
	case "costing":
		err = s.costing(ctx, tx, input, &doc, &r)
	case "closing":
		var generation int
		if err = tx.QueryRowContext(ctx, "SELECT count(*)+1 FROM operations WHERE kind='closing' AND substr(date,1,7)=?", input.Date[:7]).Scan(&generation); err != nil {
			return r, err
		}
		doc.SourceID = fmt.Sprintf("%s:%d", input.Date[:7], generation)
		err = s.closing(ctx, tx, input, &doc, &r)
	case "corporate_tax":
		doc.SourceID = input.TaxFrom + ":" + input.Date
		err = s.corporateTax(ctx, tx, input, p, &doc, &r)
	default:
		err = accounting.Fail("VALIDATION", "Loại nghiệp vụ tự động không hợp lệ.")
	}
	if err != nil {
		return r, err
	}
	if err = recordBankMovements(ctx, tx, input, doc); err != nil {
		return r, err
	}
	doc, err = s.Ledger.SaveTx(ctx, tx, doc)
	if err != nil {
		return r, err
	}
	if input.Kind == "asset" {
		a := input.Asset
		cost, _ := accounting.Money(a.Cost)
		residual, _ := accounting.Money(a.Residual)
		if _, err = tx.ExecContext(ctx, "INSERT INTO assets VALUES(?,?,?,?,?,?,?,?,?,0)", a.ID, a.Code, a.Name, a.Kind, cost, residual, a.Months, a.StartPeriod, doc.ID); err != nil {
			return r, err
		}
	}
	if input.DraftID != "" {
		b, e := json.Marshal(input)
		if e != nil {
			return r, e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE voucher_inputs SET payload=? WHERE document_id=?", string(b), doc.ID); e != nil {
			return r, e
		}
	}
	r.Document, err = s.Ledger.PostTx(ctx, tx, ledger.Post{ID: doc.ID, Version: doc.Version})
	if err != nil {
		return r, err
	}
	if input.Kind == "closing" {
		if _, err = tx.ExecContext(ctx, "UPDATE periods SET closed=1 WHERE period=?", input.Date[:7]); err != nil {
			return r, err
		}
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return r, err
	}
	calculation, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO operations VALUES(?,?,?,?,?,?,?,?)", sqlite.ID(), input.Kind, doc.SourceID, input.Date, doc.ID, p.ID, string(payload), string(calculation))
	if err != nil {
		return r, err
	}
	if (input.Kind == "sale" || input.Kind == "purchase") && input.PaymentAccount != "" {
		kind := "receipt"
		if input.Kind == "purchase" {
			kind = "payment"
		}
		payment, e := s.run(ctx, tx, Input{BankAccountID: input.BankAccountID, Kind: kind, SourceID: "settlement:" + doc.ID, Date: input.Date, Description: input.Description, PartnerID: input.PartnerID, MoneyAccount: input.PaymentAccount, Amount: r.Amounts["total"], Allocations: []Allocation{{DocumentID: doc.ID, Amount: r.Amounts["total"]}}, Voucher: input.Voucher})
		if e != nil {
			return r, e
		}
		r.PaymentDocument = &payment.Document
		calculation, e = json.Marshal(r)
		if e != nil {
			return r, e
		}
		_, err = tx.ExecContext(ctx, "UPDATE operations SET calculation=? WHERE document_id=?", string(calculation), doc.ID)
	}
	return r, err
}
func addLine(doc *ledger.Document, account, partner, description string, debit, credit int64) {
	if debit == 0 && credit == 0 {
		return
	}
	doc.Lines = append(doc.Lines, ledger.Line{AccountID: account, PartnerID: partner, Description: description, Debit: accounting.Text(debit), Credit: accounting.Text(credit)})
}
