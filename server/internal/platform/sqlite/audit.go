package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type auditTable struct{ name, keys, columns string }

// This schema is migration 8; extending the capture list requires a new migration.
var auditTables = []auditTable{
	{"excel_imports", "id", "id actor_id epoch owner_only filename file_hash state configuration fingerprint result created_at imported_at"},
	{"users", "id", "id username name role active credential_changed"},
	{"accounts", "id", "id code name account_group requires_partner active"},
	{"partners", "id", "id code name role email phone active"},
	{"periods", "period", "period closed"},
	{"documents", "id", "id reference type date description partner_id status version source_type source_id reversal_of policy_id posted_at posted_order"},
	{"document_lines", "document_id position", "document_id position account_id partner_id partner_name account_name description debit credit"},
	{"voucher_inputs", "document_id", "document_id payload"},
	{"bank_accounts", "id", "id number bank_name account_id active"},
	{"bank_movements", "document_id position", "document_id position bank_id debit credit"},
	{"warehouses", "id", "id code name active"},
	{"items", "id", "id code name unit tracked active"},
	{"employees", "id", "id code name base_salary insurance_base dependents active"},
	{"policies", "id", "id version effective_from data"},
	{"operations", "id", "id kind source_id date document_id policy_id payload calculation"},
	{"assets", "id", "id code name kind cost residual months start_period recognized_document allocated"},
	{"stock_lots", "id", "id item_id document_id date quantity value scale"},
	{"stock_movements", "id", "id item_id document_id date quantity value scale"},
	{"financial_reports", "id", "id version from_date to_date updated_at actor_id data"},
	{"opening_balances", "id", "id document_id cutoff source payload"},
	{"opening_adjustments", "document_id", "document_id opening_document_id before_document_id reversal_document_id base_version confirmed_version reason before_payload after_payload created_by created_at confirmed_by confirmed_at"},
	{"account_hierarchy", "account_id revision", "account_id revision parent_id"},
	{"attachments", "id", "id document_id hash name size"},
}

var auditSchema = makeAuditSchema()

