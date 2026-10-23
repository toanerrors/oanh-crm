package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
)

type OpeningLine struct {
	ledger.Line
	BankAccountID string `json:"bankAccountId,omitempty"`
}
type OpeningStock struct {
	ItemID      string `json:"itemId"`
	WarehouseID string `json:"warehouseId"`
	Quantity    string `json:"quantity"`
	Value       string `json:"value"`
}
type OpeningInput struct {
	ID      string         `json:"id"`
	Version int            `json:"version"`
	Cutoff  string         `json:"cutoff"`
	Source  string         `json:"source"`
	Lines   []OpeningLine  `json:"lines"`
	Stocks  []OpeningStock `json:"stocks"`
	Assets  []Asset        `json:"assets"`
}
type OpeningState struct {
	ledger.Context
	Input          *OpeningInput           `json:"input"`
	Document       *ledger.Document        `json:"document"`
	PostedCount    int                     `json:"postedCount"`
	RootID         string                  `json:"rootId"`
	BalanceVersion int                     `json:"balanceVersion"`
	Adjustment     *OpeningAdjustmentInput `json:"adjustment,omitempty"`
}

func openingAt(ctx context.Context, tx *sql.Tx) (OpeningState, error) {
	var state OpeningState
	var raw, id string
	var err error
	state.Context, err = ledger.ReadContext(ctx, tx)
	if err != nil {
		return state, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE status='posted' AND source_type NOT IN ('opening','opening_adjustment','opening_adjustment_reversal')").Scan(&state.PostedCount); err != nil {
		return state, err
	}
	err = tx.QueryRowContext(ctx, "SELECT opening_document_id,document_id,payload FROM opening_current").Scan(&state.RootID, &id, &raw)
	if err == sql.ErrNoRows {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	var input OpeningInput
	if err = json.Unmarshal([]byte(raw), &input); err != nil {
		return state, err
	}
	doc, err := ledger.ReadDocument(ctx, tx, id)
	input.Version = doc.Version
	state.Input, state.Document = &input, &doc
	if err != nil {
		return state, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(confirmed_version),0) FROM opening_adjustments").Scan(&state.BalanceVersion); err != nil {
		return state, err
	}
	var draftID string
	err = tx.QueryRowContext(ctx, "SELECT document_id FROM opening_adjustments WHERE confirmed_version IS NULL").Scan(&draftID)
	if err == nil {
		a, e := readOpeningAdjustment(ctx, tx, draftID)
		state.Adjustment = &a.OpeningAdjustmentInput
		return state, e
	}
	if err != sql.ErrNoRows {
		return state, err
	}
	return state, nil
}
func (s *Service) Opening(ctx context.Context) (OpeningState, error) {
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return OpeningState{}, err
	}
	defer tx.Rollback()
	state, err := openingAt(ctx, tx)
	if err != nil {
		return state, err
	}
	return state, tx.Commit()
}
func (s *Service) openingDocument(ctx context.Context, tx *sql.Tx, in OpeningInput) (ledger.Document, Policy, error) {
	return s.openingDocumentPolicy(ctx, tx, in, nil)
}
func (s *Service) openingDocumentPolicy(ctx context.Context, tx *sql.Tx, in OpeningInput, pinned *Policy) (ledger.Document, Policy, error) {
	doc := ledger.Document{ID: in.ID, Version: in.Version, Type: "journal", Date: in.Cutoff, Description: "Số dư đầu kỳ: " + in.Source, SourceType: "opening", SourceID: "initial", PolicyID: "opening"}
	var policy Policy
	if !accounting.ID(in.ID) || in.Version < 0 || !accounting.Date(in.Cutoff) || in.Cutoff[8:] != "01" || strings.TrimSpace(in.Source) == "" || len(in.Source) > 900 || len(in.Lines)+len(in.Stocks)+len(in.Assets) > 150 {
		return doc, policy, accounting.Fail("VALIDATION", "Chọn ngày đầu tháng chuyển đổi, nguồn đối chiếu và tối đa 150 dòng chi tiết.")
	}
	if len(in.Stocks)+len(in.Assets) > 0 {
		var err error
		if pinned != nil {
			policy = *pinned
		} else {
			policy, err = PolicyAt(ctx, tx, in.Cutoff)
		}
		if err != nil {
			return doc, policy, err
		}
		doc.PolicyID = policy.ID
	}
	for _, line := range in.Lines {
		var group, code string
		if err := tx.QueryRowContext(ctx, "SELECT account_group,code FROM accounts WHERE id=? AND active=1", line.AccountID).Scan(&group, &code); err != nil {
			return doc, policy, accounting.Fail("INVALID_ACCOUNT", "Chọn tài khoản còn hiệu lực.")
		}
		if group == "inventory" || group == "asset" {
			return doc, policy, accounting.Fail("SUBLEDGER_REQUIRED", "Số dư kho/tài sản nhập tại tab Tồn kho hoặc Tài sản để đồng thời tạo chi tiết phân hệ.")
		}
		if group == "revenue" || group == "expense" || code == "911" {
			return doc, policy, accounting.Fail("INVALID_OPENING_ACCOUNT", "Chuyển số dư sau khi kết chuyển doanh thu, chi phí và xác định kết quả; không nhập số dư tài khoản kết quả vào đây.")
		}
		var bankCount int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM bank_accounts WHERE account_id=?", line.AccountID).Scan(&bankCount); err != nil {
			return doc, policy, err
		}
		if line.BankAccountID != "" {
			bank, err := bankAt(ctx, tx, line.BankAccountID)
			if err != nil {
				return doc, policy, err
			}
			if bank.AccountID != line.AccountID {
				return doc, policy, accounting.Fail("INVALID_BANK", "Tài khoản ngân hàng không khớp tài khoản kế toán.")
			}
		} else if bankCount > 0 || code == "112" || code == "1121" || code == "1122" || code == "1123" {
			return doc, policy, accounting.Fail("INVALID_BANK", "Số dư tiền gửi phải chi tiết theo tài khoản ngân hàng.")
		}
		doc.Lines = append(doc.Lines, line.Line)
	}
	for _, stock := range in.Stocks {
		var active, tracked bool
		if err := tx.QueryRowContext(ctx, "SELECT active,tracked FROM items WHERE id=?", stock.ItemID).Scan(&active, &tracked); err != nil || !active || !tracked {
			return doc, policy, accounting.Fail("INVALID_ITEM", "Chọn hàng hóa theo dõi tồn kho còn hiệu lực.")
		}
		if err := tx.QueryRowContext(ctx, "SELECT active FROM warehouses WHERE id=?", stock.WarehouseID).Scan(&active); err != nil || !active {
			return doc, policy, accounting.Fail("INVALID_WAREHOUSE", "Chọn kho còn hiệu lực.")
		}
		q, err := accounting.Quantity(stock.Quantity, policy.QuantityScale)
		if err != nil || q <= 0 {
			return doc, policy, accounting.Fail("VALIDATION", "Số lượng tồn đầu kỳ phải dương và đúng scale của chính sách.")
		}
		value, err := accounting.Money(stock.Value)
		if err != nil {
			return doc, policy, err
		}
		if value > 0 {
			addLine(&doc, "156", "", "Tồn kho đầu kỳ", value, 0)
		}
	}
	seen := map[string]bool{}
	codes := map[string]bool{}
	for _, a := range in.Assets {
		cost, err := accounting.Money(a.Cost)
		if err != nil {
			return doc, policy, err
		}
		residual, err := accounting.Money(a.Residual)
		if err != nil {
			return doc, policy, err
		}
		allocated, err := accounting.Money(a.Allocated)
		if err != nil {
			return doc, policy, err
		}
		if !accounting.ID(a.ID) || !accounting.ID(a.Code) || strings.TrimSpace(a.Name) == "" || len(a.Name) > 200 || seen[a.ID] || codes[strings.ToLower(a.Code)] || a.Kind != "fixed" && a.Kind != "prepaid" || cost == 0 || residual >= cost || allocated > cost-residual || a.Months < 1 || a.Months > 1200 || !accounting.Period(a.StartPeriod) || a.StartPeriod > in.Cutoff[:7] {
			return doc, policy, accounting.Fail("VALIDATION", "Kiểm tra mã, loại, nguyên giá, giá trị thu hồi, lũy kế và thời gian khấu hao/phân bổ.")
		}
		seen[a.ID] = true
		codes[strings.ToLower(a.Code)] = true
		elapsed := openingElapsed(a.StartPeriod, in.Cutoff)
		if elapsed >= a.Months && allocated != cost-residual || elapsed == 0 && allocated != 0 || elapsed < a.Months && (cost-residual)/int64(a.Months) == 0 {
			return doc, policy, accounting.Fail("INVALID_ALLOCATION", "Lũy kế không phù hợp thời gian phân bổ; tài sản hết thời gian phải được phân bổ hết.")
		}
		if elapsed < a.Months && cost-residual-allocated <= ((cost-residual)/int64(a.Months))*int64(a.Months-elapsed-1) {
			return doc, policy, accounting.Fail("INVALID_ALLOCATION", "Giá trị còn lại không đủ phân bổ các kỳ còn lại theo đường thẳng và phần dư tháng cuối.")
		}
		if a.Kind == "fixed" {
			addLine(&doc, "211", "", "Nguyên giá đầu kỳ: "+a.Name, cost, 0)
			if allocated > 0 {
				addLine(&doc, "214", "", "Hao mòn đầu kỳ: "+a.Name, 0, allocated)
			}
		} else if cost > allocated {
			addLine(&doc, "242", "", "Chi phí trả trước đầu kỳ: "+a.Name, cost-allocated, 0)
		}
	}
	if len(doc.Lines) < 2 || len(doc.Lines) > 200 {
		return doc, policy, accounting.Fail("VALIDATION", "Nhập từ 2 đến 200 dòng hạch toán có số tiền, gồm cả chi tiết kho/tài sản.")
	}
	return doc, policy, nil
}
func openingElapsed(start, cutoff string) int {
	a, _ := time.Parse("2006-01", start)
	b, _ := time.Parse("2006-01-02", cutoff)
	return (b.Year()-a.Year())*12 + int(b.Month()-a.Month())
}
func (s *Service) SaveOpening(ctx context.Context, cmd ledger.Command, in OpeningInput) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "admin.opening.save", in, func(tx *sql.Tx) (any, error) {
		return s.saveOpeningTx(ctx, tx, in)
	})
}
func (s *Service) saveOpeningTx(ctx context.Context, tx *sql.Tx, in OpeningInput) (any, error) {
	if in.Lines == nil {
		in.Lines = []OpeningLine{}
	}
	if in.Stocks == nil {
		in.Stocks = []OpeningStock{}
	}
	if in.Assets == nil {
		in.Assets = []Asset{}
	}
	state, err := openingAt(ctx, tx)
	if err != nil {
		return nil, err
	}
	if state.Document != nil && (state.Document.Status == "posted" || state.Document.ID != in.ID) {
		return nil, accounting.Fail("OPENING_LOCKED", "Số dư đầu kỳ đã xác nhận hoặc đã có bản nhập khác. Mở lại bản hiện tại.")
	}
	if state.PostedCount > 0 {
		return nil, accounting.Fail("OPENING_HISTORY_EXISTS", "Chỉ khởi tạo số dư trước lần ghi sổ đầu tiên để tránh cộng trùng lịch sử.")
	}
	if state.Document == nil {
		var exists int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE id=?", in.ID).Scan(&exists); err != nil {
			return nil, err
		}
		if exists > 0 {
			return nil, accounting.Fail("DUPLICATE_SOURCE", "Mã bản nhập đã thuộc chứng từ khác.")
		}
	}
	doc, _, err := s.openingDocument(ctx, tx, in)
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
	in.Version = doc.Version
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO opening_balances VALUES(1,?,?,?,?) ON CONFLICT(id) DO UPDATE SET cutoff=excluded.cutoff,source=excluded.source,payload=excluded.payload", doc.ID, in.Cutoff, in.Source, string(raw)); err != nil {
		return nil, err
	}
	state, err = openingAt(ctx, tx)
	state.Revision++
	return state, err
}
func (s *Service) PostOpening(ctx context.Context, cmd ledger.Command, in ledger.Post) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "admin.opening.post", in, func(tx *sql.Tx) (any, error) {
		state, err := openingAt(ctx, tx)
		if err != nil {
			return nil, err
		}
		if state.Document == nil || state.Document.ID != in.ID {
			return nil, accounting.Fail("NOT_FOUND", "Chưa lưu số dư đầu kỳ.")
		}
		doc := *state.Document
		if doc.Status == "posted" {
			return nil, accounting.Fail("OPENING_LOCKED", "Số dư đầu kỳ đã xác nhận và không được sửa trực tiếp.")
		}
		if doc.Version != in.Version {
			return nil, accounting.Fail("VERSION_CONFLICT", "Bản nhập số dư đã thay đổi. Mở lại để đối chiếu.")
		}
		if state.PostedCount > 0 {
			return nil, accounting.Fail("OPENING_HISTORY_EXISTS", "Đã có chứng từ ghi sổ; không thể khởi tạo số dư cộng thêm lịch sử.")
		}
		_, policy, err := s.openingDocument(ctx, tx, *state.Input)
		if err != nil {
			return nil, err
		}
		if len(state.Input.Stocks)+len(state.Input.Assets) > 0 {
			doc.PolicyID = policy.ID
			if _, err = tx.ExecContext(ctx, "UPDATE documents SET policy_id=? WHERE id=?", doc.PolicyID, doc.ID); err != nil {
				return nil, err
			}
		}
		for i, line := range state.Input.Lines {
			if line.BankAccountID != "" {
				d, _ := accounting.Money(line.Debit)
				c, _ := accounting.Money(line.Credit)
				if _, err = tx.ExecContext(ctx, "INSERT INTO bank_movements VALUES(?,?,?,?,?)", doc.ID, i, line.BankAccountID, d, c); err != nil {
					return nil, err
				}
			}
		}
		for i, stock := range state.Input.Stocks {
			quantity, _ := accounting.Quantity(stock.Quantity, policy.QuantityScale)
			value, _ := accounting.Money(stock.Value)
			if _, err = s.stock(ctx, tx, &doc, stock.ItemID, quantity, value, policy, i, true, stock.WarehouseID); err != nil {
				return nil, err
			}
		}
		for _, a := range state.Input.Assets {
			cost, _ := accounting.Money(a.Cost)
			residual, _ := accounting.Money(a.Residual)
			allocated, _ := accounting.Money(a.Allocated)
			if _, err = tx.ExecContext(ctx, "INSERT INTO assets VALUES(?,?,?,?,?,?,?,?,?,?)", a.ID, a.Code, a.Name, a.Kind, cost, residual, a.Months, a.StartPeriod, doc.ID, allocated); err != nil {
				return nil, err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO opening_assets VALUES(?,?,?)", a.ID, openingElapsed(a.StartPeriod, state.Input.Cutoff), allocated); err != nil {
				return nil, err
			}
		}
		if _, err = s.Ledger.PostTx(ctx, tx, in); err != nil {
			return nil, err
		}
		state, err = openingAt(ctx, tx)
		state.Revision++
		return state, err
	})
}

func (s *Service) DiscardOpening(ctx context.Context, cmd ledger.Command, in ledger.Post) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "admin.opening.discard", in, func(tx *sql.Tx) (any, error) {
		state, err := openingAt(ctx, tx)
		if err != nil {
			return nil, err
		}
		if state.Document == nil || state.Document.ID != in.ID {
			return nil, accounting.Fail("NOT_FOUND", "Không tìm thấy nháp số dư.")
		}
		if state.Document.Status == "posted" {
			return nil, accounting.Fail("OPENING_LOCKED", "Số dư đã xác nhận không được xóa.")
		}
		if state.Document.Version != in.Version {
			return nil, accounting.Fail("VERSION_CONFLICT", "Bản nhập số dư đã thay đổi.")
		}
		for _, query := range []string{"DELETE FROM opening_balances WHERE document_id=?", "DELETE FROM document_lines WHERE document_id=?", "DELETE FROM documents WHERE id=?"} {
			if _, err = tx.ExecContext(ctx, query, in.ID); err != nil {
				return nil, err
			}
		}
		state, err = openingAt(ctx, tx)
		state.Revision++
		return state, err
	})
}
