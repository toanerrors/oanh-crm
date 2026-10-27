package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

type OpeningAdjustmentInput struct {
	Input       OpeningInput `json:"input"`
	BaseVersion int          `json:"baseVersion"`
	Reason      string       `json:"reason"`
}
type OpeningAdjustment struct {
	ledger.Context
	OpeningAdjustmentInput
	Before           OpeningInput     `json:"before"`
	Document         ledger.Document  `json:"document"`
	ConfirmedVersion *int             `json:"confirmedVersion"`
	CreatedAt        string           `json:"createdAt"`
	ConfirmedAt      string           `json:"confirmedAt"`
	Actor            string           `json:"actor"`
	Blockers         []OpeningBlocker `json:"blockers"`
}
type OpeningBlocker struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	DocumentID string `json:"documentId,omitempty"`
}
type OpeningAdjustmentPost struct {
	ID       string `json:"id"`
	Version  int    `json:"version"`
	Revision int64  `json:"revision"`
}

func readOpeningAdjustment(ctx context.Context, tx *sql.Tx, id string) (OpeningAdjustment, error) {
	var a OpeningAdjustment
	var before, after string
	a.Blockers = []OpeningBlocker{}
	err := tx.QueryRowContext(ctx, `SELECT a.base_version,a.reason,a.before_payload,a.after_payload,a.confirmed_version,a.created_at,COALESCE(a.confirmed_at,''),u.username FROM opening_adjustments a JOIN users u ON u.id=COALESCE(a.confirmed_by,a.created_by) WHERE a.document_id=?`, id).Scan(&a.BaseVersion, &a.Reason, &before, &after, &a.ConfirmedVersion, &a.CreatedAt, &a.ConfirmedAt, &a.Actor)
	if err == sql.ErrNoRows {
		return a, accounting.Fail("NOT_FOUND", "Không tìm thấy bản điều chỉnh số dư.")
	}
	if err != nil {
		return a, err
	}
	if err = json.Unmarshal([]byte(before), &a.Before); err != nil {
		return a, err
	}
	if err = json.Unmarshal([]byte(after), &a.Input); err != nil {
		return a, err
	}
	a.Document, err = ledger.ReadDocument(ctx, tx, id)
	a.Input.Version = a.Document.Version
	return a, err
}
func adjustmentPolicy(ctx context.Context, tx *sql.Tx, doc *ledger.Document, in OpeningInput) (*Policy, error) {
	if len(in.Stocks)+len(in.Assets) == 0 && doc.PolicyID == "opening" {
		return nil, nil
	}
	if doc.PolicyID == "opening" {
		p, e := PolicyAt(ctx, tx, in.Cutoff)
		return &p, e
	}
	var raw string
	var p Policy
	if e := tx.QueryRowContext(ctx, "SELECT data FROM policies WHERE id=?", doc.PolicyID).Scan(&raw); e != nil {
		return nil, e
	}
	e := json.Unmarshal([]byte(raw), &p)
	return &p, e
}
func (s *Service) adjustmentDocument(ctx context.Context, tx *sql.Tx, state OpeningState, in OpeningInput) (ledger.Document, Policy, error) {
	p, err := adjustmentPolicy(ctx, tx, state.Document, in)
	if err != nil {
		return ledger.Document{}, Policy{}, err
	}
	doc, policy, err := s.openingDocumentPolicy(ctx, tx, in, p)
	doc.SourceType, doc.SourceID = "opening_adjustment", in.ID
	doc.Description = "Điều chỉnh số dư đầu kỳ: " + in.Source
	if p != nil {
		policy = *p
		doc.PolicyID = p.ID
	}
	return doc, policy, err
}
func (s *Service) SaveOpeningAdjustment(ctx context.Context, cmd ledger.Command, in OpeningAdjustmentInput) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "admin.opening.adjustment.save", in, func(tx *sql.Tx) (any, error) {
		return s.saveOpeningAdjustmentTx(ctx, tx, cmd.Actor, in)
	})
}
func (s *Service) saveOpeningAdjustmentTx(ctx context.Context, tx *sql.Tx, actor string, in OpeningAdjustmentInput) (any, error) {
	state, err := openingAt(ctx, tx)
	if err != nil {
		return nil, err
	}
	if state.Document == nil || state.Document.Status != "posted" {
		return nil, accounting.Fail("OPENING_REQUIRED", "Xác nhận số dư ban đầu trước khi lập điều chỉnh.")
	}
	if in.BaseVersion != state.BalanceVersion {
		return nil, accounting.Fail("VERSION_CONFLICT", "Đã có phiên bản số dư mới. Mở lại để đối chiếu.")
	}
	if in.Input.Cutoff != state.Input.Cutoff {
		return nil, accounting.Fail("VALIDATION", "Điều chỉnh giữ nguyên mốc chuyển đổi ban đầu.")
	}
	if strings.TrimSpace(in.Reason) == "" || utf8.RuneCountInString(in.Reason) > 1000 {
		return nil, accounting.Fail("VALIDATION", "Nhập lý do điều chỉnh (tối đa 1000 ký tự).")
	}
	if state.Adjustment != nil && state.Adjustment.Input.ID != in.Input.ID {
		return nil, accounting.Fail("VERSION_CONFLICT", "Đang có bản nháp điều chỉnh khác. Tiếp tục bản nháp đã lưu.")
	}
	if state.Adjustment == nil {
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE id=?", in.Input.ID).Scan(&count); err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, accounting.Fail("DUPLICATE_SOURCE", "Mã bản điều chỉnh thuộc chứng từ khác.")
		}
	}
	if in.Input.Lines == nil {
		in.Input.Lines = []OpeningLine{}
	}
	if in.Input.Stocks == nil {
		in.Input.Stocks = []OpeningStock{}
	}
	if in.Input.Assets == nil {
		in.Input.Assets = []Asset{}
	}
	doc, _, err := s.adjustmentDocument(ctx, tx, state, in.Input)
	if err != nil {
		return nil, err
	}
	doc, err = s.Ledger.SaveTx(ctx, tx, doc)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE documents SET policy_id=? WHERE id=?", doc.PolicyID, doc.ID); err != nil {
		return nil, err
	}
	in.Input.Version = doc.Version
	before, _ := json.Marshal(state.Input)
	after, _ := json.Marshal(in.Input)
	_, err = tx.ExecContext(ctx, `INSERT INTO opening_adjustments(document_id,opening_document_id,before_document_id,base_version,reason,before_payload,after_payload,created_by,created_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(document_id) DO UPDATE SET reason=excluded.reason,after_payload=excluded.after_payload`, doc.ID, state.RootID, state.Document.ID, in.BaseVersion, in.Reason, string(before), string(after), actor, sqlite.Now())
	if err != nil {
		return nil, err
	}
	state, err = openingAt(ctx, tx)
	state.Revision++
	return state, err
}
func (s *Service) OpeningAdjustment(ctx context.Context, id string) (OpeningAdjustment, error) {
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return OpeningAdjustment{}, err
	}
	defer tx.Rollback()
	a, err := readOpeningAdjustment(ctx, tx, id)
	if accounting.Code(err) == "NOT_FOUND" {
		var raw string
		if e := tx.QueryRowContext(ctx, "SELECT payload FROM opening_balances WHERE document_id=?", id).Scan(&raw); e == nil {
			if e = json.Unmarshal([]byte(raw), &a.Input); e != nil {
				return a, e
			}
			a.Before = a.Input
			a.Document, e = ledger.ReadDocument(ctx, tx, id)
			if e != nil {
				return a, e
			}
			if a.Document.Status != "posted" {
				return a, accounting.Fail("NOT_FOUND", "Chưa có số dư ban đầu đã xác nhận.")
			}
			zero := 0
			a.ConfirmedVersion = &zero
			a.Reason = "Số dư ban đầu: " + a.Input.Source
			err = nil
		}
	}
	if err != nil {
		return a, err
	}
	a.Context, err = ledger.ReadContext(ctx, tx)
	if err != nil {
		return a, err
	}
	if a.ConfirmedVersion == nil {
		state, e := openingAt(ctx, tx)
		if e != nil {
			return a, e
		}
		_, p, e := s.adjustmentDocument(ctx, tx, state, a.Input)
		if e != nil {
			a.Blockers = append(a.Blockers, OpeningBlocker{"INVALID_INPUT", e.Error(), ""})
		} else {
			a.Blockers, e = s.openingAdjustmentBlockers(ctx, tx, state, a, p)
			if e != nil {
				return a, e
			}
		}
	}
	return a, tx.Commit()
}
func (s *Service) OpeningHistory(ctx context.Context, page, size int) (any, error) {
	if page < 1 || page > 1000000 || size < 1 || size > 100 {
		return nil, accounting.Fail("VALIDATION", "Phân trang lịch sử không hợp lệ.")
	}
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	c, err := ledger.ReadContext(ctx, tx)
	if err != nil {
		return nil, err
	}
	type row struct {
		ID        string `json:"id"`
		Version   int    `json:"version"`
		Reference string `json:"reference"`
		Reason    string `json:"reason"`
		At        string `json:"at"`
		Actor     string `json:"actor"`
	}
	items := []row{}
	var total int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM opening_adjustments WHERE confirmed_version IS NOT NULL").Scan(&total); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.document_id,a.confirmed_version,d.reference,a.reason,a.confirmed_at,u.username FROM opening_adjustments a JOIN documents d ON d.id=a.document_id JOIN users u ON u.id=a.confirmed_by WHERE a.confirmed_version IS NOT NULL ORDER BY a.confirmed_version DESC LIMIT ? OFFSET ?`, size, (page-1)*size)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.ID, &r.Version, &r.Reference, &r.Reason, &r.At, &r.Actor); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	state, err := openingAt(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return struct {
		ledger.Context
		Items     []row  `json:"items"`
		Total     int    `json:"total"`
		InitialID string `json:"initialId"`
	}{c, items, total, state.RootID}, nil
}
func (s *Service) DiscardOpeningAdjustment(ctx context.Context, cmd ledger.Command, in ledger.Post) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "admin.opening.adjustment.discard", in, func(tx *sql.Tx) (any, error) {
		return s.discardOpeningAdjustmentTx(ctx, tx, in)
	})
}
func (s *Service) discardOpeningAdjustmentTx(ctx context.Context, tx *sql.Tx, in ledger.Post) (any, error) {
	a, err := readOpeningAdjustment(ctx, tx, in.ID)
	if err != nil {
		return nil, err
	}
	if a.Document.Status != "draft" || a.ConfirmedVersion != nil {
		return nil, accounting.Fail("DOCUMENT_POSTED", "Bản điều chỉnh đã xác nhận không được xóa.")
	}
	if a.Document.Version != in.Version {
		return nil, accounting.Fail("VERSION_CONFLICT", "Bản nháp đã thay đổi.")
	}
	for _, q := range []string{"DELETE FROM opening_adjustments WHERE document_id=?", "DELETE FROM document_lines WHERE document_id=?", "DELETE FROM documents WHERE id=?"} {
		if _, err = tx.ExecContext(ctx, q, in.ID); err != nil {
			return nil, err
		}
	}
	state, err := openingAt(ctx, tx)
	state.Revision++
	return state, err
}
func (s *Service) ConfirmOpeningAdjustment(ctx context.Context, cmd ledger.Command, in OpeningAdjustmentPost) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "admin.opening.adjustment.confirm", in, func(tx *sql.Tx) (any, error) {
		state, err := openingAt(ctx, tx)
		if err != nil {
			return nil, err
		}
		if state.Revision != in.Revision {
			return nil, accounting.Fail("STALE_PREVIEW", "Dữ liệu đã thay đổi. Xem lại đối chiếu trước khi xác nhận.")
		}
		a, err := readOpeningAdjustment(ctx, tx, in.ID)
		if err != nil {
			return nil, err
		}
		if a.Document.Status != "draft" || a.ConfirmedVersion != nil || a.BaseVersion != state.BalanceVersion || a.Document.Version != in.Version {
			return nil, accounting.Fail("VERSION_CONFLICT", "Bản điều chỉnh đã thay đổi hoặc đã xác nhận.")
		}
		_, p, err := s.adjustmentDocument(ctx, tx, state, a.Input)
		if err != nil {
			return nil, err
		}
		blockers, err := s.openingAdjustmentBlockers(ctx, tx, state, a, p)
		if err != nil {
			return nil, err
		}
		if len(blockers) > 0 {
			return nil, accounting.Fail(blockers[0].Code, blockers[0].Message)
		}
		reverse, err := s.Ledger.PrepareOpeningReversalTx(ctx, tx, state.Document.ID, a.Reason)
		if err != nil {
			return nil, err
		}
		for _, pair := range []struct {
			doc     string
			lines   []OpeningLine
			reverse bool
		}{{reverse.ID, a.Before.Lines, true}, {a.Document.ID, a.Input.Lines, false}} {
			for i, l := range pair.lines {
				if l.BankAccountID == "" {
					continue
				}
				d, _ := accounting.Money(l.Debit)
				c, _ := accounting.Money(l.Credit)
				if pair.reverse {
					d, c = c, d
				}
				if _, err = tx.ExecContext(ctx, "INSERT INTO bank_movements VALUES(?,?,?,?,?)", pair.doc, i, l.BankAccountID, d, c); err != nil {
					return nil, err
				}
			}
		}
		if err = s.adjustOpeningSubledgers(ctx, tx, a, &reverse, p); err != nil {
			return nil, err
		}
		if _, err = s.Ledger.PostOpeningReversalTx(ctx, tx, ledger.Post{ID: reverse.ID, Version: reverse.Version}); err != nil {
			return nil, err
		}
		a.Input.Version = a.Document.Version + 1
		raw, _ := json.Marshal(a.Input)
		_, err = tx.ExecContext(ctx, `UPDATE opening_adjustments SET reversal_document_id=?,confirmed_version=base_version+1,after_payload=?,confirmed_by=?,confirmed_at=? WHERE document_id=?`, reverse.ID, string(raw), cmd.Actor, sqlite.Now(), a.Document.ID)
		if err != nil {
			return nil, err
		}
		if _, err = s.Ledger.PostTx(ctx, tx, ledger.Post{ID: a.Document.ID, Version: a.Document.Version}); err != nil {
			return nil, err
		}
		state, err = openingAt(ctx, tx)
		state.Revision++
		return state, err
	})
}

