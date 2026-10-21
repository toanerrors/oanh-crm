package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
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

var expectedFingerprint = sync.OnceValues(func() (string, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return "", err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(schema); err != nil {
		return "", err
	}
	return fingerprint(context.Background(), db)
})

func verifySchema(ctx context.Context, db *sql.DB) error {
	expected, err := expectedFingerprint()
	if err != nil {
		return err
	}
	actual, err := fingerprint(ctx, db)
	if err != nil {
		return err
	}
	if expected != actual {
		return fmt.Errorf("database schema fingerprint differs from application migration")
	}
	var version, count int
	if err = db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0),count(*) FROM schema_migrations").Scan(&version, &count); err != nil {
		return err
	}
	if version != 1 || count != 1 {
		return fmt.Errorf("unsupported migration version")
	}
	return nil
}

type stockTotal struct{ quantity, value *big.Int }

func stocks(ctx context.Context, db *sql.DB, table string) (map[string]stockTotal, error) {
	if table != "stock_lots" && table != "stock_movements" {
		return nil, fmt.Errorf("unsupported stock source")
	}
	rows, err := db.QueryContext(ctx, "SELECT item_id,quantity,value,scale FROM "+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]stockTotal{}
	for rows.Next() {
		var id string
		var quantity, value int64
		var scale int
		if err = rows.Scan(&id, &quantity, &value, &scale); err != nil {
			return nil, err
		}
		if scale < 0 || scale > 6 {
			return nil, fmt.Errorf("invalid quantity scale")
		}
		total, exists := values[id]
		if !exists {
			total = stockTotal{new(big.Int), new(big.Int)}
		}
		factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(6-scale)), nil)
		total.quantity.Add(total.quantity, new(big.Int).Mul(big.NewInt(quantity), factor))
		total.value.Add(total.value, big.NewInt(value))
		values[id] = total
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
	return nil
}
