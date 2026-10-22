package ledger

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"oanh.local/accounting/internal/platform/sqlite"
)

func financialFixture() FinancialInput {
	return FinancialInput{ID: "report-test", Template: "tt99-2026-v1", From: "2026-01-01", To: "2026-12-31", PreviousFrom: "2025-01-01", PreviousTo: "2025-12-31", PreparedDate: "2027-01-10", PreparedBy: "Người lập thử", ChiefAccountant: "Kế toán thử", Director: "Đại diện thử", Address: "Địa chỉ giả lập", Notes: "Thuyết minh giả lập, chưa phải hồ sơ pháp định.", Sheets: []string{"B01-DN", "B02-DN", "B03-DN"}, Values: map[string]map[string]FinancialValue{}}
}
func fillFinancial(input *FinancialInput) {
	for _, sheet := range FinancialForms().Sheets {
		chosen := false
		for _, id := range input.Sheets {
			chosen = chosen || id == sheet.ID
		}
		if !chosen {
			continue
		}
		input.Values[sheet.ID] = map[string]FinancialValue{}
		for _, r := range sheet.Rows {
			if len(r.Formula) == 0 {
				input.Values[sheet.ID][r.Code] = FinancialValue{Current: "0", Previous: "0"}
			}
		}
	}
}
func TestFinancialExactTotalsMissingAndCrossChecks(t *testing.T) {
	input := financialFixture()
	out, err := CalculateFinancial(input)
	if err != nil || out.Computed["B01-DN"]["280"].Current != "" || len(out.Issues) == 0 {
		t.Fatal(out, err)
	}
	fillFinancial(&input)
	input.Values["B02-DN"]["70"] = FinancialValue{Current: "-0.25", Previous: "1234.56"}
	input.Values["B01-DN"]["111"] = FinancialValue{Current: "9007199254740993", Previous: "100"}
	input.Values["B01-DN"]["411a"] = FinancialValue{Current: "9007199254740993", Previous: "100"}
	input.Values["B03-DN"]["60"] = FinancialValue{Current: "100", Previous: "100"}
	input.Values["B03-DN"]["01"] = FinancialValue{Current: "9007199254740893", Previous: "0"}
	input.Values["B02-DN"]["01"] = FinancialValue{Current: "1000", Previous: "0"}
	input.Values["B02-DN"]["02"] = FinancialValue{Current: "100", Previous: "0"}
	input.Values["B02-DN"]["11"] = FinancialValue{Current: "400", Previous: "0"}
	input.Values["B02-DN"]["51"] = FinancialValue{Current: "100", Previous: "0"}
	out, err = CalculateFinancial(input)
	if err != nil || len(out.Issues) != 0 || out.Computed["B01-DN"]["280"].Current != "9007199254740993" || out.Computed["B02-DN"]["60"].Current != "400" {
		t.Fatal(out, err)
	}
	input.Values["B03-DN"]["02"] = FinancialValue{Current: "-1", Previous: "0"}
	out, err = CalculateFinancial(input)
	if err != nil || len(out.Issues) != 1 || !strings.Contains(out.Issues[0].Message, "Tiền cuối kỳ") {
		t.Fatal(out, err)
	}
	input.Values["B01-DN"]["280"] = FinancialValue{Current: "1"}
	if _, err = CalculateFinancial(input); err == nil {
		t.Fatal("accepted derived amount")
	}
	delete(input.Values["B01-DN"], "280")
	input.Values["B02-DN"]["01"] = FinancialValue{Current: "1e9"}
	if _, err = CalculateFinancial(input); err == nil {
		t.Fatal("accepted noncanonical money")
	}
	input.From = "2025-01-01"
	if _, err = CalculateFinancial(input); err == nil {
		t.Fatal("accepted pre-TT99 year")
	}
}
func TestFinancialSaveRetryConcurrencyReopenAndBackup(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := sqlite.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"INSERT INTO users VALUES('owner','test','Kiểm thử','owner','unused',1)", "INSERT INTO users VALUES('viewer','read','Xem','viewer','unused',1)"} {
		if _, err = store.Write.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	var epoch string
	if err = store.Read.QueryRow("SELECT epoch FROM meta").Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: store}
	input := financialFixture()
	cmd := Command{Actor: "owner", Epoch: epoch, Key: "financial-save"}
	b, err := s.SaveFinancial(ctx, cmd, input)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.SaveFinancial(ctx, cmd, input)
	if err != nil || string(b) != string(retry) {
		t.Fatal(err)
	}
	var out FinancialReport
	if err = json.Unmarshal(b, &out); err != nil || out.Input.Version != 1 {
		t.Fatal(out, err)
	}
	input.Version = 1
	input.Director = "Thay đổi giả lập"
	if _, err = s.SaveFinancial(ctx, cmd, input); err == nil {
		t.Fatal("accepted key payload mismatch")
	}
	if _, err = s.SaveFinancial(ctx, Command{Actor: "viewer", Epoch: epoch, Key: "no-write"}, input); err == nil {
		t.Fatal("viewer wrote report")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, key := range []string{"concurrent-a", "concurrent-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, e := s.SaveFinancial(ctx, Command{Actor: "owner", Epoch: epoch, Key: key}, input)
			results <- e
		}(key)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	if _, err = store.Write.Exec("CREATE TRIGGER fail_financial_audit BEFORE INSERT ON audit WHEN NEW.action='financial.save' BEGIN SELECT RAISE(ABORT,'test rollback'); END"); err != nil {
		t.Fatal(err)
	}
	input.Version = 2
	if _, err = s.SaveFinancial(ctx, Command{Actor: "owner", Epoch: epoch, Key: "rollback"}, input); err == nil {
		t.Fatal("expected rollback")
	}
	var storedVersion int
	store.Read.QueryRow("SELECT version FROM financial_reports WHERE id=?", input.ID).Scan(&storedVersion)
	if storedVersion != 2 {
		t.Fatal(storedVersion)
	}
	store.Write.Exec("DROP TRIGGER fail_financial_audit")
	var versions int
	if err = store.Read.QueryRow("SELECT count(*) FROM commands WHERE operation='financial.save'").Scan(&versions); err != nil || versions != 2 {
		t.Fatal(versions, err)
	}
	archive, _, err := store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = sqlite.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s.Store = store
	reports, err := s.FinancialReports(ctx, "2026", "")
	if err != nil || len(reports) != 1 || reports[0].Input.Version != 2 {
		t.Fatal(reports, err)
	}
	staged, err := store.Stage(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Activate(ctx, staged.ID); err != nil {
		t.Fatal(err)
	}
	reports, err = s.FinancialReports(ctx, "2026", "")
	if err != nil || len(reports) != 1 || reports[0].Input.Director != "Thay đổi giả lập" {
		t.Fatal(reports, err)
	}
	if _, err = s.SaveFinancial(ctx, Command{Actor: "owner", Epoch: epoch, Key: "old-epoch"}, reports[0].Input); err == nil {
		t.Fatal("accepted old epoch")
	}
}
func TestFinancialSourceRangeAndPartnerSides(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, q := range []string{
		"INSERT INTO periods VALUES('2025-12',0)", "INSERT INTO periods VALUES('2026-01',0)", "INSERT INTO periods VALUES('2027-01',0)",
		"INSERT INTO partners VALUES('a','A','Khách A','customer','','',1)", "INSERT INTO partners VALUES('b','B','Khách B','customer','','',1)",
		"INSERT INTO documents(id,reference,type,date,description,status,version,source_type,source_id) VALUES('before','CT-1','journal','2025-12-31','Giả lập','draft',1,'journal','before')",
		"INSERT INTO document_lines VALUES('before',0,'131','a','A','Phải thu','',100,0)", "INSERT INTO document_lines VALUES('before',1,'131','b','B','Phải thu','',0,40)", "INSERT INTO document_lines VALUES('before',2,'411',NULL,'','Vốn','',0,60)", "UPDATE documents SET status='posted' WHERE id='before'",
		"INSERT INTO documents(id,reference,type,date,description,status,version,source_type,source_id) VALUES('now','CT-2','journal','2026-01-01','Giả lập','draft',1,'journal','now')",
		"INSERT INTO document_lines VALUES('now',0,'131','a','A','Phải thu','',0,20)", "INSERT INTO document_lines VALUES('now',1,'131','b','B','Phải thu','',10,0)", "INSERT INTO document_lines VALUES('now',2,'111',NULL,'','Tiền','',10,0)", "UPDATE documents SET status='posted' WHERE id='now'",
		"INSERT INTO documents(id,reference,type,date,description,status,version,source_type,source_id) VALUES('after','CT-3','journal','2027-01-01','Giả lập','draft',1,'journal','after')", "INSERT INTO document_lines VALUES('after',0,'111',NULL,'','Tiền','',1000,0)", "INSERT INTO document_lines VALUES('after',1,'411',NULL,'','Vốn','',0,1000)", "UPDATE documents SET status='posted' WHERE id='after'",
	} {
		if _, err = store.Write.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	out, err := (&Service{Store: store}).FinancialSource(ctx, "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range out.Rows {
		if row.Code == "131" && (row.OpeningDebit != "100" || row.OpeningCredit != "40" || row.ClosingDebit != "80" || row.ClosingCredit != "30" || row.Debit != "10" || row.Credit != "20") {
			t.Fatal(row)
		}
		if row.Code == "111" && row.Debit != "10" {
			t.Fatal(row)
		}
	}
}
