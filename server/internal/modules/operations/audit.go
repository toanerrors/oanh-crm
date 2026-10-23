package operations

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

type AuditRestoreInput struct {
	AuditID          string `json:"auditId"`
	Reason           string `json:"reason"`
	ExpectedRevision *int64 `json:"expectedRevision,omitempty"`
	Fingerprint      string `json:"fingerprint,omitempty"`
}
type AuditRestorePreview struct {
	AuditID     string               `json:"auditId"`
	Revision    int64                `json:"revision"`
	Fingerprint string               `json:"fingerprint"`
	Changes     []sqlite.AuditChange `json:"changes"`
}

func AuditRestoreSupport(e ledger.AuditEntry) (bool, string) {
	if e.RestoredBy != "" {
		return false, "Thao tác này đã được khôi phục. Mở nhật ký khôi phục liên kết để tiếp tục hoàn tác/khôi phục lại."
	}
	if e.Legacy {
		return false, "Nhật ký này chưa lưu dữ liệu trước/sau nên không thể khôi phục tự động."
	}
	switch e.BaseAction {
	case "document.save", "voucher.save", "partner.save", "admin.account.save", "bank.save", "warehouse.save", "item.save", "admin.employee.save", "financial.save", "admin.opening.save", "admin.opening.discard", "admin.opening.adjustment.save", "admin.opening.adjustment.discard":
		if e.Count == 0 {
			return false, "Thao tác không làm thay đổi dữ liệu."
		}
		for _, c := range e.Changes {
			if c.Table == "documents" {
				for _, raw := range []json.RawMessage{c.Before, c.After} {
					var state map[string]*string
					if len(raw) > 0 && json.Unmarshal(raw, &state) == nil && state["status"] != nil && *state["status"] == "posted" {
						return false, "Chứng từ đã ghi sổ: mở chứng từ gốc để lập điều chỉnh hoặc đảo có liên kết."
					}
				}
			}
		}
		return true, "Dữ liệu sẽ được kiểm tra lại và lưu thành một phiên bản mới, có nhật ký liên kết."
	}
	if strings.HasPrefix(e.BaseAction, "operation.") || e.BaseAction == "document.post" || e.BaseAction == "document.reverse" || strings.HasSuffix(e.BaseAction, ".confirm") || e.BaseAction == "admin.opening.post" {
		return false, "Số liệu đã ghi nhận cần điều chỉnh/đảo ở nghiệp vụ gốc; giữ nguyên lịch sử đã xác nhận."
	}
	if e.BaseAction == "admin.user.save" {
		return false, "Quyền và mật khẩu không khôi phục tự động. Mở mục Người dùng để cập nhật có kiểm soát."
	}
	if strings.HasPrefix(e.BaseAction, "excel.") {
		return false, "Nhập hàng loạt giữ lịch sử nguồn. Mở các chứng từ/danh mục liên quan để xử lý từng nghiệp vụ."
	}
	return false, "Thao tác này cần được xử lý tại màn hình nghiệp vụ gốc."
}