type openingStockKey struct{ item, warehouse string }
type openingStockTotal struct{ quantity, value int64 }

func openingStockTotals(stocks []OpeningStock, scale int) (map[openingStockKey]openingStockTotal, error) {
	result := map[openingStockKey]openingStockTotal{}
	for _, s := range stocks {
		k := openingStockKey{s.ItemID, s.WarehouseID}
		v := result[k]
		q, e := accounting.Quantity(s.Quantity, scale)
		if e != nil {
			return nil, e
		}
		n, e := accounting.Money(s.Value)
		if e != nil {
			return nil, e
		}
		v.quantity, e = accounting.Add(v.quantity, q)
		if e != nil {
			return nil, e
		}
		v.value, e = accounting.Add(v.value, n)
		if e != nil {
			return nil, e
		}
		result[k] = v
	}
	return result, nil
}
func changedOpeningStocks(before, after []OpeningStock, scale int) ([]openingStockKey, map[openingStockKey]openingStockTotal, map[openingStockKey]openingStockTotal, error) {
	b, e := openingStockTotals(before, scale)
	if e != nil {
		return nil, nil, nil, e
	}
	a, e := openingStockTotals(after, scale)
	if e != nil {
		return nil, nil, nil, e
	}
	keys := []openingStockKey{}
	all := map[openingStockKey]bool{}
	for k := range b {
		all[k] = true
	}
	for k := range a {
		all[k] = true
	}
	for k := range all {
		if b[k] != a[k] {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].item == keys[j].item {
			return keys[i].warehouse < keys[j].warehouse
		}
		return keys[i].item < keys[j].item
	})
	return keys, b, a, nil
}
func sameAssetValues(a, b Asset) bool {
	a.Name, b.Name = "", ""
	a.Code, b.Code = "", ""
	return reflect.DeepEqual(a, b)
}
func (s *Service) openingAdjustmentBlockers(ctx context.Context, tx *sql.Tx, state OpeningState, a OpeningAdjustment, p Policy) ([]OpeningBlocker, error) {
	out := []OpeningBlocker{}
	add := func(code, msg, id string) { out = append(out, OpeningBlocker{code, msg, id}) }
	if a.BaseVersion != state.BalanceVersion {
		add("VERSION_CONFLICT", "Đã có phiên bản số dư mới; mở lại bản hiệu lực.", "")
	}
	var closed bool
	err := tx.QueryRowContext(ctx, "SELECT closed FROM periods WHERE period=?", a.Input.Cutoff[:7]).Scan(&closed)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == sql.ErrNoRows || closed {
		add("PERIOD_CLOSED", "Kỳ chuyển đổi chưa mở hoặc đã khóa. Xử lý kỳ trong Thiết lập rồi xác nhận lại.", "")
	}
	rows, err := tx.QueryContext(ctx, "SELECT period FROM periods WHERE closed=1 AND period>? ORDER BY period", a.Input.Cutoff[:7])
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var period string
		if err = rows.Scan(&period); err != nil {
			rows.Close()
			return nil, err
		}
		add("PERIOD_CLOSED", "Kỳ "+period+" đã khóa và chịu ảnh hưởng số dư. Mở lại kỳ để điều chỉnh hồi tố.", "")
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	d, c := new(big.Int), new(big.Int)
	for _, l := range a.Document.Lines {
		v, e := accounting.Money(l.Debit)
		if e != nil {
			return nil, e
		}
		w, e := accounting.Money(l.Credit)
		if e != nil {
			return nil, e
		}
		d.Add(d, big.NewInt(v))
		c.Add(c, big.NewInt(w))
	}
	if d.Sign() == 0 || d.Cmp(c) != 0 {
		add("UNBALANCED_ENTRY", "Số dư sau điều chỉnh phải cân đối Nợ/Có và lớn hơn 0.", "")
	}
	keys, _, _, err := changedOpeningStocks(a.Before.Stocks, a.Input.Stocks, p.QuantityScale)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		var id, ref string
		err = tx.QueryRowContext(ctx, `SELECT d.id,d.reference FROM stock_movements m JOIN stock_movement_warehouses w ON w.movement_id=m.id JOIN documents d ON d.id=m.document_id WHERE m.item_id=? AND w.warehouse_id=? AND (m.quantity<0 OR (m.quantity=0 AND m.value<>0)) AND d.status='posted' AND d.source_type NOT IN ('opening','opening_adjustment','opening_adjustment_reversal') ORDER BY d.posted_order LIMIT 1`, k.item, k.warehouse).Scan(&id, &ref)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == nil {
			add("OPENING_STOCK_USED", "Hàng "+k.item+" tại kho "+k.warehouse+" đã có xuất/điều chỉnh giá trị kho "+ref+". Cần xử lý chứng từ xuất trước khi thay tồn đầu kỳ.", id)
		}
	}
	before, after := map[string]Asset{}, map[string]Asset{}
	for _, v := range a.Before.Assets {
		before[v.ID] = v
	}
	for _, v := range a.Input.Assets {
		after[v.ID] = v
	}
	for id, b := range before {
		n, exists := after[id]
		if exists && !strings.EqualFold(b.Code, n.Code) {
			var dup int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM assets WHERE code=? COLLATE NOCASE AND id<>?", n.Code, id).Scan(&dup); err != nil {
				return nil, err
			}
			if dup > 0 {
				add("DUPLICATE_CODE", "Mã tài sản "+n.Code+" đã tồn tại.", "")
			}
		}
		if exists && sameAssetValues(b, n) {
			continue
		}
		var doc, ref string
		err = tx.QueryRowContext(ctx, `SELECT d.id,d.reference FROM operations o JOIN documents d ON d.id=o.document_id WHERE o.kind='depreciation' AND substr(o.source_id,1,length(?))=? ORDER BY d.posted_order LIMIT 1`, id+":", id+":").Scan(&doc, &ref)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == nil {
			add("OPENING_ASSET_USED", "Tài sản "+b.Code+" đã khấu hao/phân bổ tại "+ref+". Chỉ đổi tên; thay số liệu cần xử lý phát sinh liên quan trước.", doc)
		}
	}
	for id, n := range after {
		if _, ok := before[id]; ok {
			continue
		}
		var exists int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM assets WHERE id=? OR code=? COLLATE NOCASE", id, n.Code).Scan(&exists); err != nil {
			return nil, err
		}
		if exists > 0 {
			add("DUPLICATE_SOURCE", "Mã tài sản "+n.Code+" đã có lịch sử; không tái sử dụng.", "")
		}
	}
	return out, nil
}
func (s *Service) adjustOpeningSubledgers(ctx context.Context, tx *sql.Tx, a OpeningAdjustment, reverse *ledger.Document, p Policy) error {
	keys, before, after, err := changedOpeningStocks(a.Before.Stocks, a.Input.Stocks, p.QuantityScale)
	if err != nil {
		return err
	}
	for i, k := range keys {
		b, n := before[k], after[k]
		var qty, value, otherQty, otherValue int64
		rows, e := tx.QueryContext(ctx, `SELECT l.quantity,l.value,l.scale,d.source_type,COALESCE(p.data,'') FROM stock_lots l JOIN stock_lot_warehouses w ON w.lot_id=l.id JOIN documents d ON d.id=l.document_id LEFT JOIN policies p ON p.id=d.policy_id WHERE l.item_id=? AND w.warehouse_id=? AND l.quantity>0`, k.item, k.warehouse)
		if e != nil {
			return e
		}
		for rows.Next() {
			var q, v int64
			var scale int
			var source, raw string
			if e = rows.Scan(&q, &v, &scale, &source, &raw); e != nil {
				break
			}
			if scale != p.QuantityScale {
				e = accounting.Fail("STOCK_POLICY_LOCKED", "Scale tồn kho đã thay đổi; đối chiếu trước khi điều chỉnh.")
				break
			}
			var original Policy
			if e = json.Unmarshal([]byte(raw), &original); e != nil {
				break
			}
			if original.InventoryMethod != p.InventoryMethod {
				e = accounting.Fail("STOCK_POLICY_LOCKED", "Phương pháp tính giá kho đã thay đổi; xử lý tồn trước khi điều chỉnh.")
				break
			}
			if ledger.IsOpening(source) {
				qty, e = accounting.Add(qty, q)
				if e == nil {
					value, e = accounting.Add(value, v)
				}
			} else {
				otherQty, e = accounting.Add(otherQty, q)
				if e == nil {
					otherValue, e = accounting.Add(otherValue, v)
				}
			}
			if e != nil {
				break
			}
		}
		rowErr := rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if rowErr != nil {
			return rowErr
		}
		if qty != b.quantity || value != b.value {
			return accounting.Fail("SUBLEDGER_MISMATCH", "Lô tồn đầu kỳ không khớp phiên bản đang dùng; cần đối chiếu dữ liệu trước.")
		}
		if _, e = accounting.Add(otherQty, n.quantity); e != nil {
			return e
		}
		if _, e = accounting.Add(otherValue, n.value); e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, `UPDATE stock_lots SET quantity=0,value=0 WHERE item_id=? AND id IN(SELECT w.lot_id FROM stock_lot_warehouses w JOIN stock_lots l ON l.id=w.lot_id JOIN documents d ON d.id=l.document_id WHERE w.warehouse_id=? AND d.source_type IN ('opening','opening_adjustment'))`, k.item, k.warehouse); err != nil {
			return err
		}
		if b.quantity > 0 {
			if err = insertStockMovement(ctx, tx, k.item, reverse, -b.quantity, -b.value, p.QuantityScale, k.warehouse); err != nil {
				return err
			}
		}
		if n.quantity > 0 {
			lotID := fmt.Sprintf("%s-%06d", a.Document.ID, i)
			if _, err = tx.ExecContext(ctx, "INSERT INTO stock_lots VALUES(?,?,?,?,?,?,?)", lotID, k.item, a.Document.ID, a.Input.Cutoff, n.quantity, n.value, p.QuantityScale); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO stock_lot_warehouses VALUES(?,?)", lotID, k.warehouse); err != nil {
				return err
			}
			if err = insertStockMovement(ctx, tx, k.item, &a.Document, n.quantity, n.value, p.QuantityScale, k.warehouse); err != nil {
				return err
			}
		}
	}
	old := map[string]Asset{}
	for _, v := range a.Before.Assets {
		old[v.ID] = v
	}
	for _, v := range a.Input.Assets {
		b, exists := old[v.ID]
		if exists && sameAssetValues(b, v) {
			if b.Name != v.Name || b.Code != v.Code {
				if _, err = tx.ExecContext(ctx, "UPDATE assets SET code=?,name=? WHERE id=?", v.Code, v.Name, v.ID); err != nil {
					return err
				}
			}
			continue
		}
		cost, _ := accounting.Money(v.Cost)
		residual, _ := accounting.Money(v.Residual)
		allocated, _ := accounting.Money(v.Allocated)
		if exists {
			_, err = tx.ExecContext(ctx, "UPDATE assets SET code=?,name=?,kind=?,cost=?,residual=?,months=?,start_period=?,allocated=? WHERE id=?", v.Code, v.Name, v.Kind, cost, residual, v.Months, v.StartPeriod, allocated, v.ID)
		} else {
			_, err = tx.ExecContext(ctx, "INSERT INTO assets VALUES(?,?,?,?,?,?,?,?,?,?)", v.ID, v.Code, v.Name, v.Kind, cost, residual, v.Months, v.StartPeriod, a.Document.ID, allocated)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
