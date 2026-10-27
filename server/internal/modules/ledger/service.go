package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
)

type Service struct{ Store *sqlite.Store }

func (s *Service) Write(ctx context.Context, cmd Command, operation string, payload any, apply func(*sql.Tx) (any, error)) (json.RawMessage, error) {
	if !accounting.ID(cmd.Key) {
		return nil, accounting.Fail("IDEMPOTENCY_REQUIRED", "Mỗi thao tác ghi cần khóa chống gửi trùng hợp lệ.")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	hash := sqlite.Hash(b)
	tx, err := s.Store.Write.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.CheckWriteScopeTx(ctx, tx, cmd, operation); err != nil {
		return nil, err
	}
	var oldHash, result string
	err = tx.QueryRowContext(ctx, "SELECT payload_hash,result FROM commands WHERE actor_id=? AND operation=? AND key=?", cmd.Actor, operation, cmd.Key).Scan(&oldHash, &result)
	if err == nil {
		if oldHash != hash {
			return nil, accounting.Fail("IDEMPOTENCY_CONFLICT", "Khóa chống trùng đã dùng cho nội dung khác.")
		}
		return json.RawMessage(result), nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	auditID, err := sqlite.BeginAudit(ctx, tx, cmd.Actor)
	if err != nil {
		return nil, err
	}
	value, err := apply(tx)
	if err != nil {
		return nil, err
	}
	b, err = json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE meta SET revision=revision+1 WHERE id=1"); err != nil {
		return nil, err
	}
	metadata, err := auditMetadata(ctx, tx, auditID, operation)
	if err != nil {
		return nil, err
	}
	if err = sqlite.FinishAudit(ctx, tx, auditID, cmd.Actor, operation, cmd.Key, metadata); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO commands VALUES(?,?,?,?,?,?)", cmd.Actor, operation, cmd.Key, hash, string(b), sqlite.Now()); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}
func (s *Service) CheckWriteScopeTx(ctx context.Context, tx *sql.Tx, cmd Command, operation string) error {
	var epoch, role string
	if err := tx.QueryRowContext(ctx, "SELECT epoch FROM meta WHERE id=1").Scan(&epoch); err != nil {
		return err
	}
	if epoch != cmd.Epoch {
		return accounting.Fail("DATASET_CHANGED", "Phiên dữ liệu đã thay đổi. Tải lại ứng dụng trước khi tiếp tục.")
	}
	err := tx.QueryRowContext(ctx, "SELECT role FROM users WHERE id=? AND active=1", cmd.Actor).Scan(&role)
	if err == sql.ErrNoRows {
		return accounting.Fail("FORBIDDEN", "Người dùng không có quyền thao tác.")
	}
	if err != nil {
		return err
	}
	if role == "viewer" || ((strings.HasPrefix(operation, "admin.") || operation == "operation.closing") && role != "owner") {
		return accounting.Fail("FORBIDDEN", "Không đủ quyền thực hiện thao tác.")
	}
	return nil
}

func reservedSource(source string) bool {
	return source != "" && strings.Contains("|opening|opening_adjustment|opening_adjustment_reversal|closing|closing_reversal|reversal|payroll|asset|depreciation|costing|corporate_tax|transfer|treasury_reversal|", "|"+source+"|")
}
func manualSource(kind, source string) (string, error) {
	if source != "" && source != "manual" && source != kind {
		return "", accounting.Fail("DERIVED_FIELD", "Loại nguồn phải khớp loại nghiệp vụ; dùng cùng mã nguồn khi nhập tay hoặc import.")
	}
	return kind, nil
}
func (s *Service) Save(ctx context.Context, cmd Command, doc Document) (json.RawMessage, error) {
	if doc.PolicyID != "" || doc.ReversalOf != "" || reservedSource(doc.SourceType) || doc.Status != "" && doc.Status != "draft" && doc.Status != "posted" {
		return nil, accounting.Fail("DERIVED_FIELD", "Không được tự đặt trạng thái ghi sổ, chính sách hoặc liên kết đảo.")
	}
	source, err := manualSource(doc.Type, doc.SourceType)
	if err != nil {
		return nil, err
	}
	doc.SourceType = source
	return s.Write(ctx, cmd, "document.save", doc, func(tx *sql.Tx) (any, error) {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM voucher_inputs WHERE document_id=?)+(SELECT count(*) FROM opening_balances WHERE document_id=?)+(SELECT count(*) FROM opening_adjustments WHERE document_id=?)", doc.ID, doc.ID, doc.ID).Scan(&count); err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, accounting.Fail("SUBLEDGER_REQUIRED", "Mở biểu mẫu nghiệp vụ để sửa nháp và tính lại dữ liệu.")
		}
		var status, oldDate, policyID, reversalOf, sourceType string
		var version int
		err := tx.QueryRowContext(ctx, "SELECT status,version,date,COALESCE(policy_id,''),COALESCE(reversal_of,''),source_type FROM documents WHERE id=?", doc.ID).Scan(&status, &version, &oldDate, &policyID, &reversalOf, &sourceType)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == nil && status == "posted" {
			if policyID != "" || reversalOf != "" || reservedSource(sourceType) {
				return nil, accounting.Fail("SUBLEDGER_REQUIRED", "Chứng từ tự động hoặc chứng từ đảo cần sửa tại nghiệp vụ nguồn.")
			}
			if version != doc.Version {
				return nil, accounting.Fail("VERSION_CONFLICT", "Chứng từ đã thay đổi. Tải lại và đối chiếu nội dung.")
			}
			var closed bool
			if e := tx.QueryRowContext(ctx, "SELECT closed FROM periods WHERE period=?", oldDate[:7]).Scan(&closed); e == sql.ErrNoRows || closed {
				return nil, accounting.Fail("PERIOD_CLOSED", "Kỳ kế toán của chứng từ đã khóa.")
			} else if e != nil {
				return nil, e
			}
			if _, e := tx.ExecContext(ctx, "UPDATE documents SET status='draft' WHERE id=?", doc.ID); e != nil {
				return nil, e
			}
			saved, e := s.SaveTx(ctx, tx, doc)
			if e != nil {
				return nil, e
			}
			return s.PostTx(ctx, tx, Post{ID: saved.ID, Version: saved.Version})
		}
		return s.SaveTx(ctx, tx, doc)
	})
}
func (s *Service) SaveTx(ctx context.Context, tx *sql.Tx, doc Document) (Document, error) {
	return s.saveTx(ctx, tx, doc, false)
}
func (s *Service) saveTx(ctx context.Context, tx *sql.Tx, doc Document, historical bool) (Document, error) {
	if !accounting.ID(doc.ID) || !accounting.Date(doc.Date) || strings.TrimSpace(doc.Description) == "" || len(doc.Description) > 1000 || len(doc.Lines) > 200 || len(doc.Lines) == 0 {
		return doc, accounting.Fail("VALIDATION", "ID, ngày, diễn giải và dòng hạch toán phải hợp lệ (tối đa 200 dòng).")
	}
	if !strings.Contains("|receipt|payment|sale|purchase|journal|", "|"+doc.Type+"|") {
		return doc, accounting.Fail("VALIDATION", "Loại chứng từ không hợp lệ.")
	}
	if doc.SourceType == "" {
		doc.SourceType = doc.Type
	}
	if doc.SourceID == "" {
		doc.SourceID = doc.ID
	}
	if !accounting.ID(doc.SourceType) || !accounting.ID(doc.SourceID) {
		return doc, accounting.Fail("VALIDATION", "Định danh nguồn chứng từ không hợp lệ.")
	}
	var status string
	var version int
	err := tx.QueryRowContext(ctx, "SELECT status,version FROM documents WHERE id=?", doc.ID).Scan(&status, &version)
	if err != nil && err != sql.ErrNoRows {
		return doc, err
	}
	if status == "posted" {
		return doc, accounting.Fail("DOCUMENT_POSTED", "Chứng từ đã ghi sổ không được sửa trực tiếp.")
	}
	if (err == sql.ErrNoRows && doc.Version != 0) || (err == nil && version != doc.Version) {
		return doc, accounting.Fail("VERSION_CONFLICT", "Nháp đã thay đổi. Tải lại và đối chiếu nội dung.")
	}
	if err == sql.ErrNoRows {
		doc.Version, err = sqlite.RecoveryVersion(ctx, tx, "documents", doc.ID)
		if err != nil {
			return doc, err
		}
	}
	var duplicate string
	err = tx.QueryRowContext(ctx, "SELECT id FROM documents WHERE source_type=? AND source_id=? AND id<>?", doc.SourceType, doc.SourceID, doc.ID).Scan(&duplicate)
	if err == nil {
		return doc, accounting.Fail("DUPLICATE_SOURCE", "Nguồn nghiệp vụ đã được ghi nhận.")
	}
	if err != sql.ErrNoRows {
		return doc, err
	}
	if doc.PartnerID != "" {
		var active bool
		if e := tx.QueryRowContext(ctx, "SELECT active FROM partners WHERE id=?", doc.PartnerID).Scan(&active); e != nil || !active && !historical {
			return doc, accounting.Fail("INVALID_PARTNER", "Đối tượng chứng từ không còn hiệu lực.")
		}
	}
	debit, credit := new(big.Int), new(big.Int)
	for i := range doc.Lines {
		line := &doc.Lines[i]
		d, e := accounting.Money(line.Debit)
		if e != nil {
			return doc, e
		}
		c, e := accounting.Money(line.Credit)
		if e != nil {
			return doc, e
		}
		if d > 0 && c > 0 || d == 0 && c == 0 {
			return doc, accounting.Fail("INVALID_LINE", "Mỗi dòng chỉ nhập số tiền dương ở một bên Nợ hoặc Có.")
		}
		debit.Add(debit, big.NewInt(d))
		credit.Add(credit, big.NewInt(c))
		var required, active bool
		accountName, partnerName := line.AccountName, line.PartnerName
		if e = tx.QueryRowContext(ctx, "SELECT name,requires_partner,active FROM accounts WHERE id=?", line.AccountID).Scan(&line.AccountName, &required, &active); e != nil || !active && !historical {
			return doc, accounting.Fail("INVALID_ACCOUNT", "Tài khoản không tồn tại hoặc ngừng sử dụng.")
		}
		if required && line.PartnerID == "" && !historical {
			return doc, accounting.Fail("INVALID_PARTNER", "Tài khoản yêu cầu đối tượng trên từng dòng.")
		}
		if line.PartnerID != "" {
			if e = tx.QueryRowContext(ctx, "SELECT name,active FROM partners WHERE id=?", line.PartnerID).Scan(&line.PartnerName, &active); e != nil || !active && !historical {
				return doc, accounting.Fail("INVALID_PARTNER", "Đối tượng dòng không còn hiệu lực.")
			}
		}
		if historical {
			line.AccountName, line.PartnerName = accountName, partnerName
		}
	}
	if !debit.IsInt64() || !credit.IsInt64() {
		return doc, accounting.Fail("AMOUNT_OVERFLOW", "Tổng chứng từ vượt giới hạn int64.")
	}
	doc.Version++
	doc.Status = "draft"
	doc.Reference = "NHAP-" + doc.ID
	if version == 0 {
		_, err = tx.ExecContext(ctx, "INSERT INTO documents(id,reference,type,date,description,partner_id,status,version,source_type,source_id,reversal_of,policy_id) VALUES(?,?,?,?,?,?,'draft',?,?,?,?,?)", doc.ID, doc.Reference, doc.Type, doc.Date, doc.Description, nullable(doc.PartnerID), doc.Version, doc.SourceType, doc.SourceID, nullable(doc.ReversalOf), nullable(doc.PolicyID))
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE documents SET type=?,date=?,description=?,partner_id=?,version=?,source_type=?,source_id=?,policy_id=? WHERE id=?", doc.Type, doc.Date, doc.Description, nullable(doc.PartnerID), doc.Version, doc.SourceType, doc.SourceID, nullable(doc.PolicyID), doc.ID)
	}
	if err != nil {
		return doc, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM document_lines WHERE document_id=?", doc.ID); err != nil {
		return doc, err
	}
	for i, line := range doc.Lines {
		d, _ := accounting.Money(line.Debit)
		c, _ := accounting.Money(line.Credit)
		if _, err = tx.ExecContext(ctx, "INSERT INTO document_lines VALUES(?,?,?,?,?,?,?,?,?)", doc.ID, i, line.AccountID, nullable(line.PartnerID), line.PartnerName, line.AccountName, line.Description, d, c); err != nil {
			return doc, err
		}
	}
	return doc, nil
}
func (s *Service) Post(ctx context.Context, cmd Command, input Post) (json.RawMessage, error) {
	return s.Write(ctx, cmd, "document.post", input, func(tx *sql.Tx) (any, error) {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM voucher_inputs WHERE document_id=?)+(SELECT count(*) FROM opening_balances WHERE document_id=?)+(SELECT count(*) FROM opening_adjustments WHERE document_id=?)", input.ID, input.ID, input.ID).Scan(&count); err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, accounting.Fail("SUBLEDGER_REQUIRED", "Mở biểu mẫu nghiệp vụ, tính lại và ghi nhận cùng phân hệ.")
		}
		return s.PostTx(ctx, tx, input)
	})
}
func (s *Service) PostTx(ctx context.Context, tx *sql.Tx, input Post) (Document, error) {
	return s.postTx(ctx, tx, input, false)
}
func (s *Service) postTx(ctx context.Context, tx *sql.Tx, input Post, historical bool) (Document, error) {
	doc, err := ReadDocument(ctx, tx, input.ID)
	if err != nil {
		return doc, err
	}
	if doc.Status != "draft" {
		return doc, accounting.Fail("DOCUMENT_POSTED", "Chứng từ đã ghi sổ.")
	}
	if doc.Version != input.Version {
		return doc, accounting.Fail("VERSION_CONFLICT", "Phiên bản nháp đã thay đổi.")
	}
	for _, line := range doc.Lines {
		var group string
		var active, required bool
		if err = tx.QueryRowContext(ctx, "SELECT account_group,active,requires_partner FROM accounts WHERE id=?", line.AccountID).Scan(&group, &active, &required); err != nil {
			return doc, err
		}
		if !active && !historical {
			return doc, accounting.Fail("INVALID_ACCOUNT", "Tài khoản đã ngừng sử dụng.")
		}
		if doc.PolicyID == "" && (group == "inventory" || group == "asset") {
			return doc, accounting.Fail("SUBLEDGER_REQUIRED", "Tài khoản kho/tài sản cần lập qua Nghiệp vụ tự động để ghi đồng thời chi tiết phân hệ.")
		}
		if !historical && (required || line.PartnerID != "") {
			if err = tx.QueryRowContext(ctx, "SELECT active FROM partners WHERE id=?", line.PartnerID).Scan(&active); err != nil || !active && !historical {
				return doc, accounting.Fail("INVALID_PARTNER", "Đối tượng dòng không còn hiệu lực.")
			}
		}
	}
	if doc.SourceType != "opening" {
		var cutoff string
		err = tx.QueryRowContext(ctx, "SELECT o.cutoff FROM opening_balances o JOIN documents d ON d.id=o.document_id WHERE d.status='posted'").Scan(&cutoff)
		if err != nil && err != sql.ErrNoRows {
			return doc, err
		}
		if cutoff != "" && doc.Date < cutoff {
			return doc, accounting.Fail("BEFORE_CONVERSION", "Ngày chứng từ trước mốc số dư đầu kỳ; không được cộng lại lịch sử trước chuyển đổi.")
		}
	}
	var closed bool
	err = tx.QueryRowContext(ctx, "SELECT closed FROM periods WHERE period=?", doc.Date[:7]).Scan(&closed)
	if err == sql.ErrNoRows || closed {
		return doc, accounting.Fail("PERIOD_CLOSED", "Kỳ kế toán chưa mở hoặc đã khóa.")
	}
	if err != nil {
		return doc, err
	}
	d, c := new(big.Int), new(big.Int)
	for _, line := range doc.Lines {
		dv, _ := accounting.Money(line.Debit)
		cv, _ := accounting.Money(line.Credit)
		d.Add(d, big.NewInt(dv))
		c.Add(c, big.NewInt(cv))
	}
	if d.Cmp(c) != 0 || d.Sign() == 0 {
		return doc, accounting.Fail("UNBALANCED_ENTRY", "Tổng Nợ phải bằng tổng Có và lớn hơn 0.")
	}
	var number, order int64
	scope := doc.Type + ":" + doc.Date[:4]
	if IsOpening(doc.SourceType) {
		scope = doc.SourceType + ":" + doc.Date[:4]
	}
	prefix := map[string]string{"receipt": "PT", "payment": "PC", "sale": "BH", "purchase": "MH", "journal": "CT"}[doc.Type]
	if doc.SourceType == "opening" {
		prefix = "SDDK"
	}
	if doc.SourceType == "opening_adjustment" {
		prefix = "SDDC"
	}
	if doc.SourceType == "opening_adjustment_reversal" {
		prefix = "SDDC-D"
	}
	var existingRef string
	var existingPostedAt sql.NullString
	var existingPostedOrder sql.NullInt64
	if err = tx.QueryRowContext(ctx, "SELECT reference,posted_at,posted_order FROM documents WHERE id=?", doc.ID).Scan(&existingRef, &existingPostedAt, &existingPostedOrder); err != nil {
		return doc, err
	}
	expectedPrefix := fmt.Sprintf("%s-%s-", prefix, doc.Date[:4])
	if existingPostedOrder.Valid && strings.HasPrefix(existingRef, expectedPrefix) {
		doc.Reference = existingRef
		order = existingPostedOrder.Int64
	} else {
		if err = tx.QueryRowContext(ctx, "INSERT INTO counters VALUES(?,1) ON CONFLICT(scope) DO UPDATE SET value=value+1 RETURNING value", scope).Scan(&number); err != nil {
			return doc, err
		}
		if err = tx.QueryRowContext(ctx, "INSERT INTO counters VALUES('posting-order',1) ON CONFLICT(scope) DO UPDATE SET value=value+1 RETURNING value").Scan(&order); err != nil {
			return doc, err
		}
		doc.Reference = fmt.Sprintf("%s-%s-%06d", prefix, doc.Date[:4], number)
	}
	postedAt := sqlite.Now()
	if existingPostedAt.Valid && existingPostedAt.String != "" {
		postedAt = existingPostedAt.String
	}
	doc.Status = "posted"
	doc.Version++
	_, err = tx.ExecContext(ctx, "UPDATE documents SET reference=?,status='posted',version=?,posted_at=?,posted_order=? WHERE id=?", doc.Reference, doc.Version, postedAt, order, doc.ID)
	return doc, err
}
func (s *Service) Reverse(ctx context.Context, cmd Command, input Reversal) (json.RawMessage, error) {
	return s.Write(ctx, cmd, "document.reverse", input, func(tx *sql.Tx) (any, error) {
		original, err := ReadDocument(ctx, tx, input.ID)
		if err != nil {
			return nil, err
		}
		if original.Status != "posted" || !accounting.Date(input.Date) || input.Date < original.Date || strings.TrimSpace(input.Reason) == "" {
			return nil, accounting.Fail("VALIDATION", "Chọn chứng từ đã ghi sổ, ngày đảo và lý do.")
		}
		if original.PolicyID != "" {
			return nil, accounting.Fail("SUBLEDGER_REVERSAL_REQUIRED", "Chứng từ có biến động phân hệ cần điều chỉnh qua nghiệp vụ nguồn.")
		}
		var exists int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE reversal_of=?", input.ID).Scan(&exists); err != nil {
			return nil, err
		}
		if exists > 0 {
			return nil, accounting.Fail("DUPLICATE_SOURCE", "Chứng từ đã có chứng từ đảo.")
		}
		doc := Document{ID: sqlite.ID(), Type: "journal", Date: input.Date, Description: "Đảo " + original.Reference + ": " + input.Reason, SourceType: "reversal", SourceID: original.ID, ReversalOf: original.ID}
		for _, line := range original.Lines {
			line.Debit, line.Credit = line.Credit, line.Debit
			doc.Lines = append(doc.Lines, line)
		}
		doc, err = s.SaveTx(ctx, tx, doc)
		if err != nil {
			return nil, err
		}
		return s.PostTx(ctx, tx, Post{doc.ID, doc.Version})
	})
}
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func IsOpening(source string) bool {
	return source == "opening" || source == "opening_adjustment" || source == "opening_adjustment_reversal"
}