func auditJSON(spec auditTable, prefix string) string {
	parts := []string{}
	for _, column := range strings.Fields(spec.columns) {
		value := "CAST(" + prefix + column + " AS TEXT)"
		if spec.name == "users" && column == "credential_changed" {
			value = "'0'"
		}
		parts = append(parts, "'"+column+"'", value)
	}
	return "json_object(" + strings.Join(parts, ",") + ")"
}
func auditKey(spec auditTable, prefix string) string {
	parts := []string{}
	for _, column := range strings.Fields(spec.keys) {
		parts = append(parts, "CAST("+prefix+column+" AS TEXT)")
	}
	return "json_array(" + strings.Join(parts, ",") + ")"
}
func makeAuditSchema() string {
	var sql strings.Builder
	sql.WriteString(`
CREATE TABLE audit_context(id INTEGER PRIMARY KEY CHECK(id=1),audit_id INTEGER NOT NULL,actor_name TEXT NOT NULL,reason TEXT NOT NULL DEFAULT '',restored_from INTEGER,base_action TEXT NOT NULL DEFAULT '') STRICT;
CREATE TABLE audit_details(audit_id INTEGER PRIMARY KEY REFERENCES audit(id),actor_name TEXT NOT NULL,reason TEXT NOT NULL,restored_from INTEGER REFERENCES audit(id),base_action TEXT NOT NULL,title TEXT NOT NULL,entity_type TEXT NOT NULL,entity_id TEXT NOT NULL,entity_name TEXT NOT NULL,change_count INTEGER NOT NULL CHECK(change_count>=0),search_text TEXT NOT NULL) STRICT;
CREATE TABLE audit_changes(audit_id INTEGER NOT NULL REFERENCES audit(id) DEFERRABLE INITIALLY DEFERRED,table_name TEXT NOT NULL,row_key TEXT NOT NULL CHECK(json_valid(row_key)),before_state TEXT CHECK(before_state IS NULL OR json_valid(before_state)),after_state TEXT CHECK(after_state IS NULL OR json_valid(after_state)),PRIMARY KEY(audit_id,table_name,row_key)) STRICT;
CREATE TABLE audit_recovery_versions(table_name TEXT NOT NULL CHECK(table_name IN ('documents','financial_reports')),entity_id TEXT NOT NULL,version INTEGER NOT NULL CHECK(version>0),PRIMARY KEY(table_name,entity_id)) STRICT;
CREATE TRIGGER audit_document_version_floor BEFORE DELETE ON documents WHEN OLD.status='draft' BEGIN INSERT INTO audit_recovery_versions VALUES('documents',OLD.id,OLD.version) ON CONFLICT(table_name,entity_id) DO UPDATE SET version=MAX(version,excluded.version); END;
CREATE TRIGGER audit_financial_version_floor BEFORE DELETE ON financial_reports BEGIN INSERT INTO audit_recovery_versions VALUES('financial_reports',OLD.id,OLD.version) ON CONFLICT(table_name,entity_id) DO UPDATE SET version=MAX(version,excluded.version); END;
CREATE INDEX audit_changes_entity ON audit_changes(table_name,row_key,audit_id);
CREATE INDEX audit_restored_from ON audit_details(restored_from,audit_id);
CREATE INDEX audit_occurred_at ON audit(occurred_at,id);
CREATE INDEX audit_actor_time ON audit(actor_id,id);
CREATE INDEX audit_action_time ON audit(action,id);
CREATE TRIGGER audit_no_update BEFORE UPDATE ON audit BEGIN SELECT RAISE(ABORT,'audit is immutable'); END;
CREATE TRIGGER audit_no_delete BEFORE DELETE ON audit BEGIN SELECT RAISE(ABORT,'audit is immutable'); END;
CREATE TRIGGER audit_details_no_update BEFORE UPDATE ON audit_details BEGIN SELECT RAISE(ABORT,'audit details are immutable'); END;
CREATE TRIGGER audit_details_no_delete BEFORE DELETE ON audit_details BEGIN SELECT RAISE(ABORT,'audit details are immutable'); END;
CREATE TRIGGER audit_changes_update BEFORE UPDATE ON audit_changes WHEN NOT EXISTS(SELECT 1 FROM audit_context WHERE audit_id=OLD.audit_id AND audit_id=NEW.audit_id) BEGIN SELECT RAISE(ABORT,'audit changes are immutable'); END;
CREATE TRIGGER audit_changes_delete BEFORE DELETE ON audit_changes WHEN NOT EXISTS(SELECT 1 FROM audit_context WHERE audit_id=OLD.audit_id) BEGIN SELECT RAISE(ABORT,'audit changes are immutable'); END;
CREATE TRIGGER audit_changes_insert BEFORE INSERT ON audit_changes WHEN NOT EXISTS(SELECT 1 FROM audit_context WHERE audit_id=NEW.audit_id) BEGIN SELECT RAISE(ABORT,'audit changes require a transaction context'); END;
`)
	for _, spec := range auditTables {
		for _, operation := range []string{"INSERT", "UPDATE", "DELETE"} {
			before, after, key := "NULL", "NULL", auditKey(spec, "NEW.")
			if operation != "INSERT" {
				before = auditJSON(spec, "OLD.")
			}
			if operation != "DELETE" {
				after = auditJSON(spec, "NEW.")
			} else {
				key = auditKey(spec, "OLD.")
			}
			if spec.name == "users" {
				if operation == "INSERT" {
					after = "json_set(" + after + ",'$.credential_changed','1')"
				}
				if operation == "UPDATE" {
					after = "json_set(" + after + ",'$.credential_changed',CASE WHEN OLD.password_hash IS NOT NEW.password_hash THEN '1' ELSE '0' END)"
				}
			}
			condition := "EXISTS(SELECT 1 FROM audit_context)"
			if operation == "UPDATE" {
				condition += " AND " + before + " IS NOT " + after
			}
			fmt.Fprintf(&sql, "CREATE TRIGGER audit_capture_%s_%s AFTER %s ON %s WHEN %s BEGIN INSERT INTO audit_changes(audit_id,table_name,row_key,before_state,after_state) SELECT audit_id,'%s',%s,%s,%s FROM audit_context WHERE id=1 ON CONFLICT(audit_id,table_name,row_key) DO UPDATE SET after_state=excluded.after_state; END;\n", spec.name, strings.ToLower(operation), operation, spec.name, condition, spec.name, key, before, after)
		}
	}
	return sql.String()
}

func BeginAudit(ctx context.Context, tx *sql.Tx, actor string) (int64, error) {
	var maximum int64
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(id),0) FROM audit").Scan(&maximum); err != nil {
		return 0, err
	}
	if maximum == math.MaxInt64 {
		return 0, fmt.Errorf("audit identifier exhausted")
	}
	id := maximum + 1
	_, err := tx.ExecContext(ctx, "INSERT INTO audit_context(id,audit_id,actor_name) SELECT 1,?,name FROM users WHERE id=?", id, actor)
	return id, err
}

type AuditMetadata struct {
	Title, EntityType, EntityID, EntityName, Search string
	Count                                           int
}

func FinishAudit(ctx context.Context, tx *sql.Tx, id int64, actor, operation, entity string, metadata AuditMetadata) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM audit_changes WHERE audit_id=? AND before_state IS after_state", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit(id,actor_id,action,entity_id,occurred_at,revision) SELECT ?,?,?,?,?,revision FROM meta", id, actor, operation, entity, Now()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit_details SELECT audit_id,actor_name,reason,restored_from,CASE WHEN base_action='' THEN ? ELSE base_action END,?,?,?,?,?,? FROM audit_context WHERE id=1 AND audit_id=?", operation, metadata.Title, metadata.EntityType, metadata.EntityID, metadata.EntityName, metadata.Count, metadata.Search, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM audit_context WHERE id=1 AND audit_id=?", id)
	return err
}

