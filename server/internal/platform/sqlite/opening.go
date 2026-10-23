package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"oanh.local/accounting/internal/accounting"
)

func verifyOpening(ctx context.Context, db *sql.DB) error {
	var exists int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name='opening_balances'").Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	var corrected int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name='opening_adjustments'").Scan(&corrected); err != nil {
		return err
	}
	if corrected > 0 {
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM opening_adjustments WHERE confirmed_version IS NOT NULL").Scan(&corrected); err != nil {
			return err
		}
	}
	var id, date, status, source, raw string
	err := db.QueryRowContext(ctx, "SELECT d.id,d.date,d.status,d.source_type,o.payload FROM opening_balances o JOIN documents d ON d.id=o.document_id WHERE o.cutoff=d.date AND o.source=json_extract(o.payload,'$.source')").Scan(&id, &date, &status, &source, &raw)
	if err == sql.ErrNoRows {
		if err = db.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM opening_balances)+(SELECT count(*) FROM documents WHERE source_type='opening')+(SELECT count(*) FROM opening_assets)").Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			return fmt.Errorf("opening metadata missing or inconsistent")
		}
		return nil
	}
	if err != nil {
		return err
	}
	var input struct {
		ID     string `json:"id"`
		Cutoff string `json:"cutoff"`
		Lines  []struct {
			AccountID string `json:"accountId"`
			PartnerID string `json:"partnerId"`
			Debit     string `json:"debit"`
			Credit    string `json:"credit"`
		} `json:"lines"`
		Stocks []struct {
			ItemID      string `json:"itemId"`
			WarehouseID string `json:"warehouseId"`
			Quantity    string `json:"quantity"`
			Value       string `json:"value"`
		} `json:"stocks"`
		Assets []struct {
			ID          string `json:"id"`
			Code        string `json:"code"`
			Name        string `json:"name"`
			Residual    string `json:"residual"`
			Months      int    `json:"months"`
			Kind        string `json:"kind"`
			Cost        string `json:"cost"`
			Allocated   string `json:"allocated"`
			StartPeriod string `json:"startPeriod"`
		} `json:"assets"`
	}
	if err = json.Unmarshal([]byte(raw), &input); err != nil {
		return err
	}
	if input.ID != id || input.Cutoff != date || source != "opening" || !accounting.Date(date) || date[8:] != "01" {
		return fmt.Errorf("invalid opening source")
	}
	if status == "posted" {
		if err = db.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE status='posted' AND id<>? AND (date<? OR posted_order<=(SELECT posted_order FROM documents WHERE id=?))", id, date, id).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			return fmt.Errorf("history overlaps opening conversion")
		}
	}
	expected, actual := map[string]*big.Int{}, map[string]*big.Int{}
	add := func(target map[string]*big.Int, account, partner string, d, c int64) {
		for i, n := range []int64{d, c} {
			key := fmt.Sprintf("%s|%s|%d", account, partner, i)
			if target[key] == nil {
				target[key] = new(big.Int)
			}
			target[key].Add(target[key], big.NewInt(n))
		}
	}
	money := func(text string) (int64, error) { return accounting.Money(text) }
	for _, l := range input.Lines {
		d, e := money(l.Debit)
		if e != nil {
			return e
		}
		c, e := money(l.Credit)
		if e != nil {
			return e
		}
		add(expected, l.AccountID, l.PartnerID, d, c)
	}
	for _, stock := range input.Stocks {
		n, e := money(stock.Value)
		if e != nil {
			return e
		}
		if n > 0 {
			add(expected, "156", "", n, 0)
		}
	}
	for _, a := range input.Assets {
		cost, e := money(a.Cost)
		if e != nil {
			return e
		}
		allocated, e := money(a.Allocated)
		if e != nil {
			return e
		}
		if a.Kind == "fixed" {
			add(expected, "211", "", cost, 0)
			if allocated > 0 {
				add(expected, "214", "", 0, allocated)
			}
		} else if cost > allocated {
			add(expected, "242", "", cost-allocated, 0)
		}
		if status == "posted" {
			residual, e := money(a.Residual)
			if e != nil {
				return e
			}
			var elapsed int
			var initial int64
			var recognized string
			query := "SELECT o.elapsed,o.allocated,a.recognized_document FROM opening_assets o JOIN assets a ON a.id=o.asset_id WHERE o.asset_id=?"
			args := []any{a.ID}
			if corrected == 0 {
				query += " AND a.cost=? AND a.start_period=? AND a.kind=? AND a.code=? AND a.name=? AND a.residual=? AND a.months=?"
				args = append(args, cost, a.StartPeriod, a.Kind, a.Code, a.Name, residual, a.Months)
			}
			if e = db.QueryRowContext(ctx, query, args...).Scan(&elapsed, &initial, &recognized); e != nil {
				return e
			}
			start, e := time.Parse("2006-01", a.StartPeriod)
			if e != nil {
				return e
			}
			cutoff, _ := time.Parse("2006-01-02", date)
			if recognized != id || initial != allocated || elapsed != (cutoff.Year()-start.Year())*12+int(cutoff.Month()-start.Month()) {
				return fmt.Errorf("opening asset differs from source")
			}
		}
	}
	rows, err := db.QueryContext(ctx, "SELECT account_id,COALESCE(partner_id,''),debit,credit FROM document_lines WHERE document_id=?", id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a, p string
		var d, c int64
		if err = rows.Scan(&a, &p, &d, &c); err != nil {
			rows.Close()
			return err
		}
		add(actual, a, p, d, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("opening lines differ from source")
	}
	for key, n := range expected {
		if actual[key] == nil || n.Cmp(actual[key]) != 0 {
			return fmt.Errorf("opening ledger reconciliation mismatch")
		}
	}
	if status == "posted" {
		var assetCount, openingCount int
		if err = db.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM assets WHERE recognized_document=?),(SELECT count(*) FROM opening_assets)", id).Scan(&assetCount, &openingCount); err != nil {
			return err
		}
		if assetCount != len(input.Assets) || openingCount != len(input.Assets) {
			return fmt.Errorf("opening asset count differs from source")
		}
		var scale int
		if len(input.Stocks) > 0 {
			if err = db.QueryRowContext(ctx, "SELECT json_extract(p.data,'$.quantityScale') FROM documents d JOIN policies p ON p.id=d.policy_id WHERE d.id=?", id).Scan(&scale); err != nil {
				return err
			}
		}
		want, got := map[string]*big.Int{}, map[string]*big.Int{}
		for _, stock := range input.Stocks {
			q, e := accounting.Quantity(stock.Quantity, scale)
			if e != nil {
				return e
			}
			v, e := money(stock.Value)
			if e != nil {
				return e
			}
			add(want, stock.ItemID, stock.WarehouseID, q, v)
		}
		rows, err = db.QueryContext(ctx, "SELECT m.item_id,w.warehouse_id,m.quantity,m.value FROM stock_movements m JOIN stock_movement_warehouses w ON w.movement_id=m.id WHERE m.document_id=?", id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var item, warehouse string
			var q, v int64
			if err = rows.Scan(&item, &warehouse, &q, &v); err != nil {
				rows.Close()
				return err
			}
			add(got, item, warehouse, q, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(got) != len(want) {
			return fmt.Errorf("opening stock differs from source")
		}
		for key, n := range want {
			if got[key] == nil || n.Cmp(got[key]) != 0 {
				return fmt.Errorf("opening stock reconciliation mismatch")
			}
		}
	}
	return nil
}
