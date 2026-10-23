package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"strings"
)

type Warehouse struct {
	HasReferences bool   `json:"hasReferences,omitempty"`
	ID            string `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Active        bool   `json:"active"`
}
type WarehouseStock struct {
	WarehouseID string `json:"warehouseId"`
	ItemID      string `json:"itemId"`
	Quantity    string `json:"quantity"`
	Value       string `json:"value"`
	Scale       int    `json:"scale"`
}
type WarehouseList struct {
	ledger.Context
	Warehouses []Warehouse      `json:"warehouses"`
	Stock      []WarehouseStock `json:"stock"`
}

func (s *Service) SaveWarehouse(ctx context.Context, cmd ledger.Command, w Warehouse) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "warehouse.save", w, func(tx *sql.Tx) (any, error) {
		return s.saveWarehouseTx(ctx, tx, w)
	})
}
func (s *Service) saveWarehouseTx(ctx context.Context, tx *sql.Tx, w Warehouse) (any, error) {
	if !accounting.ID(w.ID) || !accounting.ID(w.Code) || strings.TrimSpace(w.Name) == "" || len(w.Name) > 200 {
		return nil, accounting.Fail("VALIDATION", "Nhập mã và tên kho hợp lệ.")
	}
	var old Warehouse
	e := tx.QueryRowContext(ctx, "SELECT code,name FROM warehouses WHERE id=?", w.ID).Scan(&old.Code, &old.Name)
	if e != nil && e != sql.ErrNoRows {
		return nil, e
	}
	w.HasReferences, e = catalogHasReferences(ctx, tx, warehouseReferences, w.ID)
	if e != nil {
		return nil, e
	}
	if w.HasReferences && old.Code != w.Code {
		return nil, accounting.Fail("HISTORY_LOCKED", "Kho đã có chứng từ hoặc số dư: giữ mã kho. Bạn vẫn sửa được tên và trạng thái sử dụng.")
	}
	var count int
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM warehouses WHERE code=? COLLATE NOCASE AND id<>?", w.Code, w.ID).Scan(&count); e != nil {
		return nil, e
	}
	if count > 0 {
		return nil, accounting.Fail("DUPLICATE_CODE", "Mã kho đã tồn tại.")
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO warehouses VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET code=excluded.code,name=excluded.name,active=excluded.active", w.ID, w.Code, w.Name, w.Active)
	return w, e
}
func (s *Service) Warehouses(ctx context.Context) (WarehouseList, error) {
	result := WarehouseList{Warehouses: []Warehouse{}, Stock: []WarehouseStock{}}
	tx, e := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return result, e
	}
	defer tx.Rollback()
	result.Context, e = ledger.ReadContext(ctx, tx)
	if e != nil {
		return result, e
	}
	rows, e := tx.QueryContext(ctx, "SELECT id,code,name,active FROM warehouses ORDER BY code")
	if e != nil {
		return result, e
	}
	for rows.Next() {
		var w Warehouse
		if e = rows.Scan(&w.ID, &w.Code, &w.Name, &w.Active); e != nil {
			rows.Close()
			return result, e
		}
		result.Warehouses = append(result.Warehouses, w)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return result, e
	}
	usage, e := readCatalogReferences(ctx, tx, warehouseReferences)
	if e != nil {
		return result, e
	}
	for index := range result.Warehouses {
		result.Warehouses[index].HasReferences = usage[result.Warehouses[index].ID]
	}
	rows, e = tx.QueryContext(ctx, "SELECT w.warehouse_id,l.item_id,l.quantity,l.value,l.scale FROM stock_lots l JOIN stock_lot_warehouses w ON w.lot_id=l.id ORDER BY w.warehouse_id,l.item_id")
	if e != nil {
		return result, e
	}
	defer rows.Close()
	type key struct {
		warehouse, item string
		scale           int
	}
	byKey := map[key]int{}
	for rows.Next() {
		var warehouse, item string
		var qty, value int64
		var scale int
		if e = rows.Scan(&warehouse, &item, &qty, &value, &scale); e != nil {
			return result, e
		}
		k := key{warehouse, item, scale}
		i, ok := byKey[k]
		if !ok {
			i = len(result.Stock)
			byKey[k] = i
			result.Stock = append(result.Stock, WarehouseStock{WarehouseID: warehouse, ItemID: item, Quantity: "0", Value: "0", Scale: scale})
		}
		stock := &result.Stock[i]
		oldQty, _ := accounting.Money(stock.Quantity)
		oldValue, _ := accounting.Money(stock.Value)
		newQty, err := accounting.Add(oldQty, qty)
		if err != nil {
			return result, err
		}
		newValue, err := accounting.Add(oldValue, value)
		if err != nil {
			return result, err
		}
		stock.Quantity = accounting.Text(newQty)
		stock.Value = accounting.Text(newValue)
	}
	return result, rows.Err()
}
func insertStockMovement(ctx context.Context, tx *sql.Tx, item string, doc *ledger.Document, qty, value int64, scale int, warehouse string) error {
	result, e := tx.ExecContext(ctx, "INSERT INTO stock_movements(item_id,document_id,date,quantity,value,scale) VALUES(?,?,?,?,?,?)", item, doc.ID, doc.Date, qty, value, scale)
	if e != nil {
		return e
	}
	id, e := result.LastInsertId()
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO stock_movement_warehouses VALUES(?,?)", id, warehouse)
	return e
}