func (s *Service) PreviewAuditRestore(ctx context.Context, cmd ledger.Command, in AuditRestoreInput) (AuditRestorePreview, error) {
	out := AuditRestorePreview{}
	tx, err := s.Ledger.Store.Write.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = s.Ledger.CheckWriteScopeTx(ctx, tx, cmd, "admin.audit.restore"); err != nil {
		return out, err
	}
	e, fingerprint, revision, err := s.auditRestoreSource(ctx, tx, in)
	if err != nil {
		return out, err
	}
	id, err := sqlite.BeginAudit(ctx, tx, cmd.Actor)
	if err != nil {
		return out, err
	}
	if _, err = s.applyAuditRestore(ctx, tx, cmd, e); err != nil {
		return out, err
	}
	changes, err := sqlite.ReadAuditChanges(ctx, tx, id)
	if len(changes) == 0 && err == nil {
		return out, accounting.Fail("HISTORY_LOCKED", "Không có thay đổi để khôi phục.")
	}
	if err != nil {
		return out, err
	}
	return AuditRestorePreview{AuditID: e.ID, Revision: revision, Fingerprint: fingerprint, Changes: changes}, nil
}
func (s *Service) RestoreAudit(ctx context.Context, cmd ledger.Command, in AuditRestoreInput) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "admin.audit.restore", in, func(tx *sql.Tx) (any, error) {
		e, fingerprint, revision, err := s.auditRestoreSource(ctx, tx, in)
		if err != nil {
			return nil, err
		}
		if in.ExpectedRevision == nil || *in.ExpectedRevision != revision || in.Fingerprint != fingerprint {
			return nil, accounting.Fail("PREVIEW_REQUIRED", "Dữ liệu hoặc lý do đã thay đổi. Xem trước khôi phục lại trước khi xác nhận.")
		}
		value, err := s.applyAuditRestore(ctx, tx, cmd, e)
		if err != nil {
			return nil, err
		}
		source, _ := ledger.AuditID(e.ID)
		if _, err = tx.ExecContext(ctx, "UPDATE audit_context SET reason=?,restored_from=?,base_action=? WHERE id=1", strings.TrimSpace(in.Reason), source, e.BaseAction); err != nil {
			return nil, err
		}
		var id int64
		if err = tx.QueryRowContext(ctx, "SELECT audit_id FROM audit_context WHERE id=1").Scan(&id); err != nil {
			return nil, err
		}
		return struct {
			AuditID      string `json:"auditId"`
			RestoredFrom string `json:"restoredFrom"`
			Value        any    `json:"value"`
		}{strconv.FormatInt(id, 10), e.ID, value}, nil
	})
}
func (s *Service) auditRestoreSource(ctx context.Context, tx *sql.Tx, in AuditRestoreInput) (ledger.AuditEntry, string, int64, error) {
	var e ledger.AuditEntry
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > 1000 {
		return e, "", 0, accounting.Fail("VALIDATION", "Nhập lý do khôi phục, tối đa 1.000 ký tự.")
	}
	id, err := ledger.AuditID(in.AuditID)
	if err != nil {
		return e, "", 0, err
	}
	e, err = ledger.ReadAuditTx(ctx, tx, id)
	if err != nil {
		return e, "", 0, err
	}
	if allowed, why := AuditRestoreSupport(e); !allowed {
		return e, "", 0, accounting.Fail("HISTORY_LOCKED", why)
	}
	for _, change := range e.Changes {
		current, err := sqlite.AuditRow(ctx, tx, change.Table, change.Key)
		if err != nil {
			return e, "", 0, err
		}
		if change.Table == "documents" && len(current) > 0 {
			state, decodeErr := decodeAudit(current)
			if decodeErr != nil {
				return e, "", 0, decodeErr
			}
			if state.text("status") == "posted" {
				return e, "", 0, accounting.Fail("DOCUMENT_POSTED", "Chứng từ đã ghi sổ. Mở nghiệp vụ gốc để điều chỉnh/đảo có liên kết.")
			}
		}
		var later int
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM audit_changes WHERE table_name=? AND row_key=? AND audit_id>?)", change.Table, change.Key, id).Scan(&later); err != nil {
			return e, "", 0, err
		}
		if later > 0 || !bytes.Equal(current, change.After) {
			return e, "", 0, accounting.Fail("VERSION_CONFLICT", "Dữ liệu này đã có thao tác mới hơn. Chọn nhật ký mới nhất để tránh ghi đè.")
		}
	}
	var revision int64
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM meta WHERE id=1").Scan(&revision); err != nil {
		return e, "", 0, err
	}
	raw, err := json.Marshal(struct {
		ID, Reason string
		Revision   int64
		Changes    []sqlite.AuditChange
	}{e.ID, reason, revision, e.Changes})
	if err != nil {
		return e, "", 0, err
	}
	return e, sqlite.Hash(raw), revision, nil
}

type auditState map[string]*string

