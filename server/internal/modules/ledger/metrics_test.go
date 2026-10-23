package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"oanh.local/accounting/internal/platform/sqlite"
	"testing"
)

func TestDashboardMetricsWholePeriodExactBalancesAndReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := sqlite.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	for _, q := range []string{"INSERT INTO users VALUES('owner','test','Kiểm thử','owner','unused',1)", "INSERT INTO periods VALUES('2026-09',0)", "INSERT INTO periods VALUES('2026-10',0)", "INSERT INTO periods VALUES('2026-11',0)"} {
		if _, err = store.Write.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []Account{{ID: "111", Code: "111", Name: "Tiền", Group: "cash", Active: true}, {ID: "bank-ledger", Code: "1129", Name: "Tiền gửi khác", Group: "cash", Active: true}, {ID: "156", Code: "156", Name: "Hàng", Group: "inventory", Active: true}, {ID: "131", Code: "131", Name: "Phải thu", Group: "receivable", RequiresPartner: true, Active: true}, {ID: "331", Code: "331", Name: "Phải trả", Group: "payable", RequiresPartner: true, Active: true}, {ID: "4111", Code: "4111", Name: "Vốn", Group: "capital", Active: true}, {ID: "511", Code: "511", Name: "Doanh thu", Group: "revenue", Active: true}, {ID: "642", Code: "642", Name: "Chi phí", Group: "expense", Active: true}} {
		if _, err = store.Write.Exec("INSERT INTO accounts VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET account_group=excluded.account_group,requires_partner=excluded.requires_partner", a.ID, a.Code, a.Name, a.Group, a.RequiresPartner, a.Active); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = store.Write.Exec("INSERT INTO bank_accounts VALUES('bank','0009','Ngân hàng thử','bank-ledger',1)"); err != nil {
		t.Fatal(err)
	}
	var epoch string
	store.Read.QueryRow("SELECT epoch FROM meta").Scan(&epoch)
	service := Service{Store: store}
	cmd := func(key string) Command { return Command{Actor: "owner", Epoch: epoch, Key: key} }
	if _, err = service.SavePartners(ctx, cmd("partners"), []Partner{{ID: "a", Code: "A", Name: "Đối tác A", Role: "both", Active: true}, {ID: "b", Code: "B", Name: "Đối tác B", Role: "both", Active: true}}); err != nil {
		t.Fatal(err)
	}
	save := func(id, date string, lines []Line, post bool) {
		t.Helper()
		b, e := service.Save(ctx, cmd(id), Document{ID: id, Type: "journal", Date: date, Description: id, Lines: lines})
		if e != nil {
			t.Fatal(e)
		}
		var d Document
		if e = json.Unmarshal(b, &d); e != nil {
			t.Fatal(e)
		}
		if post {
			if _, e = service.Post(ctx, cmd("post-"+id), Post{ID: id, Version: d.Version}); e != nil {
				t.Fatal(e)
			}
		}
	}
	pair := func(debit, credit, amount, partner string) []Line {
		return []Line{{AccountID: debit, PartnerID: partner, Debit: amount, Credit: "0"}, {AccountID: credit, PartnerID: partner, Debit: "0", Credit: amount}}
	}
	save("past", "2026-09-01", pair("111", "4111", "9223372036854775807", ""), true)
	save("past-two", "2026-09-02", pair("111", "4111", "9223372036854775807", ""), true)
	save("purchase", "2026-10-01", pair("642", "331", "200", "a"), true)
	save("advance", "2026-10-02", pair("331", "111", "50", "b"), true)
	save("sale", "2026-10-03", pair("131", "511", "300", "a"), true)
	save("prepayment", "2026-10-04", pair("111", "131", "40", "b"), true)
	save("expense", "2026-10-05", pair("642", "111", "70", ""), true)
	save("transfer", "2026-10-06", pair("bank-ledger", "111", "10", ""), true)
	save("future", "2026-11-01", pair("642", "331", "900", "a"), true)
	for i := 0; i < 120; i++ {
		save(fmt.Sprintf("draft-%03d", i), "2026-10-22", pair("111", "4111", "1", ""), false)
	}
	lines := pair("111", "4111", "1", "")
	lines[0].Debit = "2"
	save("old-unbalanced", "2026-10-01", lines, false)
	snapshot, err := service.Snapshot(ctx, "owner", "owner", "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	m := snapshot.Server
	if len(snapshot.Documents) != 100 || m.DocumentCount != 127 || m.PostedCount != 6 || m.DraftCount != 121 || m.UnbalancedDraftCount != 1 {
		t.Fatal(len(snapshot.Documents), m)
	}
	for _, d := range snapshot.Documents {
		if d.ID == "old-unbalanced" {
			t.Fatal("fixture not beyond workspace limit")
		}
	}
	if m.Revenue != "300" || m.Expense != "270" || m.Receivable != "300" || m.Payable != "200" || m.InventoryBalance != "0" || m.MoneyBalance != "18446744073709551534" || m.BankBalance != "10" {
		t.Fatal(m)
	}
	revision := snapshot.Revision
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = sqlite.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	service.Store = store
	snapshot, err = service.Snapshot(ctx, "owner", "owner", "2026-10")
	if err != nil || snapshot.Revision != revision || snapshot.Server.Payable != "200" || snapshot.Server.UnbalancedDraftCount != 1 {
		t.Fatal(snapshot.Server, err)
	}
	if err = sqlite.Check(ctx, store.Read); err != nil {
		t.Fatal(err)
	}
}
