package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"

	"oanh.local/accounting/internal/accounting"
)

func fingerprint(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, "SELECT type,name,tbl_name,COALESCE(sql,'') FROM sqlite_schema WHERE substr(name,1,7)<>'sqlite_' AND name<>'schema_migrations' ORDER BY type,name")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	values := [][4]string{}
	for rows.Next() {
		var v [4]string
		if err = rows.Scan(&v[0], &v[1], &v[2], &v[3]); err != nil {
			return "", err
		}
		values = append(values, v)
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return Hash(data), nil
}

var expectedFingerprints = sync.OnceValues(func() ([4]string, error) {
	var values [4]string
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return values, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for i, source := range []string{schema, vouchersSchema, financialSchema, openingSchema} {
		if _, err = db.Exec(source); err != nil {
			return values, err
		}
		values[i], err = fingerprint(context.Background(), db)
		if err != nil {
			return values, err
		}
	}
	return values, nil
})

func verifySchema(ctx context.Context, db *sql.DB) error {
	var version, count int
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0),count(*) FROM schema_migrations").Scan(&version, &count); err != nil {
		return err
	}
	if version < 1 || version > 4 || count != version {
		return fmt.Errorf("unsupported migration version")
	}
	for i, source := range []string{schema, vouchersSchema, financialSchema, openingSchema} {
		if i >= version {
			break
		}
		var checksum string
		if err := db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=?", i+1).Scan(&checksum); err != nil {
			return err
		}
		if checksum != migrationChecksum(i+1, source) {
			return fmt.Errorf("migration checksum mismatch")
		}
	}
	expected, err := expectedFingerprints()
	if err != nil {
		return err
	}
	actual, err := fingerprint(ctx, db)
	if err != nil {
		return err
	}
	if expected[version-1] != actual {
		return fmt.Errorf("database schema fingerprint differs from application migration")
	}
	return nil
}