// PrepareOpeningReversalTx copies only an immutable opening document; inactive historical references remain reversible.
func (s *Service) PrepareOpeningReversalTx(ctx context.Context, tx *sql.Tx, id, reason string) (Document, error) {
	original, err := ReadDocument(ctx, tx, id)
	if err != nil {
		return original, err
	}
	if original.Status != "posted" || (original.SourceType != "opening" && original.SourceType != "opening_adjustment") {
		return original, accounting.Fail("VALIDATION", "Chỉ đảo phiên bản số dư đầu kỳ đã xác nhận.")
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE reversal_of=?", id).Scan(&count); err != nil {
		return original, err
	}
	if count != 0 {
		return original, accounting.Fail("VERSION_CONFLICT", "Phiên bản số dư này đã được điều chỉnh.")
	}
	doc := Document{ID: sqlite.ID(), Type: "journal", Date: original.Date, Description: "Đảo số dư " + original.Reference, SourceType: "opening_adjustment_reversal", SourceID: id, ReversalOf: id, PolicyID: original.PolicyID}
	for _, line := range original.Lines {
		line.Debit, line.Credit = line.Credit, line.Debit
		doc.Lines = append(doc.Lines, line)
	}
	doc, err = s.saveTx(ctx, tx, doc, true)
	if err != nil {
		return doc, err
	}
	return doc, nil
}

func (s *Service) PostOpeningReversalTx(ctx context.Context, tx *sql.Tx, in Post) (Document, error) {
	doc, err := ReadDocument(ctx, tx, in.ID)
	if err != nil {
		return doc, err
	}
	if doc.SourceType != "opening_adjustment_reversal" || doc.ReversalOf == "" {
		return doc, accounting.Fail("VALIDATION", "Không phải chứng từ đảo số dư đầu kỳ.")
	}
	return s.postTx(ctx, tx, in, true)
}