func (r auditState) text(key string) string {
	if r[key] == nil {
		return ""
	}
	return *r[key]
}
func (r auditState) flag(key string) bool { return r.text(key) == "1" }
func decodeAudit(raw json.RawMessage) (auditState, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var r auditState
	err := json.Unmarshal(raw, &r)
	return r, err
}
func auditBefore(e ledger.AuditEntry, table string) []auditState {
	values := []auditState{}
	for _, c := range e.Changes {
		if c.Table == table && len(c.Before) > 0 {
			r, err := decodeAudit(c.Before)
			if err == nil {
				values = append(values, r)
			}
		}
	}
	return values
}
func auditRoots(e ledger.AuditEntry, table string) []sqlite.AuditChange {
	values := []sqlite.AuditChange{}
	for _, c := range e.Changes {
		if c.Table == table {
			values = append(values, c)
		}
	}
	return values
}
func (s *Service) applyAuditRestore(ctx context.Context, tx *sql.Tx, cmd ledger.Command, e ledger.AuditEntry) (any, error) {
	action := e.BaseAction
	switch action {
	case "document.save", "voucher.save", "admin.opening.save", "admin.opening.discard", "admin.opening.adjustment.save", "admin.opening.adjustment.discard":
		roots := auditRoots(e, "documents")
		if len(roots) != 1 {
			return nil, accounting.Fail("HISTORY_LOCKED", "Chọn thao tác của một chứng từ để khôi phục.")
		}
		before, err := decodeAudit(roots[0].Before)
		if err != nil {
			return nil, err
		}
		after, err := decodeAudit(roots[0].After)
		if err != nil {
			return nil, err
		}
		currentVersion := 0
		id := ""
		if after != nil {
			id = after.text("id")
			currentVersion, err = strconv.Atoi(after.text("version"))
			if err != nil {
				return nil, err
			}
			if after.text("status") != "draft" {
				return nil, accounting.Fail("DOCUMENT_POSTED", "Chứng từ đã ghi sổ cần điều chỉnh/đảo.")
			}
		}
		if before == nil {
			if action == "admin.opening.save" {
				return s.discardOpeningTx(ctx, tx, ledger.Post{ID: id, Version: currentVersion})
			}
			if action == "admin.opening.adjustment.save" {
				return s.discardOpeningAdjustmentTx(ctx, tx, ledger.Post{ID: id, Version: currentVersion})
			}
			var dependencies int
			if err = tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM operations WHERE document_id=?)+(SELECT count(*) FROM attachments WHERE document_id=?)+(SELECT count(*) FROM opening_balances WHERE document_id=?)+(SELECT count(*) FROM opening_adjustments WHERE document_id=?)", id, id, id, id).Scan(&dependencies); err != nil {
				return nil, err
			}
			if dependencies > 0 {
				return nil, accounting.Fail("HISTORY_LOCKED", "Chứng từ có liên kết mới hoặc tệp đính kèm. Mở nghiệp vụ gốc để xử lý.")
			}
			for _, q := range []string{"DELETE FROM voucher_inputs WHERE document_id=?", "DELETE FROM document_lines WHERE document_id=?", "DELETE FROM documents WHERE id=?"} {
				if _, err = tx.ExecContext(ctx, q, id); err != nil {
					return nil, err
				}
			}
			return map[string]string{"discardedDraft": id}, nil
		}
		id = before.text("id")
		if before.text("status") != "draft" {
			return nil, accounting.Fail("DOCUMENT_POSTED", "Không khôi phục trực tiếp chứng từ đã ghi sổ.")
		}
		if action == "admin.opening.save" || action == "admin.opening.discard" {
			rows := auditBefore(e, "opening_balances")
			if len(rows) != 1 {
				return nil, accounting.Fail("INVALID_SOURCE", "Thiếu nội dung số dư trước đó.")
			}
			var in OpeningInput
			if err = json.Unmarshal([]byte(rows[0].text("payload")), &in); err != nil {
				return nil, err
			}
			in.Version = currentVersion
			return s.saveOpeningTx(ctx, tx, in)
		}
		if action == "admin.opening.adjustment.save" || action == "admin.opening.adjustment.discard" {
			rows := auditBefore(e, "opening_adjustments")
			if len(rows) != 1 {
				return nil, accounting.Fail("INVALID_SOURCE", "Thiếu nội dung điều chỉnh trước đó.")
			}
			var input OpeningInput
			if err = json.Unmarshal([]byte(rows[0].text("after_payload")), &input); err != nil {
				return nil, err
			}
			input.Version = currentVersion
			base, err := strconv.Atoi(rows[0].text("base_version"))
			if err != nil {
				return nil, err
			}
			return s.saveOpeningAdjustmentTx(ctx, tx, cmd.Actor, OpeningAdjustmentInput{Input: input, Reason: rows[0].text("reason"), BaseVersion: base})
		}
		if action == "voucher.save" {
			rows := auditBefore(e, "voucher_inputs")
			if len(rows) != 1 {
				return nil, accounting.Fail("INVALID_SOURCE", "Thiếu biểu mẫu nghiệp vụ trước đó.")
			}
			var in Input
			if err = json.Unmarshal([]byte(rows[0].text("payload")), &in); err != nil {
				return nil, err
			}
			in.DraftID = id
			in.DraftVersion = currentVersion
			in.ExpectedRevision = nil
			in.ExpectedPolicyID = ""
			return s.saveVoucherTx(ctx, tx, in)
		}
		doc := ledger.Document{ID: id, Reference: before.text("reference"), Type: before.text("type"), Date: before.text("date"), Description: before.text("description"), PartnerID: before.text("partner_id"), Version: currentVersion, SourceType: before.text("source_type"), SourceID: before.text("source_id")}
		lines := map[int]ledger.Line{}
		if after != nil {
			current, err := ledger.ReadDocument(ctx, tx, id)
			if err != nil {
				return nil, err
			}
			for i, line := range current.Lines {
				lines[i] = line
			}
		}
		for _, change := range auditRoots(e, "document_lines") {
			state, err := decodeAudit(change.Before)
			if err != nil {
				return nil, err
			}
			key := []string{}
			if err = json.Unmarshal([]byte(change.Key), &key); err != nil || len(key) != 2 {
				return nil, accounting.Fail("INVALID_SOURCE", "Khóa dòng hạch toán không hợp lệ.")
			}
			position, err := strconv.Atoi(key[1])
			if err != nil {
				return nil, err
			}
			if state == nil {
				delete(lines, position)
			} else {
				lines[position] = ledger.Line{AccountID: state.text("account_id"), PartnerID: state.text("partner_id"), Description: state.text("description"), Debit: state.text("debit"), Credit: state.text("credit")}
			}
		}
		positions := []int{}
		for position := range lines {
			positions = append(positions, position)
		}
		sort.Ints(positions)
		for i, position := range positions {
			if i != position {
				return nil, accounting.Fail("INVALID_SOURCE", "Thiếu dòng hạch toán trong lịch sử khôi phục.")
			}
			doc.Lines = append(doc.Lines, lines[position])
		}

		return s.Ledger.SaveTx(ctx, tx, doc)
	case "partner.save":
		values := []ledger.Partner{}
		for _, c := range auditRoots(e, "partners") {
			r, err := decodeAudit(c.Before)
			if err != nil {
				return nil, err
			}
			create := r == nil
			if create {
				r, err = decodeAudit(c.After)
				if err != nil {
					return nil, err
				}
			}
			values = append(values, ledger.Partner{ID: r.text("id"), Code: r.text("code"), Name: r.text("name"), Role: r.text("role"), Email: r.text("email"), Phone: r.text("phone"), Active: !create && r.flag("active")})
		}
		return s.Ledger.SavePartnersTx(ctx, tx, values)
	case "admin.account.save":
		roots := auditRoots(e, "accounts")
		var r auditState
		created := false
		if len(roots) == 1 {
			var err error
			r, err = decodeAudit(roots[0].Before)
			if err != nil {
				return nil, err
			}
			if r == nil {
				created = true
				r, err = decodeAudit(roots[0].After)
				if err != nil {
					return nil, err
				}
			}
		} else {
			h := auditRoots(e, "account_hierarchy")
			if len(h) != 1 {
				return nil, accounting.Fail("HISTORY_LOCKED", "Không xác định được tài khoản cần khôi phục.")
			}
			v, err := decodeAudit(h[0].After)
			if err != nil {
				return nil, err
			}
			raw, err := sqlite.AuditRow(ctx, tx, "accounts", `["`+v.text("account_id")+`"]`)
			if err != nil {
				return nil, err
			}
			r, err = decodeAudit(raw)
			if err != nil {
				return nil, err
			}
		}
		id := r.text("id")
		var later int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM account_hierarchy WHERE account_id=? AND revision>?", id, e.Revision).Scan(&later); err != nil {
			return nil, err
		}
		if later > 0 {
			return nil, accounting.Fail("VERSION_CONFLICT", "Quan hệ tài khoản đã có thay đổi mới hơn.")
		}
		var parent sql.NullString
		err := tx.QueryRowContext(ctx, "SELECT parent_id FROM account_hierarchy WHERE account_id=? AND revision<? ORDER BY revision DESC LIMIT 1", id, e.Revision).Scan(&parent)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		current, revision, err := s.Ledger.AccountParentTx(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		target := parent.String
		if created {
			target = current
		}
		return s.Ledger.SaveAccountTx(ctx, tx, ledger.Account{ID: id, Code: r.text("code"), Name: r.text("name"), Group: r.text("account_group"), RequiresPartner: r.flag("requires_partner"), Active: !created && r.flag("active"), ParentID: &target, ParentRevision: revision})
	case "bank.save", "warehouse.save", "item.save", "admin.employee.save":
		table := map[string]string{"bank.save": "bank_accounts", "warehouse.save": "warehouses", "item.save": "items", "admin.employee.save": "employees"}[action]
		rows := auditRoots(e, table)
		if len(rows) != 1 {
			return nil, accounting.Fail("HISTORY_LOCKED", "Chọn thao tác của một bản ghi danh mục.")
		}
		r, err := decodeAudit(rows[0].Before)
		if err != nil {
			return nil, err
		}
		created := r == nil
		if created {
			r, err = decodeAudit(rows[0].After)
			if err != nil {
				return nil, err
			}
		}
		active := !created && r.flag("active")
		switch action {
		case "bank.save":
			return s.saveBankTx(ctx, tx, BankAccount{ID: r.text("id"), Number: r.text("number"), BankName: r.text("bank_name"), AccountID: r.text("account_id"), Active: active})
		case "warehouse.save":
			return s.saveWarehouseTx(ctx, tx, Warehouse{ID: r.text("id"), Code: r.text("code"), Name: r.text("name"), Active: active})
		case "item.save":
			return s.saveItemTx(ctx, tx, Item{ID: r.text("id"), Code: r.text("code"), Name: r.text("name"), Unit: r.text("unit"), Tracked: r.flag("tracked"), Active: active})
		default:
			dependents, err := strconv.Atoi(r.text("dependents"))
			if err != nil {
				return nil, err
			}
			return s.saveEmployeeTx(ctx, tx, Employee{ID: r.text("id"), Code: r.text("code"), Name: r.text("name"), BaseSalary: r.text("base_salary"), InsuranceBase: r.text("insurance_base"), Dependents: dependents, Active: active})
		}
	case "financial.save":
		roots := auditRoots(e, "financial_reports")
		if len(roots) != 1 {
			return nil, accounting.Fail("INVALID_SOURCE", "Không xác định được bản báo cáo.")
		}
		before, err := decodeAudit(roots[0].Before)
		if err != nil {
			return nil, err
		}
		after, err := decodeAudit(roots[0].After)
		if err != nil {
			return nil, err
		}
		if before == nil {
			_, err = tx.ExecContext(ctx, "DELETE FROM financial_reports WHERE id=?", after.text("id"))
			return map[string]string{"discardedReport": after.text("id")}, err
		}
		var input ledger.FinancialInput
		if err = json.Unmarshal([]byte(before.text("data")), &input); err != nil {
			return nil, err
		}
		input.Version = 0
		if after != nil {
			input.Version, err = strconv.Atoi(after.text("version"))
			if err != nil {
				return nil, err
			}
		}
		return s.Ledger.SaveFinancialTx(ctx, tx, cmd.Actor, input)
	}
	return nil, accounting.Fail("HISTORY_LOCKED", "Thao tác không hỗ trợ khôi phục tự động.")
}
