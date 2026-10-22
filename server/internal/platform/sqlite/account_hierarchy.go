package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"oanh.local/accounting/internal/accounting"
)

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
