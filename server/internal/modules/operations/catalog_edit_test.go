package operations

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestCatalogEditUnusedRecordsReplayAndAudit(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	b := BankAccount{ID: "edit-bank", Number: "00001", BankName: "Ngân hàng nhập nhầm", AccountID: "112", Active: true}
	if _, err := f.ops.SaveBank(ctx, f.cmd("bank-create"), b); err != nil {
		t.Fatal(err)
	}
	b.Number = "00002"
	b.BankName = "Ngân hàng đã sửa"
	b.AccountID = "1121"
	first, err := f.ops.SaveBank(ctx, f.cmd("bank-edit"), b)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.ops.SaveBank(ctx, f.cmd("bank-edit"), b)
	if err != nil || string(first) != string(again) {
		t.Fatal("retry", err)
	}
	var saved BankAccount
	if err = json.Unmarshal(first, &saved); err != nil || saved.HasReferences {
		t.Fatal(saved, err)
	}
	event := lastAudit(t, f, "bank.save")
	undo, err := auditRestore(t, f, event, "undo-bank-edit")
	if err != nil {
		t.Fatal(err)
	}
	var number, account string
	if err = f.store.Read.QueryRow("SELECT number,account_id FROM bank_accounts WHERE id=?", b.ID).Scan(&number, &account); err != nil || number != "00001" || account != "112" {
		t.Fatal(number, account, err)
	}
	if _, err = auditRestore(t, f, undo, "redo-bank-edit"); err != nil {
		t.Fatal(err)
	}
	w := Warehouse{ID: "edit-wh", Code: "WRONG", Name: "Kho nhập nhầm", Active: true}
	if _, err = f.ops.SaveWarehouse(ctx, f.cmd("wh-create"), w); err != nil {
		t.Fatal(err)
	}
	w.Code = "CORRECT"
	w.Name = "Kho đã sửa"
	if _, err = f.ops.SaveWarehouse(ctx, f.cmd("wh-edit"), w); err != nil {
		t.Fatal(err)
	}
	item := Item{ID: "edit-item", Code: "HH-WRONG", Name: "Mã nhập nhầm", Unit: "cái", Tracked: true, Active: true}
	if _, err = f.ops.SaveItem(ctx, f.cmd("item-create"), item); err != nil {
		t.Fatal(err)
	}
	item.Code = "DV-CORRECT"
	item.Unit = "tháng"
	item.Tracked = false
	if _, err = f.ops.SaveItem(ctx, f.cmd("item-edit"), item); err != nil {
		t.Fatal(err)
	}
	employee := Employee{ID: "edit-employee", Code: "NV-WRONG", Name: "Nhân viên giả lập", BaseSalary: "2600", InsuranceBase: "1000", Active: true}
	if _, err = f.ops.SaveEmployee(ctx, f.cmd("employee-create"), employee); err != nil {
		t.Fatal(err)
	}
	employee.Code = "NV-CORRECT"
	if _, err = f.ops.SaveEmployee(ctx, f.cmd("employee-edit"), employee); err != nil {
		t.Fatal(err)
	}
	a := ledger.Account{ID: "edit-account", Code: "6991", Name: "Tài khoản nhập nhầm", Group: "expense", Active: true}
	if _, err = f.ledger.SaveAccount(ctx, f.cmd("account-create"), a); err != nil {
		t.Fatal(err)
	}
	a.Code = "6992"
	a.Name = "Tài khoản đã sửa"
	if _, err = f.ledger.SaveAccount(ctx, f.cmd("account-edit"), a); err != nil {
		t.Fatal(err)
	}
	p := ledger.Partner{ID: "edit-partner", Code: "KH-WRONG", Name: "Đối tác giả lập", Role: "customer", Active: true}
	if _, err = f.ledger.SavePartners(ctx, f.cmd("partner-create"), []ledger.Partner{p}); err != nil {
		t.Fatal(err)
	}
	p.Code = "DT-CORRECT"
	p.Role = "both"
	if _, err = f.ledger.SavePartners(ctx, f.cmd("partner-edit"), []ledger.Partner{p}); err != nil {
		t.Fatal(err)
	}
	data, err := f.ops.Data(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range data.Items {
		if i.HasReferences {
			t.Fatal("unused item locked", i.ID)
		}
	}
	banks, err := f.ops.Banks(ctx, "2026-11")
	if err != nil || banks.Accounts[0].Number != "00002" || banks.Accounts[0].AccountID != "1121" || banks.Accounts[0].HasReferences {
		t.Fatal(banks, err)
	}
	archive, _, err := f.store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Stage(ctx, archive); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.Open(ctx, f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.store = store
	f.ledger.Store = store
	t.Cleanup(func() { store.Close() })
	if err = store.Read.QueryRow("SELECT number,account_id FROM bank_accounts WHERE id=?", b.ID).Scan(&number, &account); err != nil || number != "00002" || account != "1121" {
		t.Fatal("reopen", number, account, err)
	}
	if err = sqlite.Check(ctx, store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogEditReferencedFieldsAndHistoricalSnapshots(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	b := BankAccount{ID: "used-bank", Number: "00003", BankName: "Tên ngân hàng gốc", AccountID: "112", Active: true}
	if _, err := f.ops.SaveBank(ctx, f.cmd("bank"), b); err != nil {
		t.Fatal(err)
	}
	result := executeTreasury(t, f, "bank-receipt", Input{Kind: "receipt", SourceID: "bank-receipt", Date: "2026-10-01", Description: "Vốn giả lập", BankAccountID: b.ID, MoneyAccount: "112", CounterAccount: "411", Amount: "9007199254740993"})
	b.BankName = "Tên ngân hàng hiệu chỉnh"
	if _, err := f.ops.SaveBank(ctx, f.cmd("bank-name"), b); err != nil {
		t.Fatal(err)
	}
	event := lastAudit(t, f, "bank.save")
	undo, err := auditRestore(t, f, event, "undo-used-bank-name")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auditRestore(t, f, undo, "redo-used-bank-name"); err != nil {
		t.Fatal(err)
	}
	var oldName string
	if err := f.store.Read.QueryRow("SELECT json_extract(payload,'$.voucher.bankName') FROM operations WHERE document_id=?", result.Document.ID).Scan(&oldName); err != nil || oldName != "Tên ngân hàng gốc" {
		t.Fatal("snapshot", oldName, err)
	}
	for _, field := range []string{"number", "account"} {
		changed := b
		if field == "number" {
			changed.Number = "00004"
		} else {
			changed.AccountID = "1121"
		}
		changed.HasReferences = false
		if _, err := f.ops.SaveBank(ctx, f.cmd("blocked-bank-"+field), changed); accounting.Code(err) != "HISTORY_LOCKED" {
			t.Fatal(field, err)
		}
	}
	banks, err := f.ops.Banks(ctx, "2026-11")
	if err != nil || !banks.Accounts[0].HasReferences || banks.Accounts[0].Closing != "9007199254740993" {
		t.Fatal(banks, err)
	}
	w := Warehouse{ID: "used-wh", Code: "WH1", Name: "Kho gốc", Active: true}
	if _, err = f.ops.SaveWarehouse(ctx, f.cmd("wh"), w); err != nil {
		t.Fatal(err)
	}
	i := Item{ID: "used-item", Code: "HH1", Name: "Hàng giả lập", Unit: "cái", Tracked: true, Active: true}
	if _, err = f.ops.SaveItem(ctx, f.cmd("item"), i); err != nil {
		t.Fatal(err)
	}
	in := tradeInput("stock-purchase", "purchase", "100")
	in.TradeMode = "goods"
	in.Lines[0].ItemID = i.ID
	in.Lines[0].WarehouseID = w.ID
	executeTreasury(t, f, "stock-purchase", in)
	w.Name = "Kho đã hiệu chỉnh"
	if _, err = f.ops.SaveWarehouse(ctx, f.cmd("wh-name"), w); err != nil {
		t.Fatal(err)
	}
	w.Code = "WH2"
	if _, err = f.ops.SaveWarehouse(ctx, f.cmd("wh-code"), w); accounting.Code(err) != "HISTORY_LOCKED" {
		t.Fatal(err)
	}
	for _, field := range []string{"code", "unit", "type"} {
		changed := i
		switch field {
		case "code":
			changed.Code = "HH2"
		case "unit":
			changed.Unit = "hộp"
		case "type":
			changed.Tracked = false
		}
		if _, err = f.ops.SaveItem(ctx, f.cmd("item-"+field), changed); accounting.Code(err) != "HISTORY_LOCKED" {
			t.Fatal(field, err)
		}
	}
	warehouses, err := f.ops.Warehouses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range warehouses.Warehouses {
		if row.ID == w.ID && !row.HasReferences {
			t.Fatal("warehouse missing reference")
		}
	}
	data, err := f.ops.Data(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range data.Items {
		if row.ID == i.ID && !row.HasReferences {
			t.Fatal("item missing reference")
		}
	}
	a := ledger.Account{ID: "used-account", Code: "6981", Name: "Tài khoản giả lập", Group: "expense", Active: true}
	if _, err = f.ledger.SaveAccount(ctx, f.cmd("a"), a); err != nil {
		t.Fatal(err)
	}
	p := ledger.Partner{ID: "used-partner", Code: "DT1", Name: "Đối tác giả lập", Role: "both", Active: true}
	if _, err = f.ledger.SavePartners(ctx, f.cmd("p"), []ledger.Partner{p}); err != nil {
		t.Fatal(err)
	}
	doc := f.draft("reference-draft")
	doc.PartnerID = p.ID
	doc.Lines = []ledger.Line{{AccountID: a.ID, PartnerID: p.ID, Debit: "10", Credit: "0"}, {AccountID: "411", Debit: "0", Credit: "10"}}
	if _, err = f.ledger.Save(ctx, f.cmd("ref-draft"), doc); err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.ledger.Snapshot(ctx, f.actor, "owner", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range snapshot.Accounts {
		if row.ID == a.ID && !row.HasReferences {
			t.Fatal("account missing global reference")
		}
	}
	for _, row := range snapshot.Partners {
		if row.ID == p.ID && !row.HasReferences {
			t.Fatal("partner missing global reference")
		}
	}
	a.Code = "6982"
	if _, err = f.ledger.SaveAccount(ctx, f.cmd("a-locked"), a); accounting.Code(err) != "HISTORY_LOCKED" {
		t.Fatal(err)
	}
	p.Code = "DT2"
	if _, err = f.ledger.SavePartners(ctx, f.cmd("p-locked"), []ledger.Partner{p}); accounting.Code(err) != "HISTORY_LOCKED" {
		t.Fatal(err)
	}
	p.Code = "DT1"
	p.Email = "edited@example.test"
	if _, err = f.ledger.SavePartners(ctx, f.cmd("p-contact"), []ledger.Partner{p}); err != nil {
		t.Fatal(err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
	archive, _, err := f.store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Stage(ctx, archive); err != nil {
		t.Fatal("backup after bank rename", err)
	}
}

func TestCatalogEditEmployeeAndDuplicateCodes(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	e := Employee{ID: "employee", Code: "NV1", Name: "Nhân viên giả lập", BaseSalary: "2600", InsuranceBase: "1000", Dependents: 1, Active: true}
	if _, err := f.ops.SaveEmployee(ctx, f.cmd("employee"), e); err != nil {
		t.Fatal(err)
	}
	executeTreasury(t, f, "payroll", Input{Kind: "payroll", SourceID: "salary", Date: "2026-10-31", Description: "Lương giả lập", EmployeeID: e.ID, WorkedDays: "26", Allowance: "0", NonTaxableAllowance: "0", OvertimeHours: "0", OvertimeMultiplier: "1"})
	e.Code = "NV2"
	if _, err := f.ops.SaveEmployee(ctx, f.cmd("employee-code"), e); accounting.Code(err) != "HISTORY_LOCKED" {
		t.Fatal(err)
	}
	e.Code = "NV1"
	e.Name = "Tên đã sửa"
	e.BaseSalary = "5000"
	if _, err := f.ops.SaveEmployee(ctx, f.cmd("employee-name"), e); err != nil {
		t.Fatal(err)
	}
	data, err := f.ops.Data(ctx)
	if err != nil || len(data.Employees) != 1 || !data.Employees[0].HasReferences {
		t.Fatal(data.Employees, err)
	}
	duplicate := e
	duplicate.ID = "other"
	duplicate.HasReferences = false
	if _, err = f.ops.SaveEmployee(ctx, f.cmd("employee-duplicate"), duplicate); accounting.Code(err) != "DUPLICATE_CODE" {
		t.Fatal(err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogEditConcurrentBankMappingAndPosting(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	b := BankAccount{ID: "bank", Number: "00005", BankName: "Ngân hàng giả lập", AccountID: "112", Active: true}
	if _, err := f.ops.SaveBank(ctx, f.cmd("bank"), b); err != nil {
		t.Fatal(err)
	}
	changed := b
	changed.AccountID = "1121"
	in := Input{Kind: "receipt", SourceID: "concurrent-receipt", Date: "2026-10-01", Description: "Vốn giả lập", BankAccountID: b.ID, MoneyAccount: "112", CounterAccount: "411", Amount: "100"}
	var wg sync.WaitGroup
	wg.Add(2)
	var editErr, postErr error
	go func() { defer wg.Done(); _, editErr = f.ops.SaveBank(ctx, f.cmd("edit"), changed) }()
	go func() { defer wg.Done(); _, postErr = f.ops.Execute(ctx, f.cmd("post"), in) }()
	wg.Wait()
	if editErr == nil && postErr == nil {
		t.Fatal("mapping changed during posting")
	}
	if editErr != nil && accounting.Code(editErr) != "HISTORY_LOCKED" {
		t.Fatal(editErr)
	}
	if postErr != nil && accounting.Code(postErr) != "INVALID_BANK" {
		t.Fatal(postErr)
	}
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogEditBankReferencesIncludeDraftsAndOpening(t *testing.T) {
	for _, source := range []string{"voucher-source", "voucher-destination", "opening-draft", "opening-adjustment-draft"} {
		t.Run(source, func(t *testing.T) {
			f := treasuryFixture(t)
			ctx := context.Background()
			b := BankAccount{ID: "bank", Number: "00006", BankName: "Ngân hàng giả lập", AccountID: "1121", Active: true}
			if _, err := f.ops.SaveBank(ctx, f.cmd("bank"), b); err != nil {
				t.Fatal(err)
			}
			switch source {
			case "voucher-source":
				in := Input{Kind: "receipt", SourceID: "draft-bank", Date: "2026-10-01", Description: "Nháp ngân hàng", BankAccountID: b.ID, MoneyAccount: "1121", CounterAccount: "411", Amount: "100"}
				if _, err := f.ops.SaveVoucher(ctx, f.cmd("draft"), in); err != nil {
					t.Fatal(err)
				}
			case "voucher-destination":
				in := Input{Kind: "transfer", SourceID: "draft-bank", Date: "2026-10-01", Description: "Nháp chuyển ngân hàng", DestinationBankAccountID: b.ID, MoneyAccount: "111", DestinationAccount: "1121", Amount: "100"}
				if _, err := f.ops.SaveVoucher(ctx, f.cmd("draft"), in); err != nil {
					t.Fatal(err)
				}
			case "opening-draft":
				in := openingInput()
				in.Lines = []OpeningLine{openingLine("1121", "", "1000", "0"), openingLine("411", "", "0", "1000")}
				in.Lines[0].BankAccountID = b.ID
				if _, err := f.ops.SaveOpening(ctx, f.cmd("opening"), in); err != nil {
					t.Fatal(err)
				}
			case "opening-adjustment-draft":
				state := confirmInitial(t, f, openingInput())
				in := adjustmentInput(state, "bank-adjustment", "1000")
				in.Input.Lines = []OpeningLine{openingLine("1121", "", "1000", "0"), openingLine("411", "", "0", "1000")}
				in.Input.Lines[0].BankAccountID = b.ID
				saveAdjustment(t, f, "adjustment", in)
			}
			list, err := f.ops.Banks(ctx, "2026-11")
			if err != nil || len(list.Accounts) != 1 || !list.Accounts[0].HasReferences {
				t.Fatal(list, err)
			}
			b.Number = "00007"
			if _, err = f.ops.SaveBank(ctx, f.cmd("edit-used"), b); accounting.Code(err) != "HISTORY_LOCKED" {
				t.Fatal(err)
			}
			if err = sqlite.Check(ctx, f.store.Read); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCatalogEditBankNameWithInactiveLedgerAccount(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	b := BankAccount{ID: "bank", Number: "00008", BankName: "Tên cũ", AccountID: "1121", Active: true}
	if _, err := f.ops.SaveBank(ctx, f.cmd("bank"), b); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ledger.SaveAccount(ctx, f.cmd("deactivate-account"), ledger.Account{ID: "1121", Code: "1121", Name: "Ngân hàng phụ", Group: "cash", Active: false}); err != nil {
		t.Fatal(err)
	}
	b.BankName = "Tên đã sửa"
	if _, err := f.ops.SaveBank(ctx, f.cmd("rename-bank"), b); err != nil {
		t.Fatal(err)
	}
	b.ID = "new-bank"
	b.Number = "00009"
	if _, err := f.ops.SaveBank(ctx, f.cmd("new-inactive-bank"), b); accounting.Code(err) != "INVALID_ACCOUNT" {
		t.Fatal(err)
	}
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}
