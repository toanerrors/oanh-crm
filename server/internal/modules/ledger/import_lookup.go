package ledger

import (
	"context"
	"database/sql"
	"oanh.local/accounting/internal/accounting"
	"strconv"
)

func (s *Service) ResolveImportCodeTx(ctx context.Context, tx *sql.Tx, kind, code string) (string, error) {
	if code == "" {
		return "", nil
	}
	query := ""
	switch kind {
	case "account":
		query = "SELECT id FROM accounts WHERE code=? AND active=1"
	case "partner":
		query = "SELECT id FROM partners WHERE code=? COLLATE NOCASE AND active=1"
	default:
		return "", accounting.Fail("VALIDATION", "Loại mã tham chiếu không hợp lệ.")
	}
	var id string
	err := tx.QueryRowContext(ctx, query, code).Scan(&id)
	if err == sql.ErrNoRows {
		return "", accounting.Fail("INVALID_REFERENCE", "Mã tài khoản hoặc đối tượng không tồn tại/đã ngừng sử dụng.")
	}
	return id, err
}

func (s *Service) ImportCatalogIDTx(ctx context.Context, tx *sql.Tx, kind, code string) (string, error) {
	query := ""
	switch kind {
	case "accounts":
		query = "SELECT id FROM accounts WHERE code=?"
	case "partners":
		query = "SELECT id FROM partners WHERE code=? COLLATE NOCASE"
	default:
		return "", accounting.Fail("VALIDATION", "Loại danh mục không hợp lệ.")
	}
	var id string
	err := tx.QueryRowContext(ctx, query, code).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (s *Service) ResolveImportDocumentTx(ctx context.Context, tx *sql.Tx, kind, source string) (string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM documents WHERE source_type=? AND (source_id=? OR reference=?) AND status='posted' LIMIT 2", kind, source, source)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	if len(ids) != 1 {
		return "", accounting.Fail("INVALID_SOURCE", "Không tìm thấy duy nhất chứng từ nguồn đã ghi sổ theo mã nguồn/số chứng từ.")
	}
	return ids[0], nil
}

func (s *Service) ImportCatalogOptionalTx(ctx context.Context, tx *sql.Tx, kind, id string) (map[string]string, error) {
	values := map[string]string{}
	var active bool
	switch kind {
	case "accounts":
		if err := tx.QueryRowContext(ctx, "SELECT active FROM accounts WHERE id=?", id).Scan(&active); err != nil {
			return nil, err
		}
	case "partners":
		var email, phone string
		if err := tx.QueryRowContext(ctx, "SELECT email,phone,active FROM partners WHERE id=?", id).Scan(&email, &phone, &active); err != nil {
			return nil, err
		}
		values["email"], values["phone"] = email, phone
	default:
		return nil, accounting.Fail("VALIDATION", "Loại danh mục không hợp lệ.")
	}
	values["active"] = strconv.FormatBool(active)
	return values, nil
}
