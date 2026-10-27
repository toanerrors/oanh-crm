package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
)

func (s *Service) SaveAsset(ctx context.Context, cmd ledger.Command, a Asset) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "admin.asset.save", a, func(tx *sql.Tx) (any, error) {
		return s.saveAssetTx(ctx, tx, a)
	})
}

func (s *Service) saveAssetTx(ctx context.Context, tx *sql.Tx, a Asset) (any, error) {
	a.Code = strings.ToUpper(strings.TrimSpace(a.Code))
	a.Name = strings.TrimSpace(a.Name)
	cost, err := accounting.Money(a.Cost)
	if err != nil {
		return nil, err
	}
	residual, err := accounting.Money(a.Residual)
	if err != nil {
		return nil, err
	}
	if !accounting.ID(a.ID) || !accounting.ID(a.Code) || a.Name == "" || a.Kind != "fixed" && a.Kind != "prepaid" || cost == 0 || residual >= cost || a.Months < 1 || a.Months > 1200 || !accounting.Period(a.StartPeriod) {
		return nil, accounting.Fail("VALIDATION", "Thông tin tài sản/khoản phân bổ không hợp lệ.")
	}
	if cost-residual < int64(a.Months) {
		return nil, accounting.Fail("INVALID_ALLOCATION", "Giá trị phân bổ phải đủ ít nhất một đồng cho mỗi kỳ.")
	}
	var oldCode, oldName, oldKind, oldStart, recognizedDoc string
	var oldCost, oldResidual, allocated int64
	var oldMonths int
	err = tx.QueryRowContext(ctx, "SELECT code,name,kind,cost,residual,months,start_period,recognized_document,allocated FROM assets WHERE id=?", a.ID).Scan(
		&oldCode, &oldName, &oldKind, &oldCost, &oldResidual, &oldMonths, &oldStart, &recognizedDoc, &allocated,
	)
	if err == sql.ErrNoRows {
		return nil, accounting.Fail("NOT_FOUND", "Không tìm thấy tài sản/khoản phân bổ.")
	}
	if err != nil {
		return nil, err
	}
	if allocated > cost-residual {
		return nil, accounting.Fail("INVALID_ALLOCATION", "Giá trị còn lại cần phân bổ không được nhỏ hơn số đã phân bổ.")
	}
	var dup int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM assets WHERE code=? COLLATE NOCASE AND id<>?", a.Code, a.ID).Scan(&dup); err != nil {
		return nil, err
	}
	if dup > 0 {
		return nil, accounting.Fail("DUPLICATE_CODE", "Mã tài sản/khoản phân bổ đã tồn tại.")
	}
	if oldCost != cost || oldKind != a.Kind || oldStart != a.StartPeriod {
		var depCount int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM operations WHERE kind='depreciation' AND substr(source_id,1,length(?))=?", a.ID+":", a.ID+":").Scan(&depCount); err != nil {
			return nil, err
		}
		if depCount > 0 {
			return nil, accounting.Fail("HISTORY_LOCKED", "Tài sản đã phát sinh khấu hao/phân bổ; không đổi nguyên giá, phân loại hoặc kỳ bắt đầu.")
		}
		doc, err := ledger.ReadDocument(ctx, tx, recognizedDoc)
		if err != nil {
			return nil, err
		}
		if ledger.IsOpening(doc.SourceType) {
			return nil, accounting.Fail("SUBLEDGER_REQUIRED", "Tài sản đầu kỳ cần điều chỉnh nguyên giá/loại/kỳ bắt đầu tại màn hình Số dư đầu kỳ.")
		}
		if a.StartPeriod < doc.Date[:7] {
			return nil, accounting.Fail("VALIDATION", "Kỳ bắt đầu không được trước kỳ ghi tăng.")
		}
		var closed bool
		if err = tx.QueryRowContext(ctx, "SELECT closed FROM periods WHERE period=?", doc.Date[:7]).Scan(&closed); err == sql.ErrNoRows || closed {
			return nil, accounting.Fail("PERIOD_CLOSED", "Kỳ kế toán ghi tăng tài sản đã khóa.")
		} else if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE documents SET status='draft' WHERE id=?", doc.ID); err != nil {
			return nil, err
		}
		account := "211"
		if a.Kind == "prepaid" {
			account = "242"
		}
		doc.Lines = []ledger.Line{
			{AccountID: account, Description: "Ghi tăng: " + a.Name, Debit: a.Cost, Credit: "0"},
			{AccountID: "331", PartnerID: doc.PartnerID, Description: "Phải trả ghi tăng tài sản", Debit: "0", Credit: a.Cost},
		}
		doc, err = s.Ledger.SaveTx(ctx, tx, doc)
		if err != nil {
			return nil, err
		}
		doc, err = s.Ledger.PostTx(ctx, tx, ledger.Post{ID: doc.ID, Version: doc.Version})
		if err != nil {
			return nil, err
		}
		var rawPayload, rawCalc string
		if err = tx.QueryRowContext(ctx, "SELECT payload,calculation FROM operations WHERE document_id=?", doc.ID).Scan(&rawPayload, &rawCalc); err == nil {
			var opIn Input
			var opRes Result
			if json.Unmarshal([]byte(rawPayload), &opIn) == nil && json.Unmarshal([]byte(rawCalc), &opRes) == nil {
				opIn.Asset = a
				opIn.Asset.Allocated = accounting.Text(allocated)
				if opRes.Amounts == nil {
					opRes.Amounts = map[string]string{}
				}
				opRes.Amounts["cost"] = a.Cost
				opRes.Document = doc
				newP, _ := json.Marshal(opIn)
				newC, _ := json.Marshal(opRes)
				if _, err = tx.ExecContext(ctx, "UPDATE operations SET payload=?,calculation=? WHERE document_id=?", string(newP), string(newC), doc.ID); err != nil {
					return nil, err
				}
			}
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE assets SET code=?,name=?,kind=?,cost=?,residual=?,months=?,start_period=? WHERE id=?", a.Code, a.Name, a.Kind, cost, residual, a.Months, a.StartPeriod, a.ID); err != nil {
		return nil, err
	}
	a.Allocated = accounting.Text(allocated)
	return a, nil
}

