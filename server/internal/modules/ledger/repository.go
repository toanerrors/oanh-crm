package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"strings"

	"oanh.local/accounting/internal/accounting"
)

type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ReadDocument(ctx context.Context, q Queryer, id string) (Document, error) {
	var d Document
	var partner, reversal, policy sql.NullString
	err := q.QueryRowContext(ctx, "SELECT id,reference,type,date,description,partner_id,status,version,source_type,source_id,reversal_of,policy_id FROM documents WHERE id=?", id).Scan(&d.ID, &d.Reference, &d.Type, &d.Date, &d.Description, &partner, &d.Status, &d.Version, &d.SourceType, &d.SourceID, &reversal, &policy)
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
	err := q.QueryRowContext(ctx, "SELECT company_id,company_name,epoch,revision,currency,regime FROM meta WHERE id=1").Scan(&c.CompanyID, &c.CompanyName, &c.Epoch, &c.Revision, &c.Currency, &c.Regime)
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
	rows, err := tx.QueryContext(ctx, "SELECT id,code,name,account_group,requires_partner,active FROM accounts ORDER BY code")
	if err != nil {
		return w, err
	}
	for rows.Next() {
		var a Account
		if err = rows.Scan(&a.ID, &a.Code, &a.Name, &a.Group, &a.RequiresPartner, &a.Active); err != nil {
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
	rows, err = tx.QueryContext(ctx, "SELECT id,code,name,role,email,phone,active FROM partners ORDER BY code")
	if err != nil {
		return w, err
	}
	for rows.Next() {
		var p Partner
		if err = rows.Scan(&p.ID, &p.Code, &p.Name, &p.Role, &p.Email, &p.Phone, &p.Active); err != nil {
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
	rows, err = tx.QueryContext(ctx, "SELECT id FROM documents WHERE substr(date,1,7)=? ORDER BY date DESC,posted_order DESC,id LIMIT 100", period)
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
		if len(partners) == 0 || len(partners) > 100 {
			return nil, accounting.Fail("VALIDATION", "Mỗi lần ghi từ 1 đến 100 đối tượng.")
		}
		for _, p := range partners {
			p.Code = strings.ToUpper(strings.TrimSpace(p.Code))
			p.Name = strings.TrimSpace(p.Name)
			if !accounting.ID(p.ID) || !accounting.ID(p.Code) || p.Name == "" || len(p.Name) > 300 || len(p.Email) > 254 || len(p.Phone) > 40 || !strings.Contains("|customer|supplier|both|", "|"+p.Role+"|") {
				return nil, accounting.Fail("VALIDATION", "Thông tin đối tượng không hợp lệ.")
			}
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
				if old.Code != p.Code || count > 0 && (old.Name != p.Name || old.Role != p.Role) {
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
			if _, err = tx.ExecContext(ctx, "INSERT INTO partners VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,role=excluded.role,email=excluded.email,phone=excluded.phone,active=excluded.active", p.ID, p.Code, p.Name, p.Role, p.Email, p.Phone, p.Active); err != nil {
				return nil, err
			}
		}
		return partners, nil
	})
}
func (s *Service) SaveAccount(ctx context.Context, cmd Command, a Account) (json.RawMessage, error) {
	return s.Write(ctx, cmd, "admin.account.save", a, func(tx *sql.Tx) (any, error) {
		if !accounting.ID(a.ID) || !accounting.ID(a.Code) || strings.TrimSpace(a.Name) == "" || !strings.Contains("|cash|receivable|payable|revenue|expense|inventory|capital|asset|tax|liability|wip|", "|"+a.Group+"|") {
			return nil, accounting.Fail("VALIDATION", "Thông tin tài khoản không hợp lệ.")
		}
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
			if old.Code != a.Code || n > 0 && (old.Name != a.Name || old.Group != a.Group || old.RequiresPartner != a.RequiresPartner) {
				return nil, accounting.Fail("HISTORY_LOCKED", "Không đổi tài khoản đã có lịch sử. Có thể ngừng sử dụng.")
			}
		}
		var duplicate int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM accounts WHERE code=? AND id<>?", a.Code, a.ID).Scan(&duplicate); err != nil {
			return nil, err
		}
		if duplicate > 0 {
			return nil, accounting.Fail("DUPLICATE_CODE", "Mã tài khoản đã tồn tại.")
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO accounts VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,account_group=excluded.account_group,requires_partner=excluded.requires_partner,active=excluded.active", a.ID, a.Code, a.Name, a.Group, a.RequiresPartner, a.Active)
		return a, err
	})
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
