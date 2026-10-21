package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
)

func (s *Service) corporateTax(ctx context.Context, tx *sql.Tx, in Input, p Policy, doc *ledger.Document, result *Result) error {
	if !accounting.Date(in.TaxFrom) || in.TaxFrom > in.Date {
		return accounting.Fail("VALIDATION", "Chọn khoảng tính thuế hợp lệ.")
	}
	rows, err := tx.QueryContext(ctx, "SELECT payload FROM operations WHERE kind='corporate_tax'")
	if err != nil {
		return err
	}
	for rows.Next() {
		var raw string
		var old Input
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal([]byte(raw), &old); err != nil {
			rows.Close()
			return err
		}
		if old.TaxFrom <= in.Date && old.Date >= in.TaxFrom {
			rows.Close()
			return accounting.Fail("DUPLICATE_SOURCE", "Khoảng tính thuế đã có tạm tính được ghi nhận. Điều chỉnh chênh lệch bằng chứng từ có kiểm soát.")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	profit := new(big.Int)
	rows, err = tx.QueryContext(ctx, "SELECT a.account_group,l.account_id,l.debit,l.credit FROM document_lines l JOIN documents d ON d.id=l.document_id JOIN accounts a ON a.id=l.account_id WHERE d.status='posted' AND d.date>=? AND d.date<=? AND d.source_type NOT IN ('closing','closing_reversal') AND a.account_group IN ('revenue','expense')", in.TaxFrom, in.Date)
	if err != nil {
		return err
	}
	for rows.Next() {
		var group, account string
		var debit, credit int64
		if err = rows.Scan(&group, &account, &debit, &credit); err != nil {
			rows.Close()
			return err
		}
		if account == "821" {
			continue
		}
		profit.Add(profit, big.NewInt(credit))
		profit.Sub(profit, big.NewInt(debit))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	nonDeductible, err := accounting.Money(in.NonDeductible)
	if err != nil {
		return err
	}
	exempt, err := accounting.Money(in.ExemptIncome)
	if err != nil {
		return err
	}
	loss, err := accounting.Money(in.CarriedLoss)
	if err != nil {
		return err
	}
	taxable := new(big.Int).Add(profit, big.NewInt(nonDeductible))
	taxable.Sub(taxable, big.NewInt(exempt))
	if taxable.Sign() < 0 {
		taxable.SetInt64(0)
	}
	if big.NewInt(loss).Cmp(taxable) > 0 {
		return accounting.Fail("INVALID_TAX", "Lỗ chuyển kỳ không được vượt thu nhập tính thuế trước chuyển lỗ.")
	}
	taxable.Sub(taxable, big.NewInt(loss))
	tax, err := accounting.Percent(new(big.Rat).SetInt(taxable), p.CorporateTaxRate, p.Rounding)
	if err != nil {
		return err
	}
	if tax == 0 {
		return accounting.Fail("ZERO_TAX", "Thuế tạm tính bằng 0; không tạo chứng từ ghi sổ bằng 0. Có thể tra cứu thu nhập trên báo cáo.")
	}
	addLine(doc, "821", "", "Chi phí thuế TNDN tạm tính", tax, 0)
	addLine(doc, "3334", "", "Thuế TNDN tạm phải nộp", 0, tax)
	result.Amounts = map[string]string{"profitBeforeTax": profit.String(), "taxable": taxable.String(), "tax": accounting.Text(tax)}
	result.Notes = append(result.Notes, "Tạm tính theo thuế suất và các điều chỉnh do doanh nghiệp xác nhận; số này chưa phải hồ sơ quyết toán hoặc tờ khai điện tử.")
	return nil
}
