package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
)

var AuditTitles = map[string]string{
	"setup": "Khởi tạo doanh nghiệp", "document.save": "Lưu nháp chứng từ", "document.post": "Ghi sổ chứng từ", "document.reverse": "Đảo chứng từ", "document.import": "Nhập chứng từ CSV", "voucher.save": "Lưu nháp nghiệp vụ",
	"partner.save": "Cập nhật khách hàng / nhà cung cấp", "admin.account.save": "Cập nhật tài khoản kế toán", "admin.account.reference": "Nạp hệ thống tài khoản", "bank.save": "Cập nhật tài khoản ngân hàng", "warehouse.save": "Cập nhật kho", "item.save": "Cập nhật hàng hóa / dịch vụ", "admin.employee.save": "Cập nhật nhân viên", "admin.policy.save": "Thêm phiên bản chính sách", "admin.period.save": "Mở / khóa kỳ kế toán", "admin.period.reopen": "Mở lại kỳ kế toán", "financial.save": "Lưu bản nháp báo cáo tài chính", "attachment.add": "Đính kèm chứng từ", "admin.audit.restore": "Khôi phục thao tác trước đó",
	"admin.opening.save": "Lưu nháp số dư đầu kỳ", "admin.opening.post": "Xác nhận số dư đầu kỳ", "admin.opening.discard": "Bỏ nháp số dư đầu kỳ", "admin.opening.adjustment.save": "Lưu nháp điều chỉnh số dư", "admin.opening.adjustment.confirm": "Xác nhận điều chỉnh số dư", "admin.opening.adjustment.discard": "Bỏ nháp điều chỉnh số dư", "admin.user.save": "Cập nhật người dùng / quyền truy cập", "excel.stage": "Đọc tệp nhập Excel", "excel.discard": "Bỏ tệp nhập Excel",
	"operation.receipt": "Ghi nhận thu tiền", "operation.payment": "Ghi nhận chi tiền", "operation.transfer": "Chuyển tiền nội bộ", "operation.sale": "Ghi nhận bán hàng", "operation.purchase": "Ghi nhận mua hàng", "operation.closing": "Kết chuyển kết quả kinh doanh", "operation.payroll": "Tính và ghi nhận tiền lương", "operation.asset": "Ghi tăng tài sản / chi phí trả trước", "operation.depreciation": "Khấu hao / phân bổ", "operation.costing": "Tập hợp giá thành", "operation.corporate_tax": "Tính thuế thu nhập doanh nghiệp",
}

func AuditTitle(action string) string {
	if title, ok := AuditTitles[action]; ok {
		return title
	}
	if strings.HasPrefix(action, "excel.") {
		if strings.HasSuffix(action, ".configure") {
			return "Lưu cấu hình ghép cột Excel"
		}
		return "Nhập dữ liệu từ Excel"
	}
	return "Thao tác hệ thống"
}
func auditMetadata(ctx context.Context, tx *sql.Tx, id int64, action string) (sqlite.AuditMetadata, error) {
	m := sqlite.AuditMetadata{Title: AuditTitle(action)}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM audit_changes WHERE audit_id=? AND before_state IS NOT after_state", id).Scan(&m.Count); err != nil {
		return m, err
	}
	var table, raw string
	err := tx.QueryRowContext(ctx, `SELECT table_name,COALESCE(after_state,before_state) FROM audit_changes WHERE audit_id=? AND before_state IS NOT after_state ORDER BY CASE table_name WHEN 'documents' THEN 0 WHEN 'partners' THEN 1 WHEN 'accounts' THEN 2 WHEN 'bank_accounts' THEN 3 WHEN 'financial_reports' THEN 4 WHEN 'items' THEN 5 WHEN 'warehouses' THEN 6 WHEN 'employees' THEN 7 WHEN 'users' THEN 8 ELSE 10 END,row_key LIMIT 1`, id).Scan(&table, &raw)
	if err != nil && err != sql.ErrNoRows {
		return m, err
	}
	fields := map[string]*string{}
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &fields); err != nil {
			return m, err
		}
	}
	value := func(key string) string {
		if v := fields[key]; v != nil {
			return *v
		}
		return ""
	}
	m.EntityType = table
	m.EntityID = value("id")
	if m.EntityID == "" {
		m.EntityID = value("document_id")
	}
	if m.EntityID == "" {
		m.EntityID = value("period")
	}
	for _, key := range []string{"reference", "code", "number", "name", "username", "period"} {
		if text := value(key); text != "" {
			if m.EntityName != "" {
				m.EntityName += " · "
			}
			m.EntityName += text
		}
	}
	if m.EntityName == "" {
		m.EntityName = m.EntityID
	}
	if table == "documents" && value("description") != "" {
		description := []rune(value("description"))
		if len(description) > 120 {
			description = append(description[:120], '…')
		}
		m.EntityName += " · " + string(description)
	}
	if table == "bank_accounts" {
		m.EntityName += " · " + value("bank_name")
	}
	if table == "financial_reports" {
		m.EntityName = "BCTC " + value("from_date") + " → " + value("to_date")
	}
	if table == "policies" {
		m.EntityName = "Phiên bản " + value("version") + " · " + value("effective_from")
	}
	if table == "excel_imports" {
		m.EntityName = value("filename")
	}
	if table == "account_hierarchy" {
		m.EntityID = value("account_id")
		var code, name string
		if err = tx.QueryRowContext(ctx, "SELECT code,name FROM accounts WHERE id=?", m.EntityID).Scan(&code, &name); err != nil {
			return m, err
		}
		m.EntityName = code + " · " + name
		m.EntityType = "accounts"
	}

	if table == "documents" {
		names := map[string]string{"receipt": "phiếu thu", "payment": "phiếu chi", "sale": "chứng từ bán hàng", "purchase": "chứng từ mua hàng", "journal": "chứng từ khác"}
		if name := names[value("type")]; name != "" {
			switch action {
			case "voucher.save", "document.save":
				m.Title = "Lưu nháp " + name
			case "document.post":
				m.Title = "Ghi sổ " + name
			}
		}
	}
	var actor string
	if err = tx.QueryRowContext(ctx, "SELECT actor_name FROM audit_context WHERE audit_id=?", id).Scan(&actor); err != nil {
		return m, err
	}
	m.Search = search(actor + " " + m.Title + " " + m.EntityName + " " + m.EntityID + " " + value("description"))
	return m, nil
}

