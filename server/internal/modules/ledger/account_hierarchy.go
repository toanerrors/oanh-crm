package ledger

import (
	"context"
	"database/sql"
	"oanh.local/accounting/internal/accounting"
)

func (s *Service) AccountParentTx(ctx context.Context, tx *sql.Tx, id string) (string, int64, error) {
	var parent string
	var revision int64
	err := tx.QueryRowContext(ctx, "SELECT COALESCE(parent_id,''),revision FROM account_current_parents WHERE account_id=?", id).Scan(&parent, &revision)
	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	return parent, revision, err
}

func (s *Service) validateAccountParentTx(ctx context.Context, tx *sql.Tx, id, parent string) error {
	if parent == id {
		return accounting.Fail("INVALID_PARENT", "Tài khoản không thể là tài khoản cha của chính nó.")
	}
	if parent == "" {
		return nil
	}
	var active bool
	err := tx.QueryRowContext(ctx, "SELECT active FROM accounts WHERE id=?", parent).Scan(&active)
	if err == sql.ErrNoRows || err == nil && !active {
		return accounting.Fail("INVALID_PARENT", "Chọn tài khoản cha đang sử dụng.")
	}
	if err != nil {
		return err
	}
	seen := map[string]bool{id: true}
	depth := 1
	for current := parent; current != ""; depth++ {
		if seen[current] {
			return accounting.Fail("INVALID_PARENT", "Quan hệ cha–con tạo thành vòng lặp.")
		}
		seen[current] = true
		next, _, err := s.AccountParentTx(ctx, tx, current)
		if err != nil {
			return err
		}
		current = next
	}
	var height int
	if err = tx.QueryRowContext(ctx, `WITH RECURSIVE branch(id,depth) AS (SELECT ?,1 UNION ALL SELECT p.account_id,b.depth+1 FROM account_current_parents p JOIN branch b ON p.parent_id=b.id) SELECT MAX(depth) FROM branch`, id).Scan(&height); err != nil {
		return err
	}
	if depth+height-1 > 10 {
		return accounting.Fail("INVALID_PARENT", "Hệ thống tài khoản tối đa 10 cấp.")
	}
	return nil
}
