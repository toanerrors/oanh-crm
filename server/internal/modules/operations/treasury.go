package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
)

func moneyAccount(id string) bool { return id == "111" || id == "1121" || id == "112" }

func (s *Service) reverseTreasury(ctx context.Context, tx *sql.Tx, in Input, doc *ledger.Document, result *Result) error {
	original, err := ledger.ReadDocument(ctx, tx, in.OriginalDocumentID)
	if err != nil {
		return err
	}
	var kind string
	if err = tx.QueryRowContext(ctx, "SELECT kind FROM operations WHERE document_id=?", original.ID).Scan(&kind); err != nil || kind != "receipt" && kind != "payment" && kind != "transfer" {
		return accounting.Fail("INVALID_SOURCE", "Chỉ đảo phiếu thu, chi hoặc chuyển tiền được lập qua nghiệp vụ tiền.")
	}
	if original.Status != "posted" || in.Date < original.Date {
		return accounting.Fail("VALIDATION", "Ngày đảo không được trước ngày phiếu tiền đã ghi sổ.")
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE reversal_of=?", original.ID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return accounting.Fail("DUPLICATE_SOURCE", "Phiếu tiền đã được đảo.")
	}
	doc.ReversalOf, doc.PartnerID = original.ID, original.PartnerID
	var amount int64
	for _, line := range original.Lines {
		line.Debit, line.Credit = line.Credit, line.Debit
		doc.Lines = append(doc.Lines, line)
		n, _ := accounting.Money(line.Debit)
		if amount, err = accounting.Add(amount, n); err != nil {
			return err
		}
	}
	result.Amounts = map[string]string{"total": accounting.Text(amount)}
	result.Notes = append(result.Notes, "Đảo toàn bộ phiếu tiền, giữ chứng từ gốc và trả lại công nợ theo ngày đảo.")
	return nil
}

