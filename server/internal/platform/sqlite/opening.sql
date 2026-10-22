CREATE TABLE opening_balances (id INTEGER PRIMARY KEY CHECK(id=1), document_id TEXT NOT NULL UNIQUE REFERENCES documents(id), cutoff TEXT NOT NULL CHECK(length(cutoff)=10 AND substr(cutoff,8,3)='-01'), source TEXT NOT NULL CHECK(length(trim(source)) BETWEEN 1 AND 1000), payload TEXT NOT NULL CHECK(json_valid(payload))) STRICT;
CREATE TABLE opening_assets (asset_id TEXT PRIMARY KEY REFERENCES assets(id), elapsed INTEGER NOT NULL CHECK(elapsed>=0), allocated INTEGER NOT NULL CHECK(allocated>=0)) STRICT;
CREATE TRIGGER opening_update BEFORE UPDATE ON opening_balances WHEN (SELECT status FROM documents WHERE id=OLD.document_id)='posted' BEGIN SELECT RAISE(ABORT,'opening balance is immutable'); END;
CREATE TRIGGER opening_delete BEFORE DELETE ON opening_balances WHEN (SELECT status FROM documents WHERE id=OLD.document_id)='posted' BEGIN SELECT RAISE(ABORT,'opening balance cannot be deleted'); END;
CREATE TRIGGER opening_asset_update BEFORE UPDATE ON opening_assets BEGIN SELECT RAISE(ABORT,'opening asset is immutable'); END;
CREATE TRIGGER opening_asset_delete BEFORE DELETE ON opening_assets BEGIN SELECT RAISE(ABORT,'opening asset is immutable'); END;
CREATE TRIGGER opening_source_update BEFORE UPDATE ON documents WHEN OLD.source_type='opening' AND (NEW.source_type<>'opening' OR NEW.source_id<>OLD.source_id) BEGIN SELECT RAISE(ABORT,'opening source is immutable'); END;
CREATE TRIGGER opening_cutoff_post BEFORE UPDATE OF status ON documents WHEN NEW.status='posted' BEGIN
 SELECT CASE WHEN NEW.source_type='opening' AND (NOT EXISTS(SELECT 1 FROM opening_balances WHERE document_id=NEW.id AND cutoff=NEW.date) OR EXISTS(SELECT 1 FROM documents WHERE status='posted')) THEN RAISE(ABORT,'opening requires an empty posted ledger') END;
 SELECT CASE WHEN NEW.source_type<>'opening' AND EXISTS(SELECT 1 FROM opening_balances o JOIN documents d ON d.id=o.document_id WHERE d.status='posted' AND NEW.date<o.cutoff) THEN RAISE(ABORT,'document precedes conversion cutoff') END;
END;
