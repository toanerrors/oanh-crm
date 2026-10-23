package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"oanh.local/accounting/internal/accounting"
)

func verifyBanks(ctx context.Context, db *sql.DB) error {
	var exists int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name='bank_movements'").Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	type dimension struct{ document, account string }
	type totals struct{ debit, credit *big.Int }
	actual, expected := map[dimension]*totals{}, map[dimension]*totals{}
	add := func(m map[dimension]*totals, key dimension, debit, credit int64) {
		if m[key] == nil {
			m[key] = &totals{new(big.Int), new(big.Int)}
		}
		m[key].debit.Add(m[key].debit, big.NewInt(debit))
		m[key].credit.Add(m[key].credit, big.NewInt(credit))
	}
	gl := map[dimension]*totals{}
	rows, err := db.QueryContext(ctx, "SELECT m.document_id,m.bank_id,b.account_id,m.debit,m.credit,d.status FROM bank_movements m JOIN bank_accounts b ON b.id=m.bank_id JOIN documents d ON d.id=m.document_id")
	if err != nil {
		return err
	}
	for rows.Next() {
		var doc, bank, account, status string
		var debit, credit int64
		if err = rows.Scan(&doc, &bank, &account, &debit, &credit, &status); err != nil {
			rows.Close()
			return err
		}
		if status != "posted" {
			rows.Close()
			return fmt.Errorf("bank movement references draft")
		}
		add(actual, dimension{doc, bank}, debit, credit)
		add(gl, dimension{doc, account}, debit, credit)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	type entry struct{ doc, kind, payload, reversal string }
	entries := []entry{}
	rows, err = db.QueryContext(ctx, "SELECT d.id,o.kind,o.payload,COALESCE(d.reversal_of,'') FROM operations o JOIN documents d ON d.id=o.document_id WHERE d.status='posted' AND o.kind IN ('receipt','payment','transfer','treasury_reversal')")
	if err != nil {
		return err
	}
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.doc, &e.kind, &e.payload, &e.reversal); err != nil {
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
	var openingTable int
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name='opening_balances'").Scan(&openingTable); err != nil {
		return err
	}

	if openingTable > 0 {
		query := "SELECT o.payload,d.id,0 FROM opening_balances o JOIN documents d ON d.id=o.document_id WHERE d.status='posted'"
		var adjustments int
		if err = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name='opening_adjustments'").Scan(&adjustments); err != nil {
			return err
		}
		if adjustments > 0 {
			query += " UNION ALL SELECT before_payload,reversal_document_id,1 FROM opening_adjustments WHERE confirmed_version IS NOT NULL UNION ALL SELECT after_payload,document_id,0 FROM opening_adjustments WHERE confirmed_version IS NOT NULL"
		}
		r, e := db.QueryContext(ctx, query)
		if e != nil {
			return e
		}
		type snapshot struct {
			raw, doc string
			reverse  bool
		}
		snapshots := []snapshot{}
		for r.Next() {
			var v snapshot
			if e = r.Scan(&v.raw, &v.doc, &v.reverse); e != nil {
				r.Close()
				return e
			}
			snapshots = append(snapshots, v)
		}
		e = r.Err()
		r.Close()
		if e != nil {
			return e
		}
		for _, v := range snapshots {
			var input struct {
				Lines []struct {
					BankAccountID string `json:"bankAccountId"`
					Debit         string `json:"debit"`
					Credit        string `json:"credit"`
				} `json:"lines"`
			}
			if e = json.Unmarshal([]byte(v.raw), &input); e != nil {
				return e
			}
			for _, l := range input.Lines {
				if l.BankAccountID == "" {
					continue
				}
				d, e := accounting.Money(l.Debit)
				if e != nil {
					return e
				}
				c, e := accounting.Money(l.Credit)
				if e != nil {
					return e
				}
				if v.reverse {
					d, c = c, d
				}
				add(expected, dimension{v.doc, l.BankAccountID}, d, c)
			}
		}
	}

	for _, e := range entries {
		var input struct {
			BankAccountID            string `json:"bankAccountId"`
			DestinationBankAccountID string `json:"destinationBankAccountId"`
			MoneyAccount             string `json:"moneyAccount"`
			DestinationAccount       string `json:"destinationAccount"`
			Amount                   string `json:"amount"`
			OriginalDocumentID       string `json:"originalDocumentId"`
			Voucher                  struct {
				BankNumber            string `json:"bankNumber"`
				BankName              string `json:"bankName"`
				DestinationBankNumber string `json:"destinationBankNumber"`
				DestinationBankName   string `json:"destinationBankName"`
			} `json:"voucher"`
		}
		if err = json.Unmarshal([]byte(e.payload), &input); err != nil {
			return err
		}
		if e.kind == "treasury_reversal" {
			if e.reversal != input.OriginalDocumentID {
				return fmt.Errorf("bank reversal source mismatch")
			}
			r, err := db.QueryContext(ctx, "SELECT bank_id,debit,credit FROM bank_movements WHERE document_id=?", input.OriginalDocumentID)
			if err != nil {
				return err
			}
			for r.Next() {
				var id string
				var d, c int64
				if err = r.Scan(&id, &d, &c); err != nil {
					r.Close()
					return err
				}
				add(expected, dimension{e.doc, id}, c, d)
			}
			err = r.Err()
			r.Close()
			if err != nil {
				return err
			}
			continue
		}
		if input.BankAccountID == "" && input.DestinationBankAccountID == "" {
			continue
		}
		n, err := accounting.Money(input.Amount)
		if err != nil || n == 0 {
			return fmt.Errorf("invalid bank movement amount")
		}
		check := func(id, account, number, name string) error {
			if id == "" {
				return nil
			}
			var a, num string
			if er := db.QueryRowContext(ctx, "SELECT account_id,number FROM bank_accounts WHERE id=?", id).Scan(&a, &num); er != nil {
				return er
			}
			// Bank names are editable; the voucher retains its original name snapshot.
			if a != account || num != number || strings.TrimSpace(name) == "" {
				return fmt.Errorf("bank reference differs from source snapshot")
			}
			return nil
		}
		if err = check(input.BankAccountID, input.MoneyAccount, input.Voucher.BankNumber, input.Voucher.BankName); err != nil {
			return err
		}
		if err = check(input.DestinationBankAccountID, input.DestinationAccount, input.Voucher.DestinationBankNumber, input.Voucher.DestinationBankName); err != nil {
			return err
		}
		if input.BankAccountID != "" {
			if e.kind == "receipt" {
				add(expected, dimension{e.doc, input.BankAccountID}, n, 0)
			} else {
				add(expected, dimension{e.doc, input.BankAccountID}, 0, n)
			}
		}
		if e.kind == "transfer" && input.DestinationBankAccountID != "" {
			add(expected, dimension{e.doc, input.DestinationBankAccountID}, n, 0)
		}
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("bank movements differ from operations")
	}
	for key, a := range actual {
		e := expected[key]
		if e == nil || a.debit.Cmp(e.debit) != 0 || a.credit.Cmp(e.credit) != 0 {
			return fmt.Errorf("bank operation reconciliation mismatch")
		}
	}
	ledger := map[dimension]*totals{}
	rows, err = db.QueryContext(ctx, "SELECT l.document_id,l.account_id,l.debit,l.credit FROM document_lines l WHERE EXISTS(SELECT 1 FROM bank_movements m WHERE m.document_id=l.document_id)")
	if err != nil {
		return err
	}
	for rows.Next() {
		var doc, account string
		var d, c int64
		if err = rows.Scan(&doc, &account, &d, &c); err != nil {
			rows.Close()
			return err
		}
		key := dimension{doc, account}
		if gl[key] != nil {
			add(ledger, key, d, c)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for key, a := range gl {
		e := ledger[key]
		if e == nil || a.debit.Cmp(e.debit) != 0 || a.credit.Cmp(e.credit) != 0 {
			return fmt.Errorf("bank detail differs from ledger")
		}
	}

	var invalid int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM operations o JOIN documents d ON d.id=o.document_id LEFT JOIN documents old ON old.id=json_extract(o.payload,'$.replacementOf') LEFT JOIN operations original ON original.document_id=old.id WHERE COALESCE(json_extract(o.payload,'$.replacementOf'),'')<>'' AND (o.kind NOT IN ('receipt','payment','transfer') OR old.id IS NULL OR COALESCE(original.kind,'')<>o.kind OR old.status<>'posted' OR old.source_id=d.source_id OR old.date>d.date OR length(trim(COALESCE(json_extract(o.payload,'$.replacementReason'),'')))=0 OR NOT EXISTS(SELECT 1 FROM documents r WHERE r.reversal_of=old.id AND r.status='posted' AND r.source_type='treasury_reversal' AND r.date=d.date AND r.posted_order<d.posted_order))`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("treasury replacement differs from reversal or source")
	}
	return nil
}