func (s *Service) treasury(ctx context.Context, tx *sql.Tx, in Input, doc *ledger.Document, result *Result) error {
	amount, err := accounting.Money(in.Amount)
	if err != nil {
		return err
	}
	if amount == 0 || (!moneyAccount(in.MoneyAccount) && in.BankAccountID == "") {
		return accounting.Fail("VALIDATION", "Chọn tài khoản tiền và nhập số tiền lớn hơn 0.")
	}
	if len(in.TreasuryLines) > 0 {
		if in.Kind == "transfer" || len(in.Allocations) > 0 || in.CounterAccount != "" || len(in.TreasuryLines) > 100 {
			return accounting.Fail("VALIDATION", "Không trộn dòng thu/chi với chuyển tiền hoặc phân bổ công nợ.")
		}
		var total int64
		for _, row := range in.TreasuryLines {
			n, e := accounting.Money(row.Amount)
			if e != nil {
				return e
			}
			if n == 0 {
				return accounting.Fail("VALIDATION", "Số tiền mỗi dòng phải lớn hơn 0.")
			}
			total, e = accounting.Add(total, n)
			if e != nil {
				return e
			}
			child := in
			child.Amount, child.CounterAccount, child.PartnerID, child.Description, child.TreasuryLines = row.Amount, row.CounterAccount, row.PartnerID, row.Description, nil
			if child.PartnerID == "" {
				child.PartnerID = in.PartnerID
			}
			if child.Description == "" {
				child.Description = in.Description
			}
			if e = s.treasury(ctx, tx, child, doc, &Result{}); e != nil {
				return e
			}
		}
		if total != amount {
			return accounting.Fail("AMOUNT_MISMATCH", "Tổng tiền phải bằng tổng các dòng thu/chi.")
		}
		result.Amounts = map[string]string{"total": accounting.Text(total)}
		return nil
	}
	if in.Kind == "transfer" {
		if (!moneyAccount(in.DestinationAccount) && in.DestinationBankAccountID == "") || in.DestinationAccount == in.MoneyAccount && (in.BankAccountID == "" || in.DestinationBankAccountID == "" || in.BankAccountID == in.DestinationBankAccountID) || len(in.Allocations) != 0 || in.PartnerID != "" || in.CounterAccount != "" {
			return accounting.Fail("VALIDATION", "Chọn hai tài khoản tiền khác nhau khi chuyển tiền nội bộ.")
		}
		addLine(doc, in.DestinationAccount, "", in.Description, amount, 0)
		addLine(doc, in.MoneyAccount, "", in.Description, 0, amount)
	} else {
		doc.Type = in.Kind
		counter := in.CounterAccount
		if len(in.Allocations) > 0 {
			if len(in.Allocations) > 100 || in.PartnerID == "" || counter != "" {
				return accounting.Fail("VALIDATION", "Thanh toán cần một đối tượng và tối đa 100 chứng từ nguồn; tài khoản công nợ do server xác định.")
			}
			kind := "sale"
			counter = "131"
			if in.Kind == "payment" {
				kind, counter = "purchase", "331"
			}
			var total int64
			seen := map[string]bool{}
			for _, allocation := range in.Allocations {
				if seen[allocation.DocumentID] {
					return accounting.Fail("VALIDATION", "Một chứng từ nguồn chỉ được phân bổ một lần trong phiếu.")
				}
				seen[allocation.DocumentID] = true
				invoice, e := invoiceAt(ctx, tx, allocation.DocumentID, in.Date)
				if e != nil {
					return e
				}
				if invoice.Kind != kind || invoice.PartnerID != in.PartnerID || invoice.Date > in.Date {
					return accounting.Fail("INVALID_INVOICE", "Chứng từ nguồn phải cùng đối tượng, đúng loại và không sau ngày thanh toán.")
				}
				if invoice.Unallocated {
					return accounting.Fail("UNALLOCATED_SETTLEMENT", "Nguồn công nợ hoặc phân bổ cần đối soát, hoặc đã có thanh toán sau ngày đã chọn. Kiểm tra trước khi lập phiếu tự động.")
				}
				n, e := accounting.Money(allocation.Amount)
				if e != nil {
					return e
				}
				remaining, _ := accounting.Money(invoice.Remaining)
				if n == 0 || n > remaining {
					return accounting.Fail("OVERPAYMENT", "Số phân bổ phải lớn hơn 0 và không vượt công nợ còn lại của chứng từ.")
				}
				if total, e = accounting.Add(total, n); e != nil {
					return e
				}
			}
			if total != amount {
				return accounting.Fail("ALLOCATION_MISMATCH", "Tổng phân bổ phải bằng đúng số tiền thu/chi.")
			}
		} else {
			var group string
			var active bool
			if err = tx.QueryRowContext(ctx, "SELECT account_group,active FROM accounts WHERE id=?", counter).Scan(&group, &active); err != nil || !active {
				return accounting.Fail("INVALID_ACCOUNT", "Chọn tài khoản đối ứng còn hiệu lực.")
			}
			if moneyAccount(counter) || group == "cash" || group == "inventory" || group == "asset" || group == "receivable" || group == "payable" {
				return accounting.Fail("SUBLEDGER_REQUIRED", "Thu/trả công nợ cần chọn chứng từ nguồn; chuyển tiền, kho và tài sản cần dùng nghiệp vụ tương ứng.")
			}
		}
		if in.Kind == "receipt" {
			addLine(doc, in.MoneyAccount, "", in.Description, amount, 0)
			addLine(doc, counter, in.PartnerID, in.Description, 0, amount)
		} else {
			addLine(doc, counter, in.PartnerID, in.Description, amount, 0)
			addLine(doc, in.MoneyAccount, "", in.Description, 0, amount)
		}
	}
	result.Amounts = map[string]string{"total": accounting.Text(amount)}
	result.Notes = append(result.Notes, "Phiếu tiền và phân bổ công nợ được ghi nhận cùng transaction; không tự hạch toán chênh lệch hoặc chiết khấu.")
	return nil
}

type TradeInvoice struct {
	DocumentID    string `json:"documentId"`
	Reference     string `json:"reference"`
	Kind          string `json:"kind"`
	Date          string `json:"date"`
	Description   string `json:"description"`
	PartnerID     string `json:"partnerId"`
	PartnerName   string `json:"partnerName"`
	InvoiceNumber string `json:"invoiceNumber"`
	InvoiceSeries string `json:"invoiceSeries"`
	InvoiceDate   string `json:"invoiceDate"`
	DueDate       string `json:"dueDate"`
	Amount        string `json:"amount"`
	Paid          string `json:"paid"`
	Remaining     string `json:"remaining"`
	Unallocated   bool   `json:"unallocated"`
}

