package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

type Voucher struct {
	Replacement *ledger.Document `json:"replacement,omitempty"`
	Reversal    *ledger.Document `json:"reversal,omitempty"`
	Context     *ledger.Context  `json:"context,omitempty"`
	Debit       string           `json:"debit,omitempty"`
	Credit      string           `json:"credit,omitempty"`
	Calculation *Result          `json:"calculation,omitempty"`
	Input       Input            `json:"input"`
	Document    ledger.Document  `json:"document"`
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
	if in.DraftID != "" {
		var status string
		if err := tx.QueryRowContext(ctx, "SELECT status FROM documents WHERE id=?", in.DraftID).Scan(&status); err == nil && status == "posted" {
			return nil, accounting.Fail("DOCUMENT_POSTED", "Chứng từ đã ghi sổ; nhấn Ghi sổ để cập nhật trực tiếp.")
		}
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
	var related string
	if e := tx.QueryRowContext(ctx, "SELECT d.id FROM operations o JOIN documents d ON d.id=o.document_id WHERE json_extract(o.payload,'$.replacementOf')=?", id).Scan(&related); e == nil {
		v, e := ledger.ReadDocument(ctx, tx, related)
		if e != nil {
			return result, e
		}
		result.Replacement = &v
	} else if e != sql.ErrNoRows {
		return result, e
	}
	if e := tx.QueryRowContext(ctx, "SELECT id FROM documents WHERE reversal_of=? AND status='posted'", id).Scan(&related); e == nil {
		v, e := ledger.ReadDocument(ctx, tx, related)
		if e != nil {
			return result, e
		}
		result.Reversal = &v
	} else if e != sql.ErrNoRows {
		return result, e
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

func (s *Service) DiscardVoucher(ctx context.Context, cmd ledger.Command, in ledger.Post) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "voucher.discard", in, func(tx *sql.Tx) (any, error) {
		doc, err := ledger.ReadDocument(ctx, tx, in.ID)
		if err != nil {
			return nil, err
		}
		if doc.Status != "draft" {
			return nil, accounting.Fail("DOCUMENT_POSTED", "Chứng từ đã ghi sổ không được xóa nháp.")
		}
		if doc.Version != in.Version {
			return nil, accounting.Fail("VERSION_CONFLICT", "Bản nháp đã thay đổi. Tải lại trước khi xóa.")
		}
		var dependencies int
		if err = tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM operations WHERE document_id=?)+(SELECT count(*) FROM attachments WHERE document_id=?)+(SELECT count(*) FROM opening_balances WHERE document_id=?)+(SELECT count(*) FROM opening_adjustments WHERE document_id=?)", in.ID, in.ID, in.ID, in.ID).Scan(&dependencies); err != nil {
			return nil, err
		}
		if dependencies > 0 {
			return nil, accounting.Fail("HISTORY_LOCKED", "Bản nháp có liên kết khác; không thể xóa trực tiếp.")
		}
		for _, q := range []string{"DELETE FROM voucher_inputs WHERE document_id=?", "DELETE FROM document_lines WHERE document_id=?", "DELETE FROM documents WHERE id=?"} {
			if _, err = tx.ExecContext(ctx, q, in.ID); err != nil {
				return nil, err
			}
		}
		return map[string]string{"discardedDraft": in.ID}, nil
	})
}

