package operations

import (
	"context"
	"encoding/json"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestTreasuryReplacementDraftPreviewAtomicPosting(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	f.policy(t, testPolicy())
	original := Input{Kind: "receipt", SourceID: "original", Date: "2026-10-02", Description: "Thu giả lập", MoneyAccount: "111", CounterAccount: "411", Amount: "1000000"}
	b, e := f.ops.Execute(ctx, f.cmd("original"), original)
	if e != nil {
		t.Fatal(e)
	}
	var old Result
	if e = json.Unmarshal(b, &old); e != nil {
		t.Fatal(e)
	}
	replacement := original
	replacement.SourceID = "replacement"
	replacement.Amount = "5000000"
	replacement.ReplacementOf = old.Document.ID
	replacement.ReplacementReason = "Đối chiếu số tiền đúng"
	b, e = f.ops.SaveVoucher(ctx, f.cmd("save-replacement"), replacement)
	if e != nil {
		t.Fatal(e)
	}
	var v Voucher
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = f.store.Read.QueryRow("SELECT count(*) FROM documents WHERE reversal_of=?", old.Document.ID).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
	replacement = v.Input
	p, e := f.ops.Preview(ctx, identity.User{ID: f.actor, Role: "owner"}, f.epoch, replacement)
	if e != nil {
		t.Fatal(e)
	}
	replacement.ExpectedPolicyID = p.PolicyID
	replacement.ExpectedRevision = &p.Revision
	if _, e = f.store.Write.Exec("CREATE TRIGGER fail_replacement BEFORE UPDATE OF status ON documents WHEN NEW.id='" + v.Document.ID + "' AND NEW.status='posted' BEGIN SELECT RAISE(ABORT,'simulated failure');END"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ops.Execute(ctx, f.cmd("rollback"), replacement); e == nil {
		t.Fatal("expected rollback")
	}
	if e = f.store.Read.QueryRow("SELECT count(*) FROM documents WHERE reversal_of=?", old.Document.ID).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
	if _, e = f.store.Write.Exec("DROP TRIGGER fail_replacement"); e != nil {
		t.Fatal(e)
	}
	b, e = f.ops.Execute(ctx, f.cmd("replace"), replacement)
	if e != nil {
		t.Fatal(e)
	}
	var result Result
	if e = json.Unmarshal(b, &result); e != nil {
		t.Fatal(e)
	}
	retry, e := f.ops.Execute(ctx, f.cmd("replace"), replacement)
	if e != nil || string(retry) != string(b) {
		t.Fatal(e)
	}
	view, e := f.ops.Voucher(ctx, old.Document.ID)
	if e != nil || view.Replacement == nil || view.Replacement.ID != result.Document.ID || view.Reversal == nil {
		t.Fatal(view, e)
	}
	newer, e := f.ops.Voucher(ctx, result.Document.ID)
	if e != nil || newer.Input.ReplacementOf != old.Document.ID || newer.Input.ReplacementReason != replacement.ReplacementReason {
		t.Fatal(newer, e)
	}
	replacement.DraftID = ""
	replacement.DraftVersion = 0
	replacement.SourceID = "duplicate"
	replacement.ExpectedRevision = nil
	if _, e = f.ops.Execute(ctx, f.cmd("duplicate"), replacement); accounting.Code(e) != "DUPLICATE_SOURCE" {
		t.Fatal(e)
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
}

func TestTreasuryReplacementBankMovements(t *testing.T) {
	for _, kind := range []string{"receipt", "payment", "transfer"} {
		t.Run(kind, func(t *testing.T) {
			f := openFixture(t)
			ctx := context.Background()
			f.policy(t, testPolicy())
			if _, e := f.ops.SaveBank(ctx, f.cmd("bank"), BankAccount{ID: "bank", Number: "000123", BankName: "Ngân hàng giả lập", AccountID: "112", Active: true}); e != nil {
				t.Fatal(e)
			}
			in := Input{Kind: kind, SourceID: "original", Date: "2026-10-02", Description: "Phiếu giả lập", BankAccountID: "bank", MoneyAccount: "112", CounterAccount: "411", Amount: "100"}
			if kind == "transfer" {
				in.CounterAccount = ""
				in.DestinationAccount = "111"
			}
			b, e := f.ops.Execute(ctx, f.cmd("original"), in)
			if e != nil {
				t.Fatal(e)
			}
			var original Result
			if e = json.Unmarshal(b, &original); e != nil {
				t.Fatal(e)
			}
			in.SourceID = "replacement"
			in.ReplacementOf = original.Document.ID
			in.ReplacementReason = "Sửa số tiền"
			in.Amount = "300"
			b, e = f.ops.Execute(ctx, f.cmd("replacement"), in)
			if e != nil {
				t.Fatal(e)
			}
			var replaced Result
			if e = json.Unmarshal(b, &replaced); e != nil {
				t.Fatal(e)
			}
			if e = sqlite.Check(ctx, f.store.Read); e != nil {
				t.Fatal(e)
			}
			bank, e := f.ops.Banks(ctx, "2026-10")
			if e != nil {
				t.Fatal(e)
			}
			expected := "-300"
			if kind == "receipt" {
				expected = "300"
			}
			if bank.Accounts[0].Closing != expected {
				t.Fatal(bank)
			}
			archive, _, e := f.store.Backup(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.store.Stage(ctx, archive); e != nil {
				t.Fatal(e)
			}
		})
	}
}