type AuditChange struct {
	Table  string          `json:"table"`
	Key    string          `json:"key"`
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
}

func ReadAuditChanges(ctx context.Context, tx *sql.Tx, id int64) ([]AuditChange, error) {
	rows, err := tx.QueryContext(ctx, "SELECT table_name,row_key,before_state,after_state FROM audit_changes WHERE audit_id=? AND before_state IS NOT after_state ORDER BY table_name,row_key LIMIT 20001", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []AuditChange{}
	for rows.Next() {
		var item AuditChange
		var before, after sql.NullString
		if err = rows.Scan(&item.Table, &item.Key, &before, &after); err != nil {
			return nil, err
		}
		if before.Valid {
			item.Before = json.RawMessage(before.String)
		}
		if after.Valid {
			item.After = json.RawMessage(after.String)
		}
		if err = validateAuditChange(item); err != nil {
			return nil, err
		}
		values = append(values, item)
	}
	if len(values) > 20000 {
		return nil, fmt.Errorf("audit change set exceeds display limit")
	}
	return values, rows.Err()
}
func AuditRow(ctx context.Context, tx *sql.Tx, table, key string) (json.RawMessage, error) {
	for _, spec := range auditTables {
		if spec.name == table {
			var keys []string
			if err := json.Unmarshal([]byte(key), &keys); err != nil {
				return nil, err
			}
			columns := strings.Fields(spec.keys)
			if len(columns) != len(keys) {
				return nil, fmt.Errorf("invalid audit key")
			}
			where := []string{}
			args := []any{}
			for i, column := range columns {
				where = append(where, column+"=?")
				args = append(args, keys[i])
			}
			var value string
			err := tx.QueryRowContext(ctx, "SELECT "+auditJSON(spec, "")+" FROM "+spec.name+" WHERE "+strings.Join(where, " AND "), args...).Scan(&value)
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return json.RawMessage(value), err
		}
	}
	return nil, fmt.Errorf("unsupported audit table %s", strconv.Quote(table))
}
func verifyAudit(ctx context.Context, db *sql.DB) error {
	var version, count int
	if err := db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version < 8 {
		return nil
	}
	for _, query := range []string{
		"SELECT count(*) FROM audit_context",
		"SELECT count(*) FROM audit_details d WHERE d.change_count<>(SELECT count(*) FROM audit_changes c WHERE c.audit_id=d.audit_id)",
		"SELECT count(*) FROM audit_recovery_versions r LEFT JOIN documents d ON r.table_name='documents' AND d.id=r.entity_id LEFT JOIN financial_reports f ON r.table_name='financial_reports' AND f.id=r.entity_id WHERE (d.id IS NOT NULL AND d.version<=r.version) OR (f.id IS NOT NULL AND f.version<=r.version)",
		"SELECT count(*) FROM audit_details d JOIN audit a ON a.id=d.audit_id WHERE (a.action='admin.audit.restore' AND (d.restored_from IS NULL OR d.restored_from>=d.audit_id OR NOT EXISTS(SELECT 1 FROM audit_details p WHERE p.audit_id=d.restored_from AND p.base_action=d.base_action))) OR (a.action<>'admin.audit.restore' AND (d.restored_from IS NOT NULL OR d.base_action<>a.action))",
	} {
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("audit history or recovery versions are inconsistent")
		}
	}
	return nil
}

func RecoveryVersion(ctx context.Context, tx *sql.Tx, table, id string) (int, error) {
	var version int
	err := tx.QueryRowContext(ctx, "SELECT version FROM audit_recovery_versions WHERE table_name=? AND entity_id=?", table, id).Scan(&version)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if version == math.MaxInt64 {
		return 0, fmt.Errorf("recovery version exhausted")
	}
	return version, err
}

func validateAuditChange(change AuditChange) error {
	for _, spec := range auditTables {
		if spec.name == change.Table {
			var keys []string
			if err := json.Unmarshal([]byte(change.Key), &keys); err != nil {
				return err
			}
			primary := strings.Fields(spec.keys)
			if len(keys) != len(primary) {
				return fmt.Errorf("invalid audit primary key")
			}
			for _, raw := range []json.RawMessage{change.Before, change.After} {
				if len(raw) == 0 {
					continue
				}
				var state map[string]*string
				if err := json.Unmarshal(raw, &state); err != nil {
					return err
				}
				columns := strings.Fields(spec.columns)
				if len(state) != len(columns) {
					return fmt.Errorf("incomplete audit state")
				}
				for _, column := range columns {
					if _, ok := state[column]; !ok {
						return fmt.Errorf("missing audit field")
					}
				}
				for i, column := range primary {
					if state[column] == nil || *state[column] != keys[i] {
						return fmt.Errorf("audit row identity mismatch")
					}
				}
			}
			return nil
		}
	}
	return fmt.Errorf("unsupported audit change table")
}