type InvoiceList struct {
	ledger.Context
	Invoices  []TradeInvoice `json:"invoices"`
	Total     int            `json:"total"`
	Amount    string         `json:"amount"`
	Paid      string         `json:"paid"`
	Remaining string         `json:"remaining"`
}

func invoiceAt(ctx context.Context, tx *sql.Tx, id, asOf string) (TradeInvoice, error) {
	var invoice TradeInvoice
	var payload string
	err := tx.QueryRowContext(ctx, `SELECT d.id,d.reference,CASE WHEN o.kind='asset' THEN 'purchase' ELSE d.type END,d.date,d.description,COALESCE(d.partner_id,''),COALESCE(p.name,''),COALESCE(o.payload,'{}')
		FROM documents d LEFT JOIN partners p ON p.id=d.partner_id LEFT JOIN operations o ON o.document_id=d.id
		WHERE d.id=? AND d.status='posted' AND (d.type IN ('sale','purchase') OR o.kind='asset') AND d.date<=?
		AND NOT EXISTS(SELECT 1 FROM documents r WHERE r.reversal_of=d.id AND r.status='posted' AND r.date<=?)`, id, asOf, asOf).Scan(&invoice.DocumentID, &invoice.Reference, &invoice.Kind, &invoice.Date, &invoice.Description, &invoice.PartnerID, &invoice.PartnerName, &payload)
	if err == sql.ErrNoRows {
		return invoice, accounting.Fail("INVALID_INVOICE", "Không tìm thấy chứng từ mua/bán đã ghi sổ tại ngày đã chọn.")
	}
	if err != nil {
		return invoice, err
	}
	var input Input
	if err = json.Unmarshal([]byte(payload), &input); err != nil {
		return invoice, err
	}
	invoice.Unallocated = input.Kind != "sale" && input.Kind != "purchase" && input.Kind != "asset"
	invoice.InvoiceNumber, invoice.InvoiceSeries, invoice.InvoiceDate, invoice.DueDate = input.InvoiceNumber, input.InvoiceSeries, input.InvoiceDate, input.DueDate
	account, paymentKind := "131", "receipt"
	if invoice.Kind == "purchase" {
		account, paymentKind = "331", "payment"
	}
	rows, err := tx.QueryContext(ctx, "SELECT debit,credit,COALESCE(partner_id,'') FROM document_lines WHERE document_id=? AND account_id=?", id, account)
	if err != nil {
		return invoice, err
	}
	var amount int64
	for rows.Next() {
		var debit, credit int64
		var partner string
		if err = rows.Scan(&debit, &credit, &partner); err != nil {
			rows.Close()
			return invoice, err
		}
		if partner != invoice.PartnerID {
			rows.Close()
			return invoice, accounting.Fail("INVALID_INVOICE", "Chứng từ công nợ nhiều đối tượng cần được đối soát riêng.")
		}
		if invoice.Kind == "purchase" {
			debit, credit = credit, debit
		}
		if amount, err = accounting.Add(amount, debit, -credit); err != nil {
			rows.Close()
			return invoice, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return invoice, err
	}
	if amount <= 0 {
		return invoice, accounting.Fail("INVALID_INVOICE", "Chứng từ không có công nợ dương theo đối tượng chung.")
	}
	// Read every allocation, including future-dated payments, to prevent backdated overpayment.
	rows, err = tx.QueryContext(ctx, `SELECT o.date,o.payload FROM operations o JOIN documents d ON d.id=o.document_id WHERE o.kind=? AND d.partner_id=? AND d.status='posted'
		AND NOT EXISTS(SELECT 1 FROM documents reversal WHERE reversal.reversal_of=d.id AND reversal.status='posted' AND reversal.date<=?)`, paymentKind, invoice.PartnerID, asOf)
	if err != nil {
		return invoice, err
	}
	var paid int64
	for rows.Next() {
		var date, data string
		if err = rows.Scan(&date, &data); err != nil {
			rows.Close()
			return invoice, err
		}
		var payment Input
		if err = json.Unmarshal([]byte(data), &payment); err != nil {
			rows.Close()
			return invoice, err
		}
		for _, allocation := range payment.Allocations {
			if allocation.DocumentID != id {
				continue
			}
			if date > asOf {
				invoice.Unallocated = true
				continue
			}
			n, e := accounting.Money(allocation.Amount)
			if e != nil {
				rows.Close()
				return invoice, e
			}
			if paid, err = accounting.Add(paid, n); err != nil {
				rows.Close()
				return invoice, err
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return invoice, err
	}
	var unallocated int
	column := "credit"
	if invoice.Kind == "purchase" {
		column = "debit"
	}
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM document_lines l JOIN documents d ON d.id=l.document_id
		WHERE l.account_id=? AND l.partner_id=? AND d.status='posted' AND l.`+column+`>0
		AND NOT EXISTS(SELECT 1 FROM operations o WHERE o.document_id=d.id AND o.kind=? AND json_array_length(o.payload,'$.allocations')>0)`, account, invoice.PartnerID, paymentKind).Scan(&unallocated)
	if err != nil {
		return invoice, err
	}
	invoice.Unallocated = invoice.Unallocated || unallocated > 0
	if paid > amount {
		return invoice, accounting.Fail("ALLOCATION_MISMATCH", "Phân bổ vượt công nợ nguồn; cần đối soát dữ liệu.")
	}
	invoice.Amount, invoice.Paid, invoice.Remaining = accounting.Text(amount), accounting.Text(paid), accounting.Text(amount-paid)
	return invoice, nil
}

func (s *Service) Invoices(ctx context.Context, asOf, kind, partner string, page, size int) (InvoiceList, error) {
	result := InvoiceList{Invoices: []TradeInvoice{}}
	if !accounting.Date(asOf) || kind != "sale" && kind != "purchase" || page < 1 || size < 1 || size > 100 || page > 1000000 {
		return result, accounting.Fail("BAD_REQUEST", "Ngày, loại chứng từ hoặc phân trang không hợp lệ.")
	}
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if result.Context, err = ledger.ReadContext(ctx, tx); err != nil {
		return result, err
	}
	account, column := "131", "debit"
	if kind == "purchase" {
		account, column = "331", "credit"
	}
	rows, err := tx.QueryContext(ctx, `SELECT d.id FROM documents d LEFT JOIN operations o ON o.document_id=d.id
		WHERE d.status='posted' AND (d.type=? OR (?='purchase' AND o.kind='asset')) AND d.date<=? AND (?='' OR d.partner_id=?)
		AND EXISTS(SELECT 1 FROM document_lines l WHERE l.document_id=d.id AND l.account_id=? AND l.`+column+`>0)
		AND NOT EXISTS(SELECT 1 FROM documents r WHERE r.reversal_of=d.id AND r.status='posted' AND r.date<=?)
		ORDER BY d.date DESC,d.posted_order DESC`, kind, kind, asOf, partner, partner, account, asOf)
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
		if len(ids) > 100000 {
			rows.Close()
			return result, accounting.Fail("REPORT_LIMIT", "Tập công nợ vượt 100.000 chứng từ; cần thu hẹp phạm vi.")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	amount, paid, remaining := new(big.Int), new(big.Int), new(big.Int)
	for index, id := range ids {
		invoice, e := invoiceAt(ctx, tx, id, asOf)
		if e != nil {
			return result, e
		}
		for _, v := range []struct {
			text  string
			total *big.Int
		}{{invoice.Amount, amount}, {invoice.Paid, paid}, {invoice.Remaining, remaining}} {
			n, _ := new(big.Int).SetString(v.text, 10)
			v.total.Add(v.total, n)
		}
		if index >= (page-1)*size && index < page*size {
			result.Invoices = append(result.Invoices, invoice)
		}
	}
	result.Total, result.Amount, result.Paid, result.Remaining = len(ids), amount.String(), paid.String(), remaining.String()
	return result, tx.Commit()
}