func (s *Service) registerAsset(_ context.Context, _ *sql.Tx, in Input, doc *ledger.Document, result *Result) error {
	a := in.Asset
	cost, err := accounting.Money(a.Cost)
	if err != nil {
		return err
	}
	residual, err := accounting.Money(a.Residual)
	if err != nil {
		return err
	}
	if !accounting.ID(a.ID) || !accounting.ID(a.Code) || strings.TrimSpace(a.Name) == "" || a.Kind != "fixed" && a.Kind != "prepaid" || cost == 0 || residual >= cost || a.Months < 1 || a.Months > 1200 || !accounting.Period(a.StartPeriod) || a.StartPeriod < in.Date[:7] {
		return accounting.Fail("VALIDATION", "Thông tin tài sản/khoản phân bổ không hợp lệ; kỳ bắt đầu không được trước kỳ ghi tăng.")
	}
	if cost-residual < int64(a.Months) {
		return accounting.Fail("INVALID_ALLOCATION", "Giá trị phân bổ phải đủ ít nhất một đồng cho mỗi kỳ.")
	}
	account := "211"
	if a.Kind == "prepaid" {
		account = "242"
	}
	addLine(doc, account, "", "Ghi tăng: "+a.Name, cost, 0)
	addLine(doc, "331", in.PartnerID, "Phải trả ghi tăng tài sản", 0, cost)
	result.Amounts["cost"] = a.Cost
	return nil
}
func (s *Service) depreciate(ctx context.Context, tx *sql.Tx, in Input, p Policy, doc *ledger.Document, result *Result) error {
	var a Asset
	var cost, residual, allocated int64
	err := tx.QueryRowContext(ctx, "SELECT id,name,kind,cost,residual,months,start_period,allocated FROM current_assets WHERE id=?", in.AssetID).Scan(&a.ID, &a.Name, &a.Kind, &cost, &residual, &a.Months, &a.StartPeriod, &allocated)
	if err == sql.ErrNoRows {
		return accounting.Fail("NOT_FOUND", "Không tìm thấy tài sản/khoản phân bổ.")
	}
	if err != nil {
		return err
	}
	start, _ := time.Parse("2006-01", a.StartPeriod)
	current, _ := time.Parse("2006-01", in.Date[:7])
	elapsed := (current.Year()-start.Year())*12 + int(current.Month()-start.Month())
	if elapsed < 0 || elapsed >= a.Months {
		return accounting.Fail("INVALID_PERIOD", "Kỳ nằm ngoài thời gian khấu hao/phân bổ.")
	}
	var previous int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM operations WHERE kind='depreciation' AND substr(source_id,1,length(?))=?", a.ID+":", a.ID+":").Scan(&previous); err != nil {
		return err
	}
	var imported int
	var cutoff, startPeriod string
	err = tx.QueryRowContext(ctx, "SELECT json_extract(j.value,'$.startPeriod'),json_extract(o.payload,'$.cutoff') FROM opening_current o,json_each(o.payload,'$.assets') j WHERE json_extract(j.value,'$.id')=?", a.ID).Scan(&startPeriod, &cutoff)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		imported = openingElapsed(startPeriod, cutoff)
	}
	if previous+imported != elapsed {
		return accounting.Fail("ALLOCATION_SEQUENCE", "Phải tính liên tiếp từng kỳ, không bỏ kỳ hoặc chạy lặp.")
	}
	base := (cost - residual) / int64(a.Months)
	amount := base
	if elapsed == a.Months-1 {
		amount = cost - residual - allocated
	}
	if amount <= 0 {
		return accounting.Fail("INVALID_ALLOCATION", "Giá trị phân bổ kỳ bằng 0; cần điều chỉnh số kỳ trong chính sách.")
	}
	if allocated > cost-residual-amount {
		return accounting.Fail("AMOUNT_OVERFLOW", "Lũy kế vượt nguyên giá được phân bổ.")
	}
	account := "214"
	if a.Kind == "prepaid" {
		account = "242"
	}
	addLine(doc, "642", "", "Khấu hao/phân bổ: "+a.Name, amount, 0)
	addLine(doc, account, "", a.Name, 0, amount)
	if _, err = tx.ExecContext(ctx, "UPDATE assets SET allocated=allocated+? WHERE id=?", amount, a.ID); err != nil {
		return err
	}
	result.Amounts = map[string]string{"amount": accounting.Text(amount), "accumulated": accounting.Text(allocated + amount), "remaining": accounting.Text(cost - residual - allocated - amount)}
	result.Notes = append(result.Notes, "Phân bổ đường thẳng theo số tháng nguyên; phần dư được ghi nhận tại tháng cuối theo cấu hình.")
	return nil
}
func (s *Service) costing(ctx context.Context, tx *sql.Tx, in Input, doc *ledger.Document, result *Result) error {
	amount, err := accounting.Money(in.Amount)
	if err != nil {
		return err
	}
	if amount <= 0 {
		return accounting.Fail("VALIDATION", "Giá thành kết chuyển phải lớn hơn 0.")
	}
	rows, err := tx.QueryContext(ctx, "SELECT l.debit,l.credit FROM document_lines l JOIN documents d ON d.id=l.document_id WHERE d.status='posted' AND d.date<=? AND l.account_id='154'", in.Date)
	if err != nil {
		return err
	}
	balance := new(big.Int)
	for rows.Next() {
		var d, c int64
		if err = rows.Scan(&d, &c); err != nil {
			rows.Close()
			return err
		}
		balance.Add(balance, big.NewInt(d))
		balance.Sub(balance, big.NewInt(c))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if balance.Cmp(big.NewInt(amount)) < 0 {
		return accounting.Fail("INSUFFICIENT_COST", "Giá thành kết chuyển vượt chi phí dở dang đã ghi nhận.")
	}
	addLine(doc, "632", "", "Giá thành dịch vụ hoàn thành", amount, 0)
	addLine(doc, "154", "", "Kết chuyển chi phí dở dang", 0, amount)
	result.Amounts["amount"] = in.Amount
	return nil
}
func (s *Service) closing(ctx context.Context, tx *sql.Tx, in Input, doc *ledger.Document, result *Result) error {
	sources, e := costSources(ctx, tx, in.Date)
	if e != nil {
		return e
	}
	for _, source := range sources {
		if source.Remaining != "0" {
			return accounting.Fail("UNALLOCATED_PURCHASE_COST", "Còn chi phí mua hàng chưa phân bổ hết; hoàn tất phân bổ trước khi kết chuyển.")
		}
	}
	month, _ := time.Parse("2006-01", in.Date[:7])
	if in.Date != month.AddDate(0, 1, -1).Format("2006-01-02") {
		return accounting.Fail("INVALID_PERIOD", "Kết chuyển kết quả kinh doanh phải dùng ngày cuối kỳ.")
	}
	rows, err := tx.QueryContext(ctx, "SELECT a.id,a.account_group,l.debit,l.credit FROM document_lines l JOIN documents d ON d.id=l.document_id JOIN accounts a ON a.id=l.account_id WHERE d.status='posted' AND substr(d.date,1,7)=? AND a.account_group IN ('revenue','expense') ORDER BY a.code", in.Date[:7])
	if err != nil {
		return err
	}
	balances := map[string]*big.Int{}
	groups := map[string]string{}
	order := []string{}
	for rows.Next() {
		var id, group string
		var d, c int64
		if err = rows.Scan(&id, &group, &d, &c); err != nil {
			rows.Close()
			return err
		}
		if balances[id] == nil {
			balances[id] = new(big.Int)
			order = append(order, id)
			groups[id] = group
		}
		balances[id].Add(balances[id], big.NewInt(d))
		balances[id].Sub(balances[id], big.NewInt(c))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	profit := new(big.Int)
	for _, id := range order {
		value := balances[id]
		if !value.IsInt64() {
			return accounting.Fail("AMOUNT_OVERFLOW", "Số kết chuyển vượt giới hạn int64.")
		}
		n := value.Int64()
		if n < 0 {
			if n == -9223372036854775808 {
				return accounting.Fail("AMOUNT_OVERFLOW", "Số kết chuyển vượt giới hạn.")
			}
			addLine(doc, id, "", "Kết chuyển "+map[string]string{"revenue": "doanh thu", "expense": "chi phí"}[groups[id]], -n, 0)
			addLine(doc, "911", "", "Xác định kết quả", 0, -n)
		} else {
			addLine(doc, "911", "", "Xác định kết quả", n, 0)
			addLine(doc, id, "", "Kết chuyển "+map[string]string{"revenue": "doanh thu", "expense": "chi phí"}[groups[id]], 0, n)
		}
		profit.Sub(profit, value)
	}
	if !profit.IsInt64() {
		return accounting.Fail("AMOUNT_OVERFLOW", "Kết quả kinh doanh vượt giới hạn int64.")
	}
	n := profit.Int64()
	if n >= 0 {
		addLine(doc, "911", "", "Kết chuyển lợi nhuận", n, 0)
		addLine(doc, "421", "", "Lợi nhuận sau kết chuyển", 0, n)
	} else {
		if n == -9223372036854775808 {
			return accounting.Fail("AMOUNT_OVERFLOW", "Kết quả vượt giới hạn.")
		}
		addLine(doc, "421", "", "Kết chuyển lỗ", -n, 0)
		addLine(doc, "911", "", "Kết chuyển lỗ", 0, -n)
	}
	result.Amounts["result"] = profit.String()
	result.Notes = append(result.Notes, "Kết chuyển theo tài khoản đã phân nhóm; thuế TNDN cần được ghi nhận riêng trước khi kết chuyển.")
	return nil
}
