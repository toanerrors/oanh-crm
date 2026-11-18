package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"oanh.local/accounting/internal/accounting"
)

var initialAccounts = [][4]string{
	{"821", "Chi phí thuế thu nhập doanh nghiệp", "expense", "0"}, {"3334", "Thuế thu nhập doanh nghiệp", "tax", "0"},
	{"111", "Tiền mặt", "cash", "0"}, {"1121", "Tiền gửi ngân hàng", "cash", "0"}, {"131", "Phải thu của khách hàng", "receivable", "1"}, {"1331", "Thuế GTGT được khấu trừ", "tax", "0"}, {"156", "Hàng hóa", "inventory", "0"}, {"154", "Chi phí sản xuất kinh doanh dở dang", "wip", "0"}, {"211", "Tài sản cố định hữu hình", "asset", "0"}, {"214", "Hao mòn tài sản cố định", "asset", "0"}, {"242", "Chi phí trả trước", "asset", "0"}, {"331", "Phải trả cho người bán", "payable", "1"}, {"33311", "Thuế GTGT phải nộp", "tax", "0"}, {"3335", "Thuế thu nhập cá nhân", "tax", "0"}, {"334", "Phải trả người lao động", "liability", "0"}, {"338", "Phải trả bảo hiểm", "liability", "0"}, {"411", "Vốn đầu tư của chủ sở hữu", "capital", "0"}, {"421", "Lợi nhuận sau thuế chưa phân phối", "capital", "0"}, {"511", "Doanh thu bán hàng và cung cấp dịch vụ", "revenue", "0"}, {"632", "Giá vốn hàng bán", "expense", "0"}, {"642", "Chi phí quản lý doanh nghiệp", "expense", "0"}, {"911", "Xác định kết quả kinh doanh", "capital", "0"},
}

func SeedInitialAccounts(ctx context.Context, tx *sql.Tx) error {
	for _, a := range initialAccounts {
		if _, err := tx.ExecContext(ctx, "INSERT INTO accounts VALUES(?,?,?,?,?,1) ON CONFLICT(code) DO NOTHING", a[0], a[0], a[1], a[2], a[3] == "1"); err != nil {
			return err
		}
	}
	return SeedAccountHierarchy(ctx, tx, 0)
}

func SeedAccountHierarchy(ctx context.Context, tx *sql.Tx, revision int64) error {
	parents := map[string]string{"1121": "112"}
	for _, a := range accounting.ReferenceChart().Accounts {
		if a.ParentCode != "" {
			parents[a.Code] = a.ParentCode
		}
	}
	for code, parent := range parents {
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_hierarchy(account_id,revision,parent_id)
   SELECT c.id,?,p.id FROM accounts c JOIN accounts p ON p.code=? WHERE c.code=?
   AND NOT EXISTS(SELECT 1 FROM account_hierarchy h WHERE h.account_id=c.id)`, revision, parent, code); err != nil {
			return err
		}
	}
	return nil
}

func verifyAccountHierarchy(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version < 6 {
		return nil
	}
	rows, err := db.QueryContext(ctx, "SELECT account_id,COALESCE(parent_id,''),revision FROM account_current_parents")
	if err != nil {
		return err
	}
	parents := map[string]string{}
	var maximum int64
	for rows.Next() {
		var id, parent string
		var revision int64
		if err = rows.Scan(&id, &parent, &revision); err != nil {
			rows.Close()
			return err
		}
		parents[id] = parent
		if revision > maximum {
			maximum = revision
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var revision int64
	if err = db.QueryRowContext(ctx, "SELECT revision FROM meta WHERE id=1").Scan(&revision); err != nil {
		return err
	}
	if maximum > revision {
		return fmt.Errorf("account hierarchy revision exceeds dataset")
	}
	for id := range parents {
		seen := map[string]bool{}
		depth := 0
		for current := id; current != ""; current = parents[current] {
			if seen[current] || depth >= 10 {
				return fmt.Errorf("account hierarchy cycle or depth limit")
			}
			seen[current] = true
			depth++
		}
	}
	return nil
}