type AuditEntry struct {
	TargetExists    bool                 `json:"targetExists"`
	RestoredBy      string               `json:"restoredBy,omitempty"`
	ChangesTotal    int                  `json:"changesTotal"`
	ChangesPage     int                  `json:"changesPage"`
	ChangesPageSize int                  `json:"changesPageSize"`
	BaseAction      string               `json:"baseAction"`
	ID              string               `json:"id"`
	ActorID         string               `json:"actorId"`
	Actor           string               `json:"actor"`
	Action          string               `json:"action"`
	Title           string               `json:"title"`
	EntityType      string               `json:"entityType"`
	EntityID        string               `json:"entityId"`
	EntityName      string               `json:"entityName"`
	OccurredAt      string               `json:"occurredAt"`
	Revision        int64                `json:"revision"`
	Count           int                  `json:"count"`
	Reason          string               `json:"reason"`
	RestoredFrom    string               `json:"restoredFrom,omitempty"`
	Legacy          bool                 `json:"legacy"`
	Changes         []sqlite.AuditChange `json:"changes,omitempty"`
}
type AuditOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type AuditFilter struct {
	Query, Actor, Action, From, To string
	Page, PageSize                 int
}
type AuditPage struct {
	Context
	Items   []AuditEntry  `json:"items"`
	Total   int           `json:"total"`
	Actors  []AuditOption `json:"actors"`
	Actions []AuditOption `json:"actions"`
}

const auditSelect = `SELECT CAST(a.id AS TEXT),a.actor_id,COALESCE(d.actor_name,u.name),a.action,COALESCE(d.base_action,a.action),COALESCE(d.title,''),COALESCE(d.entity_type,''),COALESCE(d.entity_id,''),COALESCE(d.entity_name,''),a.occurred_at,a.revision,COALESCE(d.change_count,0),COALESCE(d.reason,''),COALESCE(CAST(d.restored_from AS TEXT),''),COALESCE((SELECT CAST(r.audit_id AS TEXT) FROM audit_details r WHERE r.restored_from=a.id ORDER BY r.audit_id DESC LIMIT 1),''),d.audit_id IS NULL FROM audit a JOIN users u ON u.id=a.actor_id LEFT JOIN audit_details d ON d.audit_id=a.id`

