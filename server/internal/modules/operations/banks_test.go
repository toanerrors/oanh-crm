package operations

import (
	"context"
	"encoding/json"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestBankTransfersSameLedgerAccountAndRestore(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	a := BankAccount{ID: "bank-a", Number: "00123", BankName: "Ngân hàng A giả lập", AccountID: "112", Active: true}
	b := BankAccount{ID: "bank-b", Number: "00456", BankName: "Ngân hàng B giả lập", AccountID: "112", Active: true}
	for _, bank := range []BankAccount{a, b} {
		if _, err := f.ops.SaveBank(ctx, f.cmd(bank.ID), bank); err != nil {
			t.Fatal(err)
		}
	}
	executeTreasury(t, f, "bank-receipt", Input{Kind: "receipt", SourceID: "bank-receipt", Date: "2026-10-01", Description: "Nộp vốn giả lập", BankAccountID: a.ID, MoneyAccount: "112", CounterAccount: "411", Amount: "9007199254740993"})
	in := Input{Kind: "transfer", SourceID: "bank-transfer", Date: "2026-10-02", Description: "Chuyển giữa hai ngân hàng cùng TK112", BankAccountID: a.ID, DestinationBankAccountID: b.ID, MoneyAccount: "112", DestinationAccount: "112", Amount: "100"}
	saved, err := f.ops.SaveVoucher(ctx, f.cmd("transfer-draft"), in)
	if err != nil {
		t.Fatal(err)
	}
	var draft Voucher
	json.Unmarshal(saved, &draft)
	list, err := f.ops.Banks(ctx, "2026-10")
	if err != nil || list.Accounts[0].Closing != "9007199254740993" || list.Accounts[1].Closing != "0" {
		t.Fatal(list, err)
	}
	result := executeTreasury(t, f, "transfer-post", draft.Input)
	list, err = f.ops.Banks(ctx, "2026-10")
	if err != nil || list.Accounts[0].Closing != "9007199254740893" || list.Accounts[1].Closing != "100" {
		t.Fatal(list, err)
	}
	docs, err := f.ledger.Documents(ctx, ledger.Filter{Period: "2026-10", Account: "112", BankAccount: b.ID, Page: 1, PageSize: 10})
	if err != nil || docs.Total != 1 || docs.Money.Closing != "100" {
		t.Fatal(docs, err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
	archive, _, err := f.store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := f.store.Stage(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	b.Active = false
	if _, err = f.ops.SaveBank(ctx, f.cmd("deactivate-bank"), b); err != nil {
		t.Fatal(err)
	}
	executeTreasury(t, f, "bank-reversal", Input{Kind: "treasury_reversal", OriginalDocumentID: result.Document.ID, SourceID: "bank-reversal", Date: "2026-10-03", Description: "Đảo phiếu chuyển giả lập"})
	list, err = f.ops.Banks(ctx, "2026-10")
	if err != nil || list.Accounts[0].Closing != "9007199254740993" || list.Accounts[1].Closing != "0" {
		t.Fatal(list, err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Activate(ctx, stage.ID); err != nil {
		t.Fatal(err)
	}
	list, err = f.ops.Banks(ctx, "2026-10")
	if err != nil || list.Accounts[1].Closing != "100" {
		t.Fatal(list, err)
	}
	if _, err = f.store.Write.Exec("UPDATE bank_movements SET debit=1 WHERE document_id=?", result.Document.ID); err == nil {
		t.Fatal("posted bank detail changed")
	}
}

func TestBankPaymentRollbackAndInputValidation(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	bank := BankAccount{ID: "bank", Number: "012345", BankName: "Ngân hàng giả lập", AccountID: "112", Active: true}
	if _, err := f.ops.SaveBank(ctx, f.cmd("bank"), bank); err != nil {
		t.Fatal(err)
	}
	same := Input{Kind: "transfer", SourceID: "same-bank", Date: "2026-10-01", Description: "Sai tài khoản", MoneyAccount: "112", DestinationAccount: "112", BankAccountID: bank.ID, DestinationBankAccountID: bank.ID, Amount: "10"}
	if _, err := f.ops.Execute(ctx, f.cmd("same-bank"), same); accounting.Code(err) != "VALIDATION" {
		t.Fatal(err)
	}
	same.BankAccountID = "missing"
	if _, err := f.ops.Execute(ctx, f.cmd("missing-bank"), same); accounting.Code(err) != "INVALID_BANK" {
		t.Fatal(err)
	}
	same.BankAccountID = bank.ID
	same.MoneyAccount = "111"
	if _, err := f.ops.Execute(ctx, f.cmd("bank-mismatch"), same); accounting.Code(err) != "INVALID_BANK" {
		t.Fatal(err)
	}
	if _, err := f.ops.SaveItem(ctx, f.cmd("goods"), Item{ID: "goods", Code: "HH", Name: "Hàng kiểm thử", Unit: "cái", Tracked: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Write.Exec("CREATE TRIGGER fail_bank_insert BEFORE INSERT ON bank_movements BEGIN SELECT RAISE(ABORT,'injected failure');END"); err != nil {
		t.Fatal(err)
	}
	in := tradeInput("bank-rollback", "purchase", "1000")
	in.Lines[0].ItemID = "goods"
	in.BankAccountID = bank.ID
	in.PaymentAccount = "112"
	if _, err := f.ops.Execute(ctx, f.cmd("bank-rollback"), in); err == nil {
		t.Fatal("expected rollback")
	}
	for _, table := range []string{"documents", "operations", "stock_lots", "stock_movements", "bank_movements", "counters"} {
		var count int
		if err := f.store.Read.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	if _, err := f.store.Write.Exec("DROP TRIGGER fail_bank_insert"); err != nil {
		t.Fatal(err)
	}
	result := executeTreasury(t, f, "bank-purchase", in)
	if result.PaymentDocument == nil {
		t.Fatal(result)
	}
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
	bank.BankName = "Đổi lịch sử"
	if _, err := f.ops.SaveBank(ctx, f.cmd("rename-bank"), bank); accounting.Code(err) != "HISTORY_LOCKED" {
		t.Fatal(err)
	}
}
