package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
)

type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ReadDocument(ctx context.Context, q Queryer, id string) (Document, error) {
	var d Document
	var partner, reversal, policy sql.NullString
	err := q.QueryRowContext(ctx, "SELECT id,reference,type,date,description,partner_id,status,version,source_type,source_id,reversal_of,policy_id,EXISTS(SELECT 1 FROM voucher_inputs v WHERE v.document_id=documents.id) FROM documents WHERE id=?", id).Scan(&d.ID, &d.Reference, &d.Type, &d.Date, &d.Description, &partner, &d.Status, &d.Version, &d.SourceType, &d.SourceID, &reversal, &policy, &d.Voucher)
	if err == sql.ErrNoRows {
		return d, accounting.Fail("NOT_FOUND", "Không tìm thấy chứng từ.")
	}
	if err != nil {
		return d, err
	}
	d.PartnerID = partner.String
	d.ReversalOf = reversal.String
	d.PolicyID = policy.String
	d.Lines = []Line{}
	rows, err := q.QueryContext(ctx, "SELECT account_id,COALESCE(partner_id,''),description,debit,credit,partner_name,account_name FROM document_lines WHERE document_id=? ORDER BY position", id)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var line Line
		var debit, credit int64
		if err = rows.Scan(&line.AccountID, &line.PartnerID, &line.Description, &debit, &credit, &line.PartnerName, &line.AccountName); err != nil {
			return d, err
		}
		line.Debit = accounting.Text(debit)
		line.Credit = accounting.Text(credit)
		d.Lines = append(d.Lines, line)
	}
	return d, rows.Err()
}
func ReadContext(ctx context.Context, q Queryer) (Context, error) {
	var c Context
	err := q.QueryRowContext(ctx, "SELECT company_id,company_name,epoch,revision,currency,regime,COALESCE((SELECT o.cutoff FROM opening_balances o JOIN documents d ON d.id=o.document_id WHERE d.status='posted'),'') FROM meta WHERE id=1").Scan(&c.CompanyID, &c.CompanyName, &c.Epoch, &c.Revision, &c.Currency, &c.Regime, &c.ConversionDate)
	return c, err
}
func (s *Service) Snapshot(ctx context.Context, actor, role, period string) (Workspace, error) {
	w := Workspace{Actor: actor, Permissions: role, Mode: "live", Documents: []Document{}, Accounts: []Account{}, Partners: []Partner{}, Periods: []Period{}}
	tx, err := s.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return w, err
	}
	defer tx.Rollback()
	w.Context, err = ReadContext(ctx, tx)
	if err != nil {
		return w, err
	}
	w.Period = period
	w.Server, err = s.Metrics(ctx, tx, period)
	if err != nil {
		return w, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT a.id,a.code,a.name,a.account_group,a.requires_partner,a.active,COALESCE(h.parent_id,''),COALESCE(h.revision,0),r.account_id IS NOT NULL FROM accounts a LEFT JOIN account_current_parents h ON h.account_id=a.id LEFT JOIN (SELECT DISTINCT account_id FROM document_lines) r ON r.account_id=a.id ORDER BY a.code")
	if err != nil {
		return w, err
	}
	for rows.Next() {
		var a Account
		if err = rows.Scan(&a.ID, &a.Code, &a.Name, &a.Group, &a.RequiresPartner, &a.Active, &a.ParentID, &a.ParentRevision, &a.HasReferences); err != nil {
			rows.Close()
			return w, err
		}
		w.Accounts = append(w.Accounts, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return w, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT p.id,p.code,p.name,p.role,p.email,p.phone,p.active,r.id IS NOT NULL FROM partners p LEFT JOIN (SELECT partner_id AS id FROM document_lines WHERE partner_id IS NOT NULL UNION SELECT partner_id FROM documents WHERE partner_id IS NOT NULL) r ON r.id=p.id ORDER BY p.code")
	if err != nil {
		return w, err
	}
	for rows.Next() {
		var p Partner
		if err = rows.Scan(&p.ID, &p.Code, &p.Name, &p.Role, &p.Email, &p.Phone, &p.Active, &p.HasReferences); err != nil {
			rows.Close()
			return w, err
		}
		w.Partners = append(w.Partners, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return w, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT period,closed FROM periods ORDER BY period DESC")
	if err != nil {
		return w, err
	}
	for rows.Next() {
		var p Period
		if err = rows.Scan(&p.Period, &p.Closed); err != nil {
			rows.Close()
			return w, err
		}
		w.Periods = append(w.Periods, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return w, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT id FROM documents WHERE source_type NOT IN ('opening','opening_adjustment','opening_adjustment_reversal') AND substr(date,1,7)=? ORDER BY date DESC,posted_order DESC,id LIMIT 100", period)
	if err != nil {
		return w, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return w, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return w, err
	}
	for _, id := range ids {
		d, e := ReadDocument(ctx, tx, id)
		if e != nil {
			return w, e
		}
		w.Documents = append(w.Documents, d)
	}
	return w, tx.Commit()
}
func (s *Service) SavePartners(ctx context.Context, cmd Command, partners []Partner) (json.RawMessage, error) {
	return s.Write(ctx, cmd, "partner.save", partners, func(tx *sql.Tx) (any, error) {
		return s.SavePartnersTx(ctx, tx, partners)
	})
}
func (s *Service) SavePartnersTx(ctx context.Context, tx *sql.Tx, partners []Partner) (any, error) {
	if len(partners) == 0 || len(partners) > 100 {
		return nil, accounting.Fail("VALIDATION", "Mỗi lần ghi từ 1 đến 100 đối tượng.")
	}
	for index, p := range partners {
		p.Code = strings.ToUpper(strings.TrimSpace(p.Code))
		p.Name = strings.TrimSpace(p.Name)
		if !accounting.ID(p.ID) || !accounting.ID(p.Code) || p.Name == "" || len(p.Name) > 300 || len(p.Email) > 254 || len(p.Phone) > 40 || !strings.Contains("|customer|supplier|both|", "|"+p.Role+"|") {
			return nil, accounting.Fail("VALIDATION", "Thông tin đối tượng không hợp lệ.")
		}
		partners[index].HasReferences = false
		var old Partner
		err := tx.QueryRowContext(ctx, "SELECT code,name,role FROM partners WHERE id=?", p.ID).Scan(&old.Code, &old.Name, &old.Role)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == nil {
			var count int
			if err = tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM document_lines WHERE partner_id=?)+(SELECT count(*) FROM documents WHERE partner_id=?)", p.ID, p.ID).Scan(&count); err != nil {
				return nil, err
			}
			partners[index].HasReferences = count > 0
			if count > 0 && (old.Code != p.Code || old.Name != p.Name || old.Role != p.Role) {
				return nil, accounting.Fail("HISTORY_LOCKED", "Giữ mã và thông tin đã được chứng từ tham chiếu.")
			}
		}
		var duplicate int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM partners WHERE code=? AND id<>?", p.Code, p.ID).Scan(&duplicate); err != nil {
			return nil, err
		}
		if duplicate > 0 {
			return nil, accounting.Fail("DUPLICATE_CODE", "Mã đối tượng bị trùng.")
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO partners VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET code=excluded.code,name=excluded.name,role=excluded.role,email=excluded.email,phone=excluded.phone,active=excluded.active", p.ID, p.Code, p.Name, p.Role, p.Email, p.Phone, p.Active); err != nil {
			return nil, err
		}
	}
	return partners, nil
}
func (s *Service) SaveAccount(ctx context.Context, cmd Command, a Account) (json.RawMessage, error) {
	return s.Write(ctx, cmd, "admin.account.save", a, func(tx *sql.Tx) (any, error) {
		return s.SaveAccountTx(ctx, tx, a)
	})
}
func (s *Service) SaveAccountTx(ctx context.Context, tx *sql.Tx, a Account) (any, error) {
	if !accounting.ID(a.ID) || !accounting.ID(a.Code) || strings.TrimSpace(a.Name) == "" || !strings.Contains("|cash|receivable|payable|revenue|expense|inventory|capital|asset|tax|liability|wip|", "|"+a.Group+"|") {
		return nil, accounting.Fail("VALIDATION", "Thông tin tài khoản không hợp lệ.")
	}
	a.HasReferences = false
	var old Account
	err := tx.QueryRowContext(ctx, "SELECT code,name,account_group,requires_partner FROM accounts WHERE id=?", a.ID).Scan(&old.Code, &old.Name, &old.Group, &old.RequiresPartner)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		var n int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM document_lines WHERE account_id=?", a.ID).Scan(&n); err != nil {
			return nil, err
		}
		a.HasReferences = n > 0
		if n > 0 && (old.Code != a.Code || old.Name != a.Name || old.Group != a.Group || old.RequiresPartner != a.RequiresPartner) {
			return nil, accounting.Fail("HISTORY_LOCKED", "Không đổi tài khoản đã có lịch sử. Có thể ngừng sử dụng.")
		}
	}
	var bankReferences int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM bank_accounts WHERE account_id=?", a.ID).Scan(&bankReferences); err != nil {
		return nil, err
	}
	if bankReferences > 0 && a.Group != "cash" {
		return nil, accounting.Fail("HISTORY_LOCKED", "Tài khoản kế toán đang gắn ngân hàng phải giữ nhóm tiền.")
	}
	var duplicate int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM accounts WHERE code=? AND id<>?", a.Code, a.ID).Scan(&duplicate); err != nil {
		return nil, err
	}
	if duplicate > 0 {
		return nil, accounting.Fail("DUPLICATE_CODE", "Mã tài khoản đã tồn tại.")
	}
	parent, revision, err := s.AccountParentTx(ctx, tx, a.ID)
	if err != nil {
		return nil, err
	}
	if a.ParentID != nil && *a.ParentID != parent {
		if a.ParentRevision != revision {
			return nil, accounting.Fail("VERSION_CONFLICT", "Quan hệ tài khoản đã thay đổi. Mở lại tài khoản để đối chiếu.")
		}
		if err = s.validateAccountParentTx(ctx, tx, a.ID, *a.ParentID); err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO accounts VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET code=excluded.code,name=excluded.name,account_group=excluded.account_group,requires_partner=excluded.requires_partner,active=excluded.active", a.ID, a.Code, a.Name, a.Group, a.RequiresPartner, a.Active)
	if err != nil {
		return nil, err
	}
	if a.ParentID != nil && *a.ParentID != parent {
		if err = tx.QueryRowContext(ctx, "SELECT revision+1 FROM meta WHERE id=1").Scan(&revision); err != nil {
			return nil, err
		}
		var next any
		if *a.ParentID != "" {
			next = *a.ParentID
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO account_hierarchy(account_id,revision,parent_id) VALUES(?,?,?)", a.ID, revision, next); err != nil {
			return nil, err
		}
		parent = *a.ParentID
	}
	a.ParentID = &parent
	a.ParentRevision = revision
	return a, nil
}
func (s *Service) SavePeriod(ctx context.Context, cmd Command, p Period) (json.RawMessage, error) {
	return s.Write(ctx, cmd, "admin.period.save", p, func(tx *sql.Tx) (any, error) {
		if !accounting.Period(p.Period) {
			return nil, accounting.Fail("VALIDATION", "Kỳ kế toán phải có dạng YYYY-MM.")
		}
		if !p.Closed {
			var n int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM documents d WHERE d.source_type='closing' AND substr(d.date,1,7)=? AND NOT EXISTS (SELECT 1 FROM documents r WHERE r.reversal_of=d.id)", p.Period).Scan(&n); err != nil {
				return nil, err
			}
			if n > 0 {
				return nil, accounting.Fail("PERIOD_FINALIZED", "Kỳ đã kết chuyển kết quả kinh doanh. Cần quy trình điều chỉnh có liên kết trước khi mở lại.")
			}
		}
		if p.Closed {
			rows, err := tx.QueryContext(ctx, "SELECT d.id,l.debit,l.credit FROM documents d JOIN document_lines l ON l.document_id=d.id WHERE d.status='posted' AND substr(d.date,1,7)=? ORDER BY d.id", p.Period)
			if err != nil {
				return nil, err
			}
			sums := map[string]*big.Int{}
			for rows.Next() {
				var id string
				var d, c int64
				if err = rows.Scan(&id, &d, &c); err != nil {
					rows.Close()
					return nil, err
				}
				if sums[id] == nil {
					sums[id] = new(big.Int)
				}
				sums[id].Add(sums[id], big.NewInt(d))
				sums[id].Sub(sums[id], big.NewInt(c))
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
			for _, v := range sums {
				if v.Sign() != 0 {
					return nil, accounting.Fail("UNBALANCED_ENTRY", "Có chứng từ mất cân đối, chưa thể khóa kỳ.")
				}
			}
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO periods VALUES(?,?) ON CONFLICT(period) DO UPDATE SET closed=excluded.closed", p.Period, p.Closed)
		return p, err
	})
}

func (s *Service) ApplyReferenceChart(ctx context.Context, cmd Command, version string) (json.RawMessage, error) {
	return s.Write(ctx, cmd, "admin.account.reference", version, func(tx *sql.Tx) (any, error) {
		chart := accounting.ReferenceChart()
		if version != chart.Version {
			return nil, accounting.Fail("VERSION_CONFLICT", "Phiên bản hệ thống tài khoản tham chiếu đã thay đổi.")
		}
		added := 0
		for _, a := range chart.Accounts {
			result, err := tx.ExecContext(ctx, "INSERT INTO accounts(id,code,name,account_group,requires_partner,active) VALUES(?,?,?,?,?,1) ON CONFLICT(code) DO NOTHING", a.Code, a.Code, a.Name, a.Group, a.RequiresPartner)
			if err != nil {
				return nil, err
			}
			count, err := result.RowsAffected()
			if err != nil {
				return nil, err
			}
			added += int(count)
		}
		var revision int64
		if err := tx.QueryRowContext(ctx, "SELECT revision+1 FROM meta WHERE id=1").Scan(&revision); err != nil {
			return nil, err
		}
		if err := sqlite.SeedAccountHierarchy(ctx, tx, revision); err != nil {
			return nil, err
		}
		return map[string]any{"version": chart.Version, "added": added}, nil
	})
}