func (s *Service) unpostVoucherTx(ctx context.Context, tx *sql.Tx, saved *ledger.Document) error {
	var closed bool
	if err := tx.QueryRowContext(ctx, "SELECT closed FROM periods WHERE period=?", saved.Date[:7]).Scan(&closed); err == sql.ErrNoRows || closed {
		return accounting.Fail("PERIOD_CLOSED", "Kỳ kế toán của chứng từ đã khóa.")
	} else if err != nil {
		return err
	}
	var reversed int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE reversal_of=? AND status='posted'", saved.ID).Scan(&reversed); err != nil {
		return err
	}
	if reversed > 0 {
		return accounting.Fail("DOCUMENT_POSTED", "Chứng từ đã có chứng từ đảo; không sửa trực tiếp.")
	}
	var settlementID string
	err := tx.QueryRowContext(ctx, "SELECT id FROM documents WHERE source_type IN ('receipt','payment') AND source_id=?", "settlement:"+saved.ID).Scan(&settlementID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		var rev int
		if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE reversal_of=? AND status='posted'", settlementID).Scan(&rev); e != nil {
			return e
		}
		if rev > 0 {
			return accounting.Fail("DOCUMENT_POSTED", "Phiếu thanh toán đi kèm đã bị đảo; không sửa trực tiếp.")
		}
		if _, e := tx.ExecContext(ctx, "UPDATE documents SET status='draft' WHERE id=?", settlementID); e != nil {
			return e
		}
		for _, q := range []string{
			"DELETE FROM bank_movements WHERE document_id=?",
			"DELETE FROM operations WHERE document_id=?",
			"DELETE FROM voucher_inputs WHERE document_id=?",
			"DELETE FROM document_lines WHERE document_id=?",
			"DELETE FROM documents WHERE id=?",
		} {
			if _, e := tx.ExecContext(ctx, q, settlementID); e != nil {
				return e
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE documents SET status='draft' WHERE id=?", saved.ID); err != nil {
		return err
	}
	type stockKey struct{ item, warehouse string }
	keys := []stockKey{}
	rows, err := tx.QueryContext(ctx, "SELECT DISTINCT m.item_id,w.warehouse_id FROM stock_movements m JOIN stock_movement_warehouses w ON w.movement_id=m.id WHERE m.document_id=?", saved.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var k stockKey
		if err = rows.Scan(&k.item, &k.warehouse); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, k)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if _, err = tx.ExecContext(ctx, "DELETE FROM stock_movement_warehouses WHERE movement_id IN (SELECT id FROM stock_movements WHERE document_id=?)", saved.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM stock_movements WHERE document_id=?", saved.ID); err != nil {
		return err
	}
	for _, k := range keys {
		if err = rebuildStockLotsTx(ctx, tx, k.item, k.warehouse); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM bank_movements WHERE document_id=?", saved.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM operations WHERE document_id=?", saved.ID); err != nil {
		return err
	}
	return nil
}