func scanAudit(row interface{ Scan(...any) error }) (AuditEntry, error) {
	var e AuditEntry
	err := row.Scan(&e.ID, &e.ActorID, &e.Actor, &e.Action, &e.BaseAction, &e.Title, &e.EntityType, &e.EntityID, &e.EntityName, &e.OccurredAt, &e.Revision, &e.Count, &e.Reason, &e.RestoredFrom, &e.RestoredBy, &e.Legacy)
	if e.Title == "" {
		e.Title = AuditTitle(e.Action)
	}
	return e, err
}
func ReadAuditTx(ctx context.Context, tx *sql.Tx, id int64) (AuditEntry, error) {
	e, err := scanAudit(tx.QueryRowContext(ctx, auditSelect+" WHERE a.id=?", id))
	if err == sql.ErrNoRows {
		return e, accounting.Fail("NOT_FOUND", "Nhật ký không tồn tại.")
	}
	if err != nil {
		return e, err
	}
	if e.Count > 20000 {
		return e, accounting.Fail("HISTORY_LOCKED", "Thao tác hàng loạt vượt giới hạn khôi phục tự động. Mở các nghiệp vụ liên quan để xử lý.")
	}
	e.Changes, err = sqlite.ReadAuditChanges(ctx, tx, id)
	return e, err
}
func (s *Service) AuditDetail(ctx context.Context, id int64, page, pageSize int) (AuditEntry, error) {
	e := AuditEntry{}
	if page < 1 || page > 1000000 || pageSize < 1 || pageSize > 100 {
		return e, accounting.Fail("BAD_REQUEST", "Phân trang chi tiết nhật ký không hợp lệ.")
	}
	tx, err := s.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return e, err
	}
	defer tx.Rollback()
	e, err = scanAudit(tx.QueryRowContext(ctx, auditSelect+" WHERE a.id=?", id))
	if err == sql.ErrNoRows {
		return e, accounting.Fail("NOT_FOUND", "Nhật ký không tồn tại.")
	}
	if err != nil {
		return e, err
	}
	if column, ok := map[string]string{"documents": "id", "partners": "id", "accounts": "id", "bank_accounts": "id", "warehouses": "id", "items": "id", "employees": "id", "financial_reports": "id", "users": "id", "periods": "period", "opening_balances": "document_id", "opening_adjustments": "document_id", "excel_imports": "id"}[e.EntityType]; ok && e.EntityID != "" {
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+e.EntityType+" WHERE "+column+"=?)", e.EntityID).Scan(&e.TargetExists); err != nil {
			return e, err
		}
	}
	e.ChangesTotal = e.Count
	e.ChangesPage = page
	e.ChangesPageSize = pageSize
	rows, err := tx.QueryContext(ctx, "SELECT table_name,row_key,before_state,after_state FROM audit_changes WHERE audit_id=? AND before_state IS NOT after_state ORDER BY table_name,row_key LIMIT ? OFFSET ?", id, pageSize, (page-1)*pageSize)
	if err != nil {
		return e, err
	}
	e.Changes = []sqlite.AuditChange{}
	for rows.Next() {
		var c sqlite.AuditChange
		var before, after sql.NullString
		if err = rows.Scan(&c.Table, &c.Key, &before, &after); err != nil {
			rows.Close()
			return e, err
		}
		if before.Valid {
			c.Before = json.RawMessage(before.String)
		}
		if after.Valid {
			c.After = json.RawMessage(after.String)
		}
		e.Changes = append(e.Changes, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return e, err
	}
	return e, tx.Commit()
}

func auditDateBound(value string, end bool) (string, error) {
	if value == "" {
		return "", nil
	}
	if !accounting.Date(value) {
		return "", accounting.Fail("BAD_REQUEST", "Ngày lọc nhật ký không hợp lệ.")
	}
	t, err := time.Parse(time.RFC3339, value+"T00:00:00+07:00")
	if err != nil {
		return "", err
	}
	if end {
		t = t.AddDate(0, 0, 1)
	}
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z"), nil
}
func (s *Service) Audit(ctx context.Context, filter AuditFilter) (AuditPage, error) {
	out := AuditPage{Items: []AuditEntry{}, Actors: []AuditOption{}, Actions: []AuditOption{}}
	if filter.Page < 1 || filter.Page > 1000000 || filter.PageSize < 1 || filter.PageSize > 100 || len(filter.Query) > 300 || len(filter.Action) > 100 || len(filter.Actor) > 100 || filter.From != "" && filter.To != "" && filter.From > filter.To {
		return out, accounting.Fail("BAD_REQUEST", "Bộ lọc nhật ký không hợp lệ.")
	}
	from, err := auditDateBound(filter.From, false)
	if err != nil {
		return out, err
	}
	to, err := auditDateBound(filter.To, true)
	if err != nil {
		return out, err
	}
	tx, err := s.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	out.Context, err = ReadContext(ctx, tx)
	if err != nil {
		return out, err
	}
	where := ` WHERE (?='' OR a.actor_id=?) AND (?='' OR a.action=?) AND (?='' OR a.occurred_at>=?) AND (?='' OR a.occurred_at<?) AND (?='' OR instr(COALESCE(d.search_text,lower(u.name)||' '||a.action||' '||a.entity_id),?)>0)`
	q := search(filter.Query)
	args := []any{filter.Actor, filter.Actor, filter.Action, filter.Action, from, from, to, to, q, q}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM audit a JOIN users u ON u.id=a.actor_id LEFT JOIN audit_details d ON d.audit_id=a.id"+where, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, auditSelect+where+" ORDER BY a.id DESC LIMIT ? OFFSET ?", append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT DISTINCT a.actor_id,u.name FROM audit a JOIN users u ON u.id=a.actor_id ORDER BY u.name,a.actor_id")
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var o AuditOption
		if err = rows.Scan(&o.Value, &o.Label); err != nil {
			rows.Close()
			return out, err
		}
		out.Actors = append(out.Actors, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT DISTINCT action FROM audit ORDER BY action")
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var o AuditOption
		if err = rows.Scan(&o.Value); err != nil {
			rows.Close()
			return out, err
		}
		o.Label = AuditTitle(o.Value)
		out.Actions = append(out.Actions, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func AuditID(value string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 {
		return 0, accounting.Fail("BAD_REQUEST", "Mã nhật ký không hợp lệ.")
	}
	return n, nil
}
