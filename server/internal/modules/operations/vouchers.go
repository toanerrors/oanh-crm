package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
)

type Voucher struct {
	Context     *ledger.Context `json:"context,omitempty"`
	Debit       string          `json:"debit,omitempty"`
	Credit      string          `json:"credit,omitempty"`
	Calculation *Result         `json:"calculation,omitempty"`
	Input       Input           `json:"input"`
	Document    ledger.Document `json:"document"`
}

func voucherKind(kind string) bool {
	return kind == "receipt" || kind == "payment" || kind == "transfer" || kind == "sale" || kind == "purchase"
}

func validateVoucher(in Input) error {
	v := in.Voucher
	if v.OriginalCount != "" {
		n, err := accounting.Money(v.OriginalCount)
		if err != nil || n > 10000 {
			return accounting.Fail("VALIDATION", "Số chứng từ gốc kèm theo phải là số nguyên từ 0 đến 10.000.")
		}
	}
	if v.DocumentDate != "" && !accounting.Date(v.DocumentDate) {
		return accounting.Fail("VALIDATION", "Ngày chứng từ không hợp lệ.")
	}
	for _, text := range []string{v.Contact, v.Address, v.TaxCode, v.Employee, v.BankNumber, v.BankName, v.DestinationBankNumber, v.DestinationBankName, v.Reference, v.ContractNumber, v.InvoiceLookupCode, v.InvoiceLookupURL, v.OriginalCount} {
		if len(text) > 1000 {
			return accounting.Fail("VALIDATION", "Thông tin chứng từ vượt độ dài cho phép.")
		}
	}
	if len(v.Notes) > 5000 {
		return accounting.Fail("VALIDATION", "Ghi chú tối đa 5.000 byte.")
	}
	for _, row := range in.Lines {
		if len(row.Description) > 1000 {
			return accounting.Fail("VALIDATION", "Diễn giải dòng quá dài.")
		}
	}
	for _, row := range in.TreasuryLines {
		if len(row.Description) > 1000 {
			return accounting.Fail("VALIDATION", "Diễn giải dòng quá dài.")
		}
	}
	return nil
}

func (s *Service) SaveVoucher(ctx context.Context, cmd ledger.Command, in Input) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "voucher.save", in, func(tx *sql.Tx) (any, error) {
		return s.saveVoucherTx(ctx, tx, in)
	})
}
func (s *Service) saveVoucherTx(ctx context.Context, tx *sql.Tx, in Input) (any, error) {
	if !voucherKind(in.Kind) {
		return nil, accounting.Fail("VALIDATION", "Loại chứng từ không hỗ trợ lưu nháp nghiệp vụ.")
	}
	if err := normalizeBanks(ctx, tx, &in); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT voucher_preview"); err != nil {
		return nil, err
	}
	result, err := s.run(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "ROLLBACK TO voucher_preview"); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "RELEASE voucher_preview"); err != nil {
		return nil, err
	}
	doc := result.Document
	doc.Version = in.DraftVersion
	doc.Reference = "NHAP-" + strings.ToUpper(doc.ID[:8])
	doc.Status = "draft"
	doc, err = s.Ledger.SaveTx(ctx, tx, doc)
	if err != nil {
		return nil, err
	}
	in.DraftID, in.DraftVersion = doc.ID, doc.Version
	in.ExpectedRevision = nil
	in.ExpectedPolicyID = ""
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO voucher_inputs VALUES(?,?) ON CONFLICT(document_id) DO UPDATE SET payload=excluded.payload", doc.ID, string(payload)); err != nil {
		return nil, err
	}
	doc.Voucher = true
	return Voucher{Input: in, Document: doc}, nil
}

func (s *Service) Voucher(ctx context.Context, id string) (Voucher, error) {
	var result Voucher
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	result.Document, err = ledger.ReadDocument(ctx, tx, id)
	if err != nil {
		return result, err
	}
	var payload string
	err = tx.QueryRowContext(ctx, "SELECT payload FROM voucher_inputs WHERE document_id=? UNION ALL SELECT payload FROM operations WHERE document_id=? LIMIT 1", id, id).Scan(&payload)
	if err == sql.ErrNoRows {
		return result, accounting.Fail("NOT_FOUND", "Chứng từ này không có biểu mẫu nghiệp vụ đã lưu.")
	}
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal([]byte(payload), &result.Input); err != nil {
		return result, err
	}
	if !voucherKind(result.Input.Kind) && !(result.Input.Kind == "closing" && result.Document.Status == "posted") {
		return result, accounting.Fail("INVALID_SOURCE", "Loại nghiệp vụ này dùng màn hình chuyên biệt.")
	}
	result.Input.DraftID, result.Input.DraftVersion = result.Document.ID, result.Document.Version
	if result.Document.Status == "posted" {
		var calculation string
		if err = tx.QueryRowContext(ctx, "SELECT calculation FROM operations WHERE document_id=?", id).Scan(&calculation); err != nil {
			return result, err
		}
		result.Calculation = &Result{}
		if err = json.Unmarshal([]byte(calculation), result.Calculation); err != nil {
			return result, err
		}
	}
	metadata, e := ledger.ReadContext(ctx, tx)
	if e != nil {
		return result, e
	}
	result.Context = &metadata
	debit, credit := new(big.Int), new(big.Int)
	for _, line := range result.Document.Lines {
		d, _ := new(big.Int).SetString(line.Debit, 10)
		c, _ := new(big.Int).SetString(line.Credit, 10)
		debit.Add(debit, d)
		credit.Add(credit, c)
	}
	result.Debit, result.Credit = debit.String(), credit.String()
	return result, tx.Commit()
}