func rebuildStockLotsTx(ctx context.Context, tx *sql.Tx, item, warehouse string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM stock_lots WHERE item_id=? AND id IN (SELECT lot_id FROM stock_lot_warehouses WHERE warehouse_id=?)", item, warehouse); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT m.id,m.document_id,m.date,m.quantity,m.value,m.scale,d.source_type,COALESCE(p.data,'')
		FROM stock_movements m
		JOIN stock_movement_warehouses w ON w.movement_id=m.id
		JOIN documents d ON d.id=m.document_id
		LEFT JOIN policies p ON p.id=d.policy_id
		WHERE m.item_id=? AND w.warehouse_id=?
		ORDER BY m.date,CASE WHEN d.source_type IN ('opening','opening_adjustment','opening_adjustment_reversal') THEN 0 ELSE 1 END,COALESCE(d.posted_order,9223372036854775807),m.id`, item, warehouse)
	if err != nil {
		return err
	}
	type move struct {
		id              int64
		doc, date       string
		qty, val        int64
		scale           int
		source, rawPol  string
	}
	moves := []move{}
	for rows.Next() {
		var m move
		if err = rows.Scan(&m.id, &m.doc, &m.date, &m.qty, &m.val, &m.scale, &m.source, &m.rawPol); err != nil {
			rows.Close()
			return err
		}
		moves = append(moves, m)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, m := range moves {
		if m.source == "opening_adjustment_reversal" {
			if _, err = tx.ExecContext(ctx, `UPDATE stock_lots SET quantity=0,value=0 WHERE item_id=? AND id IN (SELECT w.lot_id FROM stock_lot_warehouses w JOIN stock_lots l ON l.id=w.lot_id JOIN documents d ON d.id=l.document_id WHERE w.warehouse_id=? AND d.source_type IN ('opening','opening_adjustment'))`, item, warehouse); err != nil {
				return err
			}
			continue
		}
		if m.qty > 0 {
			lotID := fmt.Sprintf("%s-%06d", m.doc, m.id)
			if _, err = tx.ExecContext(ctx, "INSERT INTO stock_lots VALUES(?,?,?,?,?,?,?)", lotID, item, m.doc, m.date, m.qty, m.val, m.scale); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO stock_lot_warehouses VALUES(?,?)", lotID, warehouse); err != nil {
				return err
			}
			continue
		}
		if m.qty < 0 {
			outQty, outVal := -m.qty, -m.val
			pol := Policy{InventoryMethod: "fifo", Rounding: "half_up"}
			if m.rawPol != "" {
				_ = json.Unmarshal([]byte(m.rawPol), &pol)
			}
			lotRows, e := tx.QueryContext(ctx, "SELECT l.id,l.quantity,l.value FROM stock_lots l LEFT JOIN documents d ON d.id=l.document_id JOIN stock_lot_warehouses w ON w.lot_id=l.id WHERE l.item_id=? AND w.warehouse_id=? AND l.quantity>0 ORDER BY l.date,CASE WHEN d.source_type IN ('opening','opening_adjustment') THEN 0 ELSE 1 END,COALESCE(d.posted_order,9223372036854775807),l.id", item, warehouse)
			if e != nil {
				return e
			}
			var lots []lot
			var totalQty, totalValue int64
			for lotRows.Next() {
				var l lot
				if e = lotRows.Scan(&l.id, &l.quantity, &l.value); e != nil {
					lotRows.Close()
					return e
				}
				totalQty += l.quantity
				totalValue += l.value
				lots = append(lots, l)
			}
			if e = lotRows.Err(); e != nil {
				lotRows.Close()
				return e
			}
			lotRows.Close()
			if totalQty < outQty || totalValue < outVal {
				return accounting.Fail("INSUFFICIENT_STOCK", "Không đủ tồn kho cho chứng từ xuất kho tiếp theo.")
			}
			if pol.InventoryMethod == "moving_average" {
				if _, err = tx.ExecContext(ctx, "DELETE FROM stock_lots WHERE item_id=? AND id IN (SELECT lot_id FROM stock_lot_warehouses WHERE warehouse_id=?)", item, warehouse); err != nil {
					return err
				}
				if totalQty > outQty {
					lotID := sqlite.ID()
					if _, err = tx.ExecContext(ctx, "INSERT INTO stock_lots VALUES(?,?,?,?,?,?,?)", lotID, item, m.doc, m.date, totalQty-outQty, totalValue-outVal, m.scale); err != nil {
						return err
					}
					if _, err = tx.ExecContext(ctx, "INSERT INTO stock_lot_warehouses VALUES(?,?)", lotID, warehouse); err != nil {
						return err
					}
				}
			} else {
				remQty, remVal := outQty, outVal
				for _, l := range lots {
					if remQty == 0 {
						break
					}
					take := min(l.quantity, remQty)
					piece := l.value
					if take == remQty {
						piece = remVal
					} else if take != l.quantity {
						piece, err = accounting.Round(new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(l.value), big.NewInt(take)), big.NewInt(l.quantity)), pol.Rounding)
						if err != nil {
							return err
						}
					}
					if piece > l.value || piece > remVal {
						return accounting.Fail("INSUFFICIENT_STOCK", "Giá trị tồn kho không đủ cho các lần xuất tiếp theo.")
					}
					if _, err = tx.ExecContext(ctx, "UPDATE stock_lots SET quantity=quantity-?,value=value-? WHERE id=?", take, piece, l.id); err != nil {
						return err
					}
					remQty -= take
					remVal -= piece
				}
			}
		}
	}
	return nil
}

func verifyEditedVoucherLinksTx(ctx context.Context, tx *sql.Tx, docID string) error {
	rows, err := tx.QueryContext(ctx, `SELECT o.kind,d.partner_id,d.date FROM operations o JOIN documents d ON d.id=o.document_id,json_each(o.payload,'$.allocations') j WHERE d.status='posted' AND o.kind IN ('receipt','payment') AND json_extract(j.value,'$.documentId')=? AND NOT EXISTS(SELECT 1 FROM documents r WHERE r.reversal_of=d.id AND r.status='posted')`, docID)
	if err != nil {
		return err
	}
	type link struct{ kind, partner, date string }
	links := []link{}
	for rows.Next() {
		var l link
		if err = rows.Scan(&l.kind, &l.partner, &l.date); err != nil {
			rows.Close()
			return err
		}
		links = append(links, l)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(links) == 0 {
		return nil
	}
	invoice, err := invoiceAt(ctx, tx, docID, "9999-12-31")
	if err != nil {
		return err
	}
	for _, l := range links {
		expectedKind := "sale"
		if l.kind == "payment" {
			expectedKind = "purchase"
		}
		if invoice.Kind != expectedKind || invoice.PartnerID != l.partner || invoice.Date > l.date {
			return accounting.Fail("OVERPAYMENT", "Chứng từ đã có phiếu thu/chi phân bổ; đối tượng, loại phiếu và ngày không được mâu thuẫn với phiếu thu/chi.")
		}
	}
	return nil
}
