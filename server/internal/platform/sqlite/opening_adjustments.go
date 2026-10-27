package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"oanh.local/accounting/internal/accounting"
)

type openingSnapshot struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Cutoff  string `json:"cutoff"`
	Source  string `json:"source"`
	Lines   []struct {
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
		Kind        string `json:"kind"`
		Cost        string `json:"cost"`
		Residual    string `json:"residual"`
		Allocated   string `json:"allocated"`
		Months      int    `json:"months"`
		StartPeriod string `json:"startPeriod"`
	} `json:"assets"`
}

func verifyOpeningAdjustments(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name='opening_adjustments'").Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	var root, raw string
	err := db.QueryRowContext(ctx, "SELECT document_id,payload FROM opening_balances").Scan(&root, &raw)
	if err == sql.ErrNoRows {
		if err = db.QueryRowContext(ctx, "SELECT count(*) FROM opening_adjustments").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("opening adjustments without original")
		}
		return nil
	}
	if err != nil {
		return err
	}
	var current openingSnapshot
	if err = json.Unmarshal([]byte(raw), &current); err != nil {
		return err
	}
	type entry struct {
		id, root, before, reverse, beforeRaw, afterRaw, status, source, date, actor, at string
		version                                                                         int
		confirmed                                                                       sql.NullInt64
	}
	entries := []entry{}
	rows, err := db.QueryContext(ctx, `SELECT a.document_id,a.opening_document_id,a.before_document_id,COALESCE(a.reversal_document_id,''),a.before_payload,a.after_payload,d.status,d.source_type,d.date,a.base_version,a.confirmed_version,COALESCE(a.confirmed_by,''),COALESCE(a.confirmed_at,'') FROM opening_adjustments a JOIN documents d ON d.id=a.document_id ORDER BY a.confirmed_version IS NULL,a.confirmed_version`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.id, &e.root, &e.before, &e.reverse, &e.beforeRaw, &e.afterRaw, &e.status, &e.source, &e.date, &e.version, &e.confirmed, &e.actor, &e.at); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	version := 0
	posted := 0
	for _, e := range entries {
		var before, after openingSnapshot
		if err = json.Unmarshal([]byte(e.beforeRaw), &before); err != nil {
			return err
		}
		if err = json.Unmarshal([]byte(e.afterRaw), &after); err != nil {
			return err
		}
		var documentVersion int
		if err = db.QueryRowContext(ctx, "SELECT version FROM documents WHERE id=?", e.id).Scan(&documentVersion); err != nil {
			return err
		}
		if after.Version != documentVersion {
			return fmt.Errorf("opening adjustment snapshot version mismatch")
		}
		before.Version, current.Version = 0, 0
		// Empty and omitted detail arrays denote the same original import.
		if len(before.Stocks) == 0 {
			before.Stocks = nil
		}
		if len(current.Stocks) == 0 {
			current.Stocks = nil
		}
		if len(before.Assets) == 0 {
			before.Assets = nil
		}
		if len(current.Assets) == 0 {
			current.Assets = nil
		}
		if e.root != root || e.before != current.ID || e.version != version || !reflect.DeepEqual(before, current) || after.ID != e.id || after.Cutoff != current.Cutoff || e.date != after.Cutoff || e.source != "opening_adjustment" {
			return fmt.Errorf("opening adjustment chain differs from snapshots")
		}
		if err = verifyOpeningSnapshot(ctx, db, e.id, after); err != nil {
			return err
		}
		if !e.confirmed.Valid {
			if e.status != "draft" || e.reverse != "" || e.actor != "" || e.at != "" {
				return fmt.Errorf("invalid opening adjustment draft")
			}
			continue
		}
		if e.status != "posted" || e.confirmed.Int64 != int64(version+1) || e.actor == "" || e.at == "" {
			return fmt.Errorf("invalid opening adjustment version")
		}
		var invalid int
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM documents r WHERE r.id=? AND r.status='posted' AND r.source_type='opening_adjustment_reversal' AND r.reversal_of=? AND r.date=? AND r.posted_order<(SELECT posted_order FROM documents WHERE id=?)`, e.reverse, e.before, e.date, e.id).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 1 {
			return fmt.Errorf("opening adjustment reversal mismatch")
		}
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM document_lines l LEFT JOIN document_lines r ON r.document_id=? AND r.position=l.position WHERE l.document_id=? AND (r.position IS NULL OR r.account_id<>l.account_id OR COALESCE(r.partner_id,'')<>COALESCE(l.partner_id,'') OR r.debit<>l.credit OR r.credit<>l.debit OR r.partner_name<>l.partner_name OR r.account_name<>l.account_name OR r.description<>l.description)`, e.reverse, e.before).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("opening reversal differs from previous lines")
		}
		if err = db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM document_lines WHERE document_id=?)-(SELECT count(*) FROM document_lines WHERE document_id=?)`, e.reverse, e.before).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("opening reversal line count differs")
		}
		if err = verifyAdjustedStock(ctx, db, before, after, e.reverse, e.id); err != nil {
			return err
		}
		current = after
		version++
		posted += 2
	}
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM documents WHERE source_type IN ('opening_adjustment','opening_adjustment_reversal') AND status='posted'").Scan(&count); err != nil {
		return err
	}
	if count != posted {
		return fmt.Errorf("orphan opening adjustment documents")
	}
	if version == 0 {
		return nil
	}
	for _, a := range current.Assets {
		var code, name, kind, start string
		var cost, residual, allocated int64
		var months int
		if err = db.QueryRowContext(ctx, "SELECT code,name,kind,start_period,cost,residual,allocated,months FROM current_assets WHERE id=?", a.ID).Scan(&code, &name, &kind, &start, &cost, &residual, &allocated, &months); err != nil {
			return err
		}
		initial, err := accounting.Money(a.Allocated)
		if err != nil {
			return err
		}
		expected := big.NewInt(initial)
		r, err := db.QueryContext(ctx, "SELECT json_extract(calculation,'$.amounts.amount') FROM operations WHERE kind='depreciation' AND substr(source_id,1,length(?))=?", a.ID+":", a.ID+":")
		if err != nil {
			return err
		}
		for r.Next() {
			var text string
			if err = r.Scan(&text); err != nil {
				r.Close()
				return err
			}
			n, e := accounting.Money(text)
			if e != nil {
				r.Close()
				return e
			}
			expected.Add(expected, big.NewInt(n))
		}
		err = r.Err()
		r.Close()
		if err != nil {
			return err
		}
		if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || kind != a.Kind || start != a.StartPeriod || accounting.Text(cost) != a.Cost || residual < 0 || residual >= cost || months < 1 || expected.Cmp(big.NewInt(allocated)) != 0 {
			return fmt.Errorf("current opening asset differs from effective snapshot")
		}
	}
	return nil
}
func verifyOpeningSnapshot(ctx context.Context, db *sql.DB, id string, in openingSnapshot) error {
	expected, actual := map[string]*big.Int{}, map[string]*big.Int{}
	add := func(m map[string]*big.Int, account, partner string, d, c int64) {
		for i, n := range []int64{d, c} {
			k := fmt.Sprintf("%s|%s|%d", account, partner, i)
			if m[k] == nil {
				m[k] = new(big.Int)
			}
			m[k].Add(m[k], big.NewInt(n))
		}
	}
	for _, l := range in.Lines {
		d, e := accounting.Money(l.Debit)
		if e != nil {
			return e
		}
		c, e := accounting.Money(l.Credit)
		if e != nil {
			return e
		}
		add(expected, l.AccountID, l.PartnerID, d, c)
	}
	for _, s := range in.Stocks {
		v, e := accounting.Money(s.Value)
		if e != nil {
			return e
		}
		if v > 0 {
			add(expected, "156", "", v, 0)
		}
	}
	for _, a := range in.Assets {
		c, e := accounting.Money(a.Cost)
		if e != nil {
			return e
		}
		v, e := accounting.Money(a.Allocated)
		if e != nil {
			return e
		}
		if a.Kind == "fixed" {
			add(expected, "211", "", c, 0)
			if v > 0 {
				add(expected, "214", "", 0, v)
			}
		} else if c > v {
			add(expected, "242", "", c-v, 0)
		}
	}
	r, e := db.QueryContext(ctx, "SELECT account_id,COALESCE(partner_id,''),debit,credit FROM document_lines WHERE document_id=?", id)
	if e != nil {
		return e
	}
	for r.Next() {
		var account, partner string
		var d, c int64
		if e = r.Scan(&account, &partner, &d, &c); e != nil {
			r.Close()
			return e
		}
		add(actual, account, partner, d, c)
	}
	e = r.Err()
	r.Close()
	if e != nil {
		return e
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("opening adjustment line count differs")
	}
	for k, v := range expected {
		if actual[k] == nil || actual[k].Cmp(v) != 0 {
			return fmt.Errorf("opening adjustment ledger differs from snapshot")
		}
	}
	return nil
}
func verifyAdjustedStock(ctx context.Context, db *sql.DB, before, after openingSnapshot, reverse, id string) error {
	var scale int
	if len(before.Stocks)+len(after.Stocks) > 0 {
		if err := db.QueryRowContext(ctx, "SELECT json_extract(p.data,'$.quantityScale') FROM documents d JOIN policies p ON p.id=d.policy_id WHERE d.id=?", id).Scan(&scale); err != nil {
			return err
		}
	}
	type key struct{ item, warehouse string }
	type value struct{ quantity, value int64 }
	totals := func(in openingSnapshot) (map[key]value, error) {
		m := map[key]value{}
		for _, s := range in.Stocks {
			k := key{s.ItemID, s.WarehouseID}
			v := m[k]
			q, e := accounting.Quantity(s.Quantity, scale)
			if e != nil {
				return nil, e
			}
			n, e := accounting.Money(s.Value)
			if e != nil {
				return nil, e
			}
			v.quantity, e = accounting.Add(v.quantity, q)
			if e != nil {
				return nil, e
			}
			v.value, e = accounting.Add(v.value, n)
			if e != nil {
				return nil, e
			}
			m[k] = v
		}
		return m, nil
	}
	b, e := totals(before)
	if e != nil {
		return e
	}
	a, e := totals(after)
	if e != nil {
		return e
	}
	all := map[key]bool{}
	for k := range b {
		all[k] = true
	}
	for k := range a {
		all[k] = true
	}
	for _, side := range []struct {
		id     string
		values map[key]value
		sign   int64
	}{{reverse, b, -1}, {id, a, 1}} {
		expected := map[key]value{}
		for k := range all {
			if b[k] != a[k] && side.values[k].quantity > 0 {
				v := side.values[k]
				expected[k] = value{v.quantity * side.sign, v.value * side.sign}
			}
		}
		actual := map[key]value{}
		r, e := db.QueryContext(ctx, "SELECT m.item_id,w.warehouse_id,m.quantity,m.value,m.scale FROM stock_movements m JOIN stock_movement_warehouses w ON w.movement_id=m.id WHERE m.document_id=?", side.id)
		if e != nil {
			return e
		}
		for r.Next() {
			var k key
			var q, v int64
			var s int
			if e = r.Scan(&k.item, &k.warehouse, &q, &v, &s); e != nil {
				r.Close()
				return e
			}
			if s != scale {
				r.Close()
				return fmt.Errorf("opening adjustment stock scale differs")
			}
			n := actual[k]
			n.quantity, e = accounting.Add(n.quantity, q)
			if e == nil {
				n.value, e = accounting.Add(n.value, v)
			}
			if e != nil {
				r.Close()
				return e
			}
			actual[k] = n
		}
		e = r.Err()
		r.Close()
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(actual, expected) {
			return fmt.Errorf("opening adjustment stock differs from snapshot")
		}
	}
	return nil
}
