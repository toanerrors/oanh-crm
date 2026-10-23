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

type BankAccount struct {
	HasReferences bool   `json:"hasReferences,omitempty"`
	ID            string `json:"id"`
	Number        string `json:"number"`
	BankName      string `json:"bankName"`
	AccountID     string `json:"accountId"`
	Active        bool   `json:"active"`
	Opening       string `json:"opening,omitempty"`
	Receipts      string `json:"receipts,omitempty"`
	Payments      string `json:"payments,omitempty"`
	Closing       string `json:"closing,omitempty"`
}
type BankList struct {
	ledger.Context
	Accounts []BankAccount `json:"accounts"`
}

func (s *Service) SaveBank(ctx context.Context, cmd ledger.Command, b BankAccount) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "bank.save", b, func(tx *sql.Tx) (any, error) {
		return s.saveBankTx(ctx, tx, b)
	})
}
func (s *Service) saveBankTx(ctx context.Context, tx *sql.Tx, b BankAccount) (any, error) {
	if b.Opening != "" || b.Receipts != "" || b.Payments != "" || b.Closing != "" {
		return nil, accounting.Fail("DERIVED_FIELD", "Không được tự đặt số dư tài khoản ngân hàng.")
	}
	if !accounting.ID(b.ID) || b.Number == "" || b.Number != strings.TrimSpace(b.Number) || len(b.Number) > 100 || strings.TrimSpace(b.BankName) == "" || len(b.BankName) > 200 {
		return nil, accounting.Fail("VALIDATION", "Nhập số tài khoản và tên ngân hàng hợp lệ.")
	}
	var old BankAccount
	err := tx.QueryRowContext(ctx, "SELECT number,bank_name,account_id FROM bank_accounts WHERE id=?", b.ID).Scan(&old.Number, &old.BankName, &old.AccountID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	var group string
	var active bool
	if err = tx.QueryRowContext(ctx, "SELECT account_group,active FROM accounts WHERE id=?", b.AccountID).Scan(&group, &active); err != nil || !active && old.AccountID != b.AccountID || group != "cash" || b.AccountID == "111" {
		return nil, accounting.Fail("INVALID_ACCOUNT", "Chọn tài khoản kế toán tiền gửi còn hiệu lực.")
	}
	b.HasReferences, err = catalogHasReferences(ctx, tx, bankReferences, b.ID)
	if err != nil {
		return nil, err
	}
	if b.HasReferences && (old.Number != b.Number || old.AccountID != b.AccountID) {
		return nil, accounting.Fail("HISTORY_LOCKED", "Đã có chứng từ hoặc số dư ngân hàng: giữ số tài khoản và tài khoản kế toán. Bạn vẫn sửa được tên ngân hàng và trạng thái sử dụng.")
	}
	var duplicate int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM bank_accounts WHERE number=? AND id<>?", b.Number, b.ID).Scan(&duplicate); err != nil {
		return nil, err
	}
	if duplicate > 0 {
		return nil, accounting.Fail("DUPLICATE_CODE", "Số tài khoản ngân hàng đã tồn tại.")
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO bank_accounts VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET number=excluded.number,bank_name=excluded.bank_name,account_id=excluded.account_id,active=excluded.active", b.ID, b.Number, b.BankName, b.AccountID, b.Active)
	return b, err
}
func bankAt(ctx context.Context, q ledger.Queryer, id string) (BankAccount, error) {
	var b BankAccount
	err := q.QueryRowContext(ctx, "SELECT b.id,b.number,b.bank_name,b.account_id,b.active FROM bank_accounts b JOIN accounts a ON a.id=b.account_id WHERE b.id=? AND a.active=1 AND a.account_group='cash'", id).Scan(&b.ID, &b.Number, &b.BankName, &b.AccountID, &b.Active)
	if err != nil || !b.Active {
		return b, accounting.Fail("INVALID_BANK", "Tài khoản ngân hàng không tồn tại hoặc đã ngừng sử dụng.")
	}
	return b, nil
}
func (s *Service) Banks(ctx context.Context, period string) (BankList, error) {
	result := BankList{Accounts: []BankAccount{}}
	if !accounting.Period(period) {
		return result, accounting.Fail("BAD_REQUEST", "Kỳ tra cứu ngân hàng không hợp lệ.")
	}
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	result.Context, err = ledger.ReadContext(ctx, tx)
	if err != nil {
		return result, err
	}
	result.Period = period
	rows, err := tx.QueryContext(ctx, "SELECT id,number,bank_name,account_id,active FROM bank_accounts ORDER BY bank_name,number")
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var b BankAccount
		if err = rows.Scan(&b.ID, &b.Number, &b.BankName, &b.AccountID, &b.Active); err != nil {
			rows.Close()
			return result, err
		}
		result.Accounts = append(result.Accounts, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	usage, err := readCatalogReferences(ctx, tx, bankReferences)
	if err != nil {
		return result, err
	}
	for index := range result.Accounts {
		result.Accounts[index].HasReferences = usage[result.Accounts[index].ID]
	}
	start, _ := time.Parse("2006-01", period)
	end := start.AddDate(0, 1, 0).Format("2006-01-02")
	for i := range result.Accounts {
		b := &result.Accounts[i]
		opening, receipts, payments := new(big.Int), new(big.Int), new(big.Int)
		movements, e := tx.QueryContext(ctx, "SELECT d.date,d.source_type,m.debit,m.credit FROM bank_movements m JOIN documents d ON d.id=m.document_id WHERE m.bank_id=? AND d.status='posted' AND d.date<?", b.ID, end)
		if e != nil {
			return result, e
		}
		for movements.Next() {
			var date, source string
			var debit, credit int64
			if e = movements.Scan(&date, &source, &debit, &credit); e != nil {
				movements.Close()
				return result, e
			}
			if ledger.IsOpening(source) || date < start.Format("2006-01-02") {
				opening.Add(opening, big.NewInt(debit))
				opening.Sub(opening, big.NewInt(credit))
			} else {
				receipts.Add(receipts, big.NewInt(debit))
				payments.Add(payments, big.NewInt(credit))
			}
		}
		e = movements.Err()
		movements.Close()
		if e != nil {
			return result, e
		}
		b.Opening, b.Receipts, b.Payments, b.Closing = opening.String(), receipts.String(), payments.String(), new(big.Int).Sub(new(big.Int).Add(opening, receipts), payments).String()
	}
	return result, tx.Commit()
}
func normalizeBanks(ctx context.Context, tx *sql.Tx, in *Input) error {
	if in.BankAccountID != "" {
		if !voucherKind(in.Kind) {
			return accounting.Fail("VALIDATION", "Loại nghiệp vụ không nhận tài khoản ngân hàng.")
		}
		b, err := bankAt(ctx, tx, in.BankAccountID)
		if err != nil {
			return err
		}
		account := &in.MoneyAccount
		if in.Kind == "sale" || in.Kind == "purchase" {
			account = &in.PaymentAccount
		}
		if *account != "" && *account != b.AccountID {
			return accounting.Fail("INVALID_BANK", "Tài khoản kế toán không khớp tài khoản ngân hàng.")
		}
		*account = b.AccountID
		in.Voucher.BankNumber, in.Voucher.BankName = b.Number, b.BankName
	}
	if in.DestinationBankAccountID != "" {
		if in.Kind != "transfer" {
			return accounting.Fail("VALIDATION", "Tài khoản nhận chỉ dùng khi chuyển tiền.")
		}
		b, err := bankAt(ctx, tx, in.DestinationBankAccountID)
		if err != nil {
			return err
		}
		if in.DestinationAccount != "" && in.DestinationAccount != b.AccountID {
			return accounting.Fail("INVALID_BANK", "Tài khoản kế toán nhận không khớp ngân hàng.")
		}
		in.DestinationAccount = b.AccountID
		in.Voucher.DestinationBankNumber, in.Voucher.DestinationBankName = b.Number, b.BankName
	}
	return nil
}
func recordBankMovements(ctx context.Context, tx *sql.Tx, in Input, doc ledger.Document) error {
	if in.Kind == "treasury_reversal" {
		_, err := tx.ExecContext(ctx, "INSERT INTO bank_movements SELECT ?,position,bank_id,credit,debit FROM bank_movements WHERE document_id=?", doc.ID, in.OriginalDocumentID)
		return err
	}
	if in.Kind != "receipt" && in.Kind != "payment" && in.Kind != "transfer" {
		return nil
	}
	n, err := accounting.Money(in.Amount)
	if err != nil {
		return err
	}
	position := 0
	add := func(id string, debit, credit int64) error {
		if id == "" {
			return nil
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO bank_movements VALUES(?,?,?,?,?)", doc.ID, position, id, debit, credit)
		position++
		return e
	}
	if in.Kind == "receipt" {
		return add(in.BankAccountID, n, 0)
	}
	if err = add(in.BankAccountID, 0, n); err != nil {
		return err
	}
	if in.Kind == "transfer" {
		return add(in.DestinationBankAccountID, n, 0)
	}
	return nil
}
