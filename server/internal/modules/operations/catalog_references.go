package operations

import (
	"context"
	"database/sql"

	"oanh.local/accounting/internal/modules/ledger"
)

const itemReferences = `SELECT item_id AS id FROM stock_movements
 UNION ALL SELECT item_id FROM stock_lots
 UNION ALL SELECT json_extract(l.value,'$.itemId') FROM operations o,json_each(o.payload,'$.lines') l
 UNION ALL SELECT json_extract(l.value,'$.itemId') FROM voucher_inputs v,json_each(v.payload,'$.lines') l
 UNION ALL SELECT json_extract(l.value,'$.itemId') FROM opening_balances o,json_each(o.payload,'$.stocks') l
 UNION ALL SELECT json_extract(l.value,'$.itemId') FROM opening_adjustments o,json_each(o.after_payload,'$.stocks') l`

const bankReferences = `SELECT bank_id AS id FROM bank_movements
 UNION ALL SELECT json_extract(payload,'$.bankAccountId') FROM operations
 UNION ALL SELECT json_extract(payload,'$.destinationBankAccountId') FROM operations
 UNION ALL SELECT json_extract(payload,'$.bankAccountId') FROM voucher_inputs
 UNION ALL SELECT json_extract(payload,'$.destinationBankAccountId') FROM voucher_inputs
 UNION ALL SELECT json_extract(l.value,'$.bankAccountId') FROM opening_balances o,json_each(o.payload,'$.lines') l
 UNION ALL SELECT json_extract(l.value,'$.bankAccountId') FROM opening_adjustments o,json_each(o.after_payload,'$.lines') l`

const warehouseReferences = `SELECT warehouse_id AS id FROM stock_movement_warehouses
 UNION ALL SELECT warehouse_id FROM stock_lot_warehouses
 UNION ALL SELECT COALESCE(NULLIF(json_extract(l.value,'$.warehouseId'),''),'default') FROM operations o,json_each(o.payload,'$.lines') l JOIN items i ON i.id=json_extract(l.value,'$.itemId') WHERE i.tracked=1
 UNION ALL SELECT COALESCE(NULLIF(json_extract(l.value,'$.warehouseId'),''),'default') FROM voucher_inputs v,json_each(v.payload,'$.lines') l JOIN items i ON i.id=json_extract(l.value,'$.itemId') WHERE i.tracked=1
 UNION ALL SELECT COALESCE(NULLIF(json_extract(l.value,'$.warehouseId'),''),'default') FROM opening_balances o,json_each(o.payload,'$.stocks') l
 UNION ALL SELECT COALESCE(NULLIF(json_extract(l.value,'$.warehouseId'),''),'default') FROM opening_adjustments o,json_each(o.after_payload,'$.stocks') l`

const employeeReferences = `SELECT json_extract(payload,'$.employeeId') AS id FROM operations
 UNION ALL SELECT json_extract(payload,'$.employeeId') FROM voucher_inputs`

func catalogHasReferences(ctx context.Context, q ledger.Queryer, source, id string) (bool, error) {
	var used bool
	err := q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM ("+source+") WHERE id=?)", id).Scan(&used)
	return used, err
}

func readCatalogReferences(ctx context.Context, q ledger.Queryer, source string) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	used := map[string]bool{}
	for rows.Next() {
		var id sql.NullString
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		if id.Valid && id.String != "" {
			used[id.String] = true
		}
	}
	return used, rows.Err()
}