func verifySettlements(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT o.payload,o.kind,d.id,d.partner_id,d.date,COALESCE(r.date,'')
		FROM operations o JOIN documents d ON d.id=o.document_id LEFT JOIN documents r ON r.reversal_of=d.id AND r.status='posted'
		WHERE o.kind IN ('receipt','payment') AND json_array_length(o.payload,'$.allocations')>0 AND d.status='posted'`)
	if err != nil {
		return err
	}
	type allocation struct {
		DocumentID string `json:"documentId"`
		Amount     string `json:"amount"`
	}
	type payment struct {
		BankAccountID                             string       `json:"bankAccountId"`
		MoneyAccount                              string       `json:"moneyAccount"`
		Amount                                    string       `json:"amount"`
		Allocations                               []allocation `json:"allocations"`
		kind, documentID, partner, date, reversed string
	}
	payments := []payment{}
	for rows.Next() {
		var p payment
		var payload string
		if err = rows.Scan(&payload, &p.kind, &p.documentID, &p.partner, &p.date, &p.reversed); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal([]byte(payload), &p); err != nil {
			rows.Close()
			return err
		}
		payments = append(payments, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	totals := map[string]*big.Int{}
	limits := map[string]*big.Int{}
	for _, p := range payments {
		amount, err := accounting.Money(p.Amount)
		if err != nil || amount == 0 || len(p.Allocations) > 100 {
			return fmt.Errorf("invalid settlement amount")
		}
		if p.MoneyAccount != "111" && p.MoneyAccount != "1121" && p.MoneyAccount != "112" && p.BankAccountID == "" {
			return fmt.Errorf("invalid settlement money account")
		}
		counter := "131"
		if p.kind == "payment" {
			counter = "331"
		}
		r, e := db.QueryContext(ctx, "SELECT account_id,COALESCE(partner_id,''),debit,credit FROM document_lines WHERE document_id=?", p.documentID)
		if e != nil {
			return e
		}
		moneyNet, debtNet := new(big.Int), new(big.Int)
		for r.Next() {
			var account, partner string
			var debit, credit int64
			if e = r.Scan(&account, &partner, &debit, &credit); e != nil {
				r.Close()
				return e
			}
			if account != counter && account != p.MoneyAccount || account == counter && partner != p.partner {
				r.Close()
				return fmt.Errorf("settlement ledger dimensions differ")
			}
			if p.kind == "payment" {
				debit, credit = credit, debit
			}
			if account == counter {
				debtNet.Add(debtNet, big.NewInt(credit))
				debtNet.Sub(debtNet, big.NewInt(debit))
			} else {
				moneyNet.Add(moneyNet, big.NewInt(debit))
				moneyNet.Sub(moneyNet, big.NewInt(credit))
			}
		}
		e = r.Err()
		r.Close()
		if e != nil {
			return e
		}
		if moneyNet.Cmp(big.NewInt(amount)) != 0 || debtNet.Cmp(big.NewInt(amount)) != 0 {
			return fmt.Errorf("settlement payload differs from ledger")
		}
		total := new(big.Int)
		seen := map[string]bool{}
		for _, a := range p.Allocations {
			n, err := accounting.Money(a.Amount)
			if err != nil || n == 0 || seen[a.DocumentID] {
				return fmt.Errorf("invalid settlement allocation")
			}
			seen[a.DocumentID] = true
			total.Add(total, big.NewInt(n))
			var kind, partner, date string
			err = db.QueryRowContext(ctx, `SELECT o.kind,d.partner_id,d.date FROM documents d JOIN operations o ON o.document_id=d.id WHERE d.id=? AND d.status='posted'`, a.DocumentID).Scan(&kind, &partner, &date)
			if err != nil || partner != p.partner || date > p.date || p.reversed != "" && p.reversed < p.date || p.kind == "receipt" && kind != "sale" || p.kind == "payment" && kind != "purchase" && kind != "asset" {
				return fmt.Errorf("invalid settlement source")
			}
			if limits[a.DocumentID] == nil {
				account := "131"
				if p.kind == "payment" {
					account = "331"
				}
				r, e := db.QueryContext(ctx, "SELECT debit,credit,COALESCE(partner_id,'') FROM document_lines WHERE document_id=? AND account_id=?", a.DocumentID, account)
				if e != nil {
					return e
				}
				limit := new(big.Int)
				for r.Next() {
					var debit, credit int64
					var linePartner string
					if e = r.Scan(&debit, &credit, &linePartner); e != nil {
						r.Close()
						return e
					}
					if linePartner != partner {
						r.Close()
						return fmt.Errorf("settlement source partner differs")
					}
					if p.kind == "payment" {
						debit, credit = credit, debit
					}
					limit.Add(limit, big.NewInt(debit))
					limit.Sub(limit, big.NewInt(credit))
				}
				e = r.Err()
				r.Close()
				if e != nil {
					return e
				}
				if limit.Sign() <= 0 {
					return fmt.Errorf("settlement source has no debt")
				}
				limits[a.DocumentID] = limit
				totals[a.DocumentID] = new(big.Int)
			}
			if p.reversed == "" {
				totals[a.DocumentID].Add(totals[a.DocumentID], big.NewInt(n))
			}
		}
		if total.Cmp(big.NewInt(amount)) != 0 {
			return fmt.Errorf("settlement allocations differ from payment")
		}
	}
	for id, total := range totals {
		if total.Cmp(limits[id]) > 0 {
			return fmt.Errorf("settlements exceed source debt")
		}
	}
	return nil
}

type stockTotal struct{ quantity, value *big.Int }
type stockKey struct{ item, warehouse string }

func stocks(ctx context.Context, db *sql.DB, table string) (map[stockKey]stockTotal, error) {
	if table != "stock_lots" && table != "stock_movements" {
		return nil, fmt.Errorf("unsupported stock source")
	}
	var hasWarehouses int
	if e := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name='warehouses' AND type='table'").Scan(&hasWarehouses); e != nil {
		return nil, e
	}
	query := "SELECT item_id,quantity,value,scale,'default' FROM " + table
	if hasWarehouses > 0 {
		dimension, key := "stock_lot_warehouses", "lot_id"
		if table == "stock_movements" {
			dimension, key = "stock_movement_warehouses", "movement_id"
		}
		var missing int
		if e := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" s LEFT JOIN "+dimension+" w ON w."+key+"=s.id WHERE w.warehouse_id IS NULL").Scan(&missing); e != nil {
			return nil, e
		}
		if missing > 0 {
			return nil, fmt.Errorf("stock warehouse dimension missing")
		}
		query = "SELECT s.item_id,s.quantity,s.value,s.scale,w.warehouse_id FROM " + table + " s JOIN " + dimension + " w ON w." + key + "=s.id"
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[stockKey]stockTotal{}
	for rows.Next() {
		var id, warehouse string
		var quantity, value int64
		var scale int
		if err = rows.Scan(&id, &quantity, &value, &scale, &warehouse); err != nil {
			return nil, err
		}
		if scale < 0 || scale > 6 {
			return nil, fmt.Errorf("invalid quantity scale")
		}
		key := stockKey{id, warehouse}
		total, exists := values[key]
		if !exists {
			total = stockTotal{new(big.Int), new(big.Int)}
		}
		factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(6-scale)), nil)
		total.quantity.Add(total.quantity, new(big.Int).Mul(big.NewInt(quantity), factor))
		total.value.Add(total.value, big.NewInt(value))
		values[key] = total
	}
	return values, rows.Err()
}
func verifySubledgers(ctx context.Context, db *sql.DB) error {
	lots, err := stocks(ctx, db, "stock_lots")
	if err != nil {
		return err
	}
	movements, err := stocks(ctx, db, "stock_movements")
	if err != nil {
		return err
	}
	for id, v := range movements {
		lot, ok := lots[id]
		if !ok {
			lot = stockTotal{new(big.Int), new(big.Int)}
		}
		if lot.quantity.Cmp(v.quantity) != 0 || lot.value.Cmp(v.value) != 0 {
			return fmt.Errorf("stock movement and remaining lots differ")
		}
		delete(lots, id)
	}
	for _, v := range lots {
		if v.quantity.Sign() != 0 || v.value.Sign() != 0 {
			return fmt.Errorf("stock lot has no source movement")
		}
	}
	stockValue := new(big.Int)
	for _, v := range movements {
		stockValue.Add(stockValue, v.value)
	}
	assetValue := new(big.Int)
	rows, err := db.QueryContext(ctx, "SELECT cost,allocated FROM assets")
	if err != nil {
		return err
	}
	for rows.Next() {
		var cost, allocated int64
		if err = rows.Scan(&cost, &allocated); err != nil {
			rows.Close()
			return err
		}
		assetValue.Add(assetValue, big.NewInt(cost))
		assetValue.Sub(assetValue, big.NewInt(allocated))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	inventory, assets := new(big.Int), new(big.Int)
	rows, err = db.QueryContext(ctx, "SELECT a.account_group,l.debit,l.credit FROM document_lines l JOIN documents d ON d.id=l.document_id JOIN accounts a ON a.id=l.account_id WHERE d.status='posted' AND a.account_group IN ('inventory','asset')")
	if err != nil {
		return err
	}
	for rows.Next() {
		var group string
		var debit, credit int64
		if err = rows.Scan(&group, &debit, &credit); err != nil {
			rows.Close()
			return err
		}
		target := assets
		if group == "inventory" {
			target = inventory
		}
		target.Add(target, big.NewInt(debit))
		target.Sub(target, big.NewInt(credit))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if inventory.Cmp(stockValue) != 0 || assets.Cmp(assetValue) != 0 {
		return fmt.Errorf("subledger values differ from ledger accounts")
	}
	var draftSources int
	if err = db.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM stock_movements m JOIN documents d ON d.id=m.document_id WHERE d.status<>'posted')+(SELECT count(*) FROM assets a JOIN documents d ON d.id=a.recognized_document WHERE d.status<>'posted')").Scan(&draftSources); err != nil {
		return err
	}
	if draftSources != 0 {
		return fmt.Errorf("subledger points to unposted source")
	}
	return verifySettlements(ctx, db)
}
