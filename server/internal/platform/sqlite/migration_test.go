package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyMigrationAndBackupRestore(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dataset := filepath.Join(root, "datasets", "legacy")
	if err := os.MkdirAll(filepath.Join(dataset, "attachments"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := Connect(ctx, filepath.Join(dataset, "accounting.sqlite"), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{schema, "CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,checksum TEXT NOT NULL) STRICT", "INSERT INTO meta VALUES(1,'company','Kiểm thử','legacy-epoch',4,'VND','test')", "INSERT INTO accounts VALUES('111','111','Tên lịch sử','cash',0,1)", "INSERT INTO accounts VALUES('411','411','Vốn lịch sử','capital',0,1)", "INSERT INTO periods VALUES('2026-10',0)", "INSERT INTO documents(id,reference,type,date,description,status,version,source_type,source_id) VALUES('doc','CT-2026-000001','journal','2026-10-01','Giả lập','draft',1,'journal','legacy')", "INSERT INTO document_lines VALUES('doc',0,'111',NULL,'','Tên lịch sử','Thu',9007199254740993,0)", "INSERT INTO document_lines VALUES('doc',1,'411',NULL,'','Vốn lịch sử','Nguồn',0,9007199254740993)", "UPDATE documents SET status='posted' WHERE id='doc'"} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec("INSERT INTO schema_migrations VALUES(1,?)", Hash([]byte(schema))); err != nil {
		t.Fatal(err)
	}
	if err = WriteFile(filepath.Join(root, "active-dataset.json"), []byte(`{"dataset":"legacy"}`)); err != nil {
		t.Fatal(err)
	}
	old := &Store{Root: root, Dataset: dataset, Read: db, Write: db}
	archive, before, err := old.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var version int
	upgraded.Read.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if version != 7 {
		t.Fatal(version)
	}
	var name string
	upgraded.Read.QueryRow("SELECT name FROM accounts WHERE code='111'").Scan(&name)
	if name != "Tên lịch sử" {
		t.Fatal(name)
	}
	var debit int64
	upgraded.Read.QueryRow("SELECT debit FROM document_lines WHERE document_id='doc' AND position=0").Scan(&debit)
	if debit != 9007199254740993 {
		t.Fatal(debit)
	}
	var n int
	upgraded.Read.QueryRow("SELECT count(*) FROM accounts").Scan(&n)
	if n != 184 {
		t.Fatal(n)
	}
	stage, err := upgraded.Stage(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = upgraded.Activate(ctx, stage.ID); err != nil {
		t.Fatal(err)
	}
	after, err := reconcile(ctx, upgraded.Read)
	if err != nil || after.Debit != before.Debit || after.Credit != before.Credit || after.Posted != before.Posted || after.Epoch == before.Epoch {
		t.Fatal(after, err)
	}
	upgraded.Read.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if version != 7 {
		t.Fatal(version)
	}
	if err = Check(ctx, upgraded.Read); err != nil {
		t.Fatal(err)
	}
	if _, err = upgraded.Write.Exec("UPDATE schema_migrations SET checksum='tampered' WHERE version=2"); err != nil {
		t.Fatal(err)
	}
	if err = Check(ctx, upgraded.Read); err == nil {
		t.Fatal("accepted changed migration checksum")
	}
}

func TestFinancialMigrationFromVersionTwo(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dataset := filepath.Join(root, "datasets", "legacy-two")
	if err := os.MkdirAll(filepath.Join(dataset, "attachments"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := Connect(ctx, filepath.Join(dataset, "accounting.sqlite"), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{schema, vouchersSchema, "CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,checksum TEXT NOT NULL) STRICT", "INSERT INTO meta VALUES(1,'company','Kiểm thử','old',0,'VND','test')", "INSERT INTO accounts VALUES('111','111','Tên lịch sử','cash',0,1)", "INSERT INTO accounts VALUES('112','112','Ngân hàng','bank',0,1)", "INSERT INTO bank_accounts VALUES('bank','0000123','Ngân hàng thử','112',1)"} {
		if _, err = db.Exec(source); err != nil {
			t.Fatal(err)
		}
	}
	for i, source := range []string{schema, vouchersSchema} {
		if _, err = db.Exec("INSERT INTO schema_migrations VALUES(?,?)", i+1, migrationChecksum(i+1, source)); err != nil {
			t.Fatal(err)
		}
	}
	if err = WriteFile(filepath.Join(root, "active-dataset.json"), []byte(`{"dataset":"legacy-two"}`)); err != nil {
		t.Fatal(err)
	}
	old := &Store{Root: root, Dataset: dataset, Read: db, Write: db}
	archive, _, err := old.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	upgraded, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var number string
	if err = upgraded.Read.QueryRow("SELECT number FROM bank_accounts WHERE id='bank'").Scan(&number); err != nil || number != "0000123" {
		t.Fatal(number, err)
	}
	var count int
	if err = upgraded.Read.QueryRow("SELECT count(*) FROM financial_reports").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	staged, err := upgraded.Stage(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = upgraded.Activate(ctx, staged.ID); err != nil {
		t.Fatal(err)
	}
	if err = Check(ctx, upgraded.Read); err != nil {
		t.Fatal(err)
	}
}

func TestOpeningMigrationFromVersionThree(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dataset := filepath.Join(root, "datasets", "legacy-three")
	if err := os.MkdirAll(filepath.Join(dataset, "attachments"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := Connect(ctx, filepath.Join(dataset, "accounting.sqlite"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,checksum TEXT NOT NULL) STRICT"); err != nil {
		t.Fatal(err)
	}
	for i, source := range []string{schema, vouchersSchema, financialSchema} {
		if _, err = db.Exec(source); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO schema_migrations VALUES(?,?)", i+1, migrationChecksum(i+1, source)); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{"INSERT INTO meta VALUES(1,'company','Kiểm thử','old',0,'VND','test')", "INSERT INTO users VALUES('owner','owner','Giả lập','owner','test',1)", "INSERT INTO financial_reports VALUES('report',1,'2026-01-01','2026-12-31','2026-10-22T00:00:00Z','owner','{\"id\":\"report\",\"version\":1,\"from\":\"2026-01-01\",\"to\":\"2026-12-31\"}')"} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err = WriteFile(filepath.Join(root, "active-dataset.json"), []byte(`{"dataset":"legacy-three"}`)); err != nil {
		t.Fatal(err)
	}
	old := &Store{Root: root, Dataset: dataset, Read: db, Write: db}
	archive, _, err := old.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	upgraded, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var version, count int
	upgraded.Read.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if version != 7 {
		t.Fatal(version)
	}
	if err = upgraded.Read.QueryRow("SELECT count(*) FROM financial_reports WHERE id='report' AND version=1").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	staged, err := upgraded.Stage(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = upgraded.Activate(ctx, staged.ID); err != nil {
		t.Fatal(err)
	}
	if err = Check(ctx, upgraded.Read); err != nil {
		t.Fatal(err)
	}
}

func TestExcelMigrationPreservesVersionFourAndReopens(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dataset := filepath.Join(root, "datasets", "version-four")
	if err := os.MkdirAll(filepath.Join(dataset, "attachments"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := Connect(ctx, filepath.Join(dataset, "accounting.sqlite"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,checksum TEXT NOT NULL) STRICT"); err != nil {
		t.Fatal(err)
	}
	for i, source := range []string{schema, vouchersSchema, financialSchema, openingSchema} {
		if _, err = db.Exec(source); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO schema_migrations VALUES(?,?)", i+1, migrationChecksum(i+1, source)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec("INSERT INTO meta VALUES(1,'company','Giả lập','before-imports',7,'VND','test')"); err != nil {
		t.Fatal(err)
	}
	before := schemaHash(4)
	if err = Check(ctx, db); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err = WriteFile(filepath.Join(root, "active-dataset.json"), []byte(`{"dataset":"version-four"}`)); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	var version, revision int
	var epoch string
	if err = upgraded.Read.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 7 {
		t.Fatal(version, err)
	}
	if err = upgraded.Read.QueryRow("SELECT revision,epoch FROM meta").Scan(&revision, &epoch); err != nil || revision != 9 || epoch != "before-imports" || before != schemaHash(4) {
		t.Fatal(revision, epoch, err)
	}
	if err = Check(ctx, upgraded.Read); err != nil {
		t.Fatal(err)
	}
	upgraded.Close()
	reopened, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = Check(ctx, reopened.Read); err != nil {
		t.Fatal(err)
	}
}

func TestAccountHierarchyMigrationFromFivePreservesIDsAndNames(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dataset := filepath.Join(root, "datasets", "version-five")
	if err := os.MkdirAll(filepath.Join(dataset, "attachments"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := Connect(ctx, filepath.Join(dataset, "accounting.sqlite"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,checksum TEXT NOT NULL) STRICT"); err != nil {
		t.Fatal(err)
	}
	for i, source := range []string{schema, vouchersSchema, financialSchema, openingSchema, importsSchema} {
		if _, err = db.Exec(source); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO schema_migrations VALUES(?,?)", i+1, migrationChecksum(i+1, source)); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{"INSERT INTO meta VALUES(1,'company','Giả lập','before-hierarchy',7,'VND','test')", "INSERT INTO accounts VALUES('parent-custom-id','112','Tên cha đang dùng','cash',0,1)", "INSERT INTO accounts VALUES('child-custom-id','1121','Tên con đang dùng','cash',0,1)"} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	before := schemaHash(5)
	if err = Check(ctx, db); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err = WriteFile(filepath.Join(root, "active-dataset.json"), []byte(`{"dataset":"version-five"}`)); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var parent, name string
	var revision, version int
	if err = store.Read.QueryRow("SELECT parent_id FROM account_current_parents WHERE account_id='child-custom-id'").Scan(&parent); err != nil || parent != "parent-custom-id" {
		t.Fatal(parent, err)
	}
	if err = store.Read.QueryRow("SELECT name FROM accounts WHERE id='child-custom-id'").Scan(&name); err != nil || name != "Tên con đang dùng" {
		t.Fatal(name, err)
	}
	if err = store.Read.QueryRow("SELECT revision FROM meta").Scan(&revision); err != nil || revision != 9 {
		t.Fatal(revision, err)
	}
	if err = store.Read.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 7 || before != schemaHash(5) {
		t.Fatal(version, err)
	}
	if err = Check(ctx, store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestOpeningAdjustmentMigrationFromSixPreservesLedger(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dataset := filepath.Join(root, "datasets", "version-six")
	if err := os.MkdirAll(filepath.Join(dataset, "attachments"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := Connect(ctx, filepath.Join(dataset, "accounting.sqlite"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,checksum TEXT NOT NULL) STRICT"); err != nil {
		t.Fatal(err)
	}
	for i, source := range []string{schema, vouchersSchema, financialSchema, openingSchema, importsSchema, accountHierarchySchema} {
		if _, err = db.Exec(source); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO schema_migrations VALUES(?,?)", i+1, migrationChecksum(i+1, source)); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{"INSERT INTO meta VALUES(1,'company','Giả lập','before-adjustments',10,'VND','test')", "INSERT INTO accounts VALUES('111','111','Tiền lịch sử','cash',0,1)", "INSERT INTO accounts VALUES('411','411','Vốn lịch sử','capital',0,1)", "INSERT INTO periods VALUES('2026-10',0)", "INSERT INTO documents(id,reference,type,date,description,status,version,source_type,source_id) VALUES('doc','CT-2026-000001','journal','2026-10-01','Giả lập','draft',1,'journal','legacy')", "INSERT INTO document_lines VALUES('doc',0,'111',NULL,'','Tiền lịch sử','Thu',9007199254740993,0)", "INSERT INTO document_lines VALUES('doc',1,'411',NULL,'','Vốn lịch sử','Nguồn',0,9007199254740993)", "UPDATE documents SET status='posted' WHERE id='doc'"} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	before := schemaHash(6)
	if err = Check(ctx, db); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err = WriteFile(filepath.Join(root, "active-dataset.json"), []byte(`{"dataset":"version-six"}`)); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	var revision, version int
	var money, name string
	if err = store.Read.QueryRow("SELECT revision FROM meta").Scan(&revision); err != nil || revision != 11 {
		t.Fatal(revision, err)
	}
	if err = store.Read.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 7 || before != schemaHash(6) {
		t.Fatal(version, err)
	}
	if err = store.Read.QueryRow("SELECT CAST(debit AS TEXT),account_name FROM document_lines WHERE document_id='doc' AND position=0").Scan(&money, &name); err != nil || money != "9007199254740993" || name != "Tiền lịch sử" {
		t.Fatal(money, name, err)
	}
	if err = Check(ctx, store.Read); err != nil {
		t.Fatal(err)
	}
	store.Close()
	reopened, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = Check(ctx, reopened.Read); err != nil {
		t.Fatal(err)
	}
}
