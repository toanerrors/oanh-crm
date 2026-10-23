CREATE TABLE opening_adjustments (
 document_id TEXT PRIMARY KEY REFERENCES documents(id),
 opening_document_id TEXT NOT NULL REFERENCES documents(id),
 before_document_id TEXT NOT NULL REFERENCES documents(id),
 reversal_document_id TEXT UNIQUE REFERENCES documents(id),
 base_version INTEGER NOT NULL CHECK(base_version>=0),
 confirmed_version INTEGER UNIQUE CHECK(confirmed_version IS NULL OR confirmed_version=base_version+1),
 reason TEXT NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 1000),
 before_payload TEXT NOT NULL CHECK(json_valid(before_payload)),
 after_payload TEXT NOT NULL CHECK(json_valid(after_payload)),
 created_by TEXT NOT NULL REFERENCES users(id), created_at TEXT NOT NULL,
 confirmed_by TEXT REFERENCES users(id), confirmed_at TEXT,
 CHECK((confirmed_version IS NULL AND confirmed_by IS NULL AND confirmed_at IS NULL) OR (confirmed_version IS NOT NULL AND reversal_document_id IS NOT NULL AND confirmed_by IS NOT NULL AND confirmed_at IS NOT NULL))
) STRICT;
CREATE UNIQUE INDEX opening_adjustment_draft ON opening_adjustments(opening_document_id) WHERE confirmed_version IS NULL;
CREATE TRIGGER opening_adjustment_update BEFORE UPDATE ON opening_adjustments WHEN OLD.confirmed_version IS NOT NULL BEGIN SELECT RAISE(ABORT,'confirmed opening adjustment is immutable'); END;
CREATE TRIGGER opening_adjustment_delete BEFORE DELETE ON opening_adjustments WHEN OLD.confirmed_version IS NOT NULL BEGIN SELECT RAISE(ABORT,'confirmed opening adjustment is immutable'); END;
CREATE TRIGGER opening_adjustment_version BEFORE UPDATE OF confirmed_version ON opening_adjustments WHEN NEW.confirmed_version IS NOT NULL BEGIN
 SELECT CASE WHEN NEW.base_version<>COALESCE((SELECT MAX(confirmed_version) FROM opening_adjustments WHERE document_id<>NEW.document_id),0) OR NOT EXISTS(SELECT 1 FROM documents r WHERE r.id=NEW.reversal_document_id AND r.status='posted' AND r.source_type='opening_adjustment_reversal' AND r.reversal_of=NEW.before_document_id) THEN RAISE(ABORT,'opening adjustment version or reversal mismatch') END;
END;
CREATE TRIGGER opening_adjustment_post BEFORE UPDATE OF status ON documents WHEN NEW.status='posted' AND NEW.source_type='opening_adjustment' BEGIN
 SELECT CASE WHEN NOT EXISTS(SELECT 1 FROM opening_adjustments a JOIN documents r ON r.id=a.opening_document_id WHERE a.document_id=NEW.id AND a.confirmed_version IS NOT NULL AND r.status='posted' AND r.source_type='opening' AND NEW.date=json_extract(a.after_payload,'$.cutoff') AND NEW.id=json_extract(a.after_payload,'$.id')) THEN RAISE(ABORT,'opening adjustment requires confirmed metadata') END;
END;
CREATE VIEW opening_current AS SELECT o.document_id AS opening_document_id, COALESCE((SELECT a.document_id FROM opening_adjustments a JOIN documents d ON d.id=a.document_id WHERE a.confirmed_version IS NOT NULL AND d.status='posted' ORDER BY a.confirmed_version DESC LIMIT 1),o.document_id) AS document_id, COALESCE((SELECT a.after_payload FROM opening_adjustments a JOIN documents d ON d.id=a.document_id WHERE a.confirmed_version IS NOT NULL AND d.status='posted' ORDER BY a.confirmed_version DESC LIMIT 1),o.payload) AS payload FROM opening_balances o;
CREATE VIEW current_assets AS SELECT a.* FROM assets a JOIN documents d ON d.id=a.recognized_document WHERE d.source_type NOT IN ('opening','opening_adjustment') OR EXISTS(SELECT 1 FROM opening_current o,json_each(o.payload,'$.assets') j WHERE json_extract(j.value,'$.id')=a.id);

CREATE TRIGGER opening_reversal_post BEFORE UPDATE OF status ON documents WHEN NEW.status='posted' AND NEW.source_type='opening_adjustment_reversal' BEGIN
 SELECT CASE WHEN NOT EXISTS(SELECT 1 FROM documents d WHERE d.id=NEW.reversal_of AND d.status='posted' AND d.source_type IN ('opening','opening_adjustment') AND NEW.date=d.date AND NEW.source_id=d.id) THEN RAISE(ABORT,'invalid opening reversal source') END;
 SELECT CASE WHEN (SELECT count(*) FROM document_lines WHERE document_id=NEW.id)<>(SELECT count(*) FROM document_lines WHERE document_id=NEW.reversal_of) OR EXISTS(SELECT 1 FROM document_lines l LEFT JOIN document_lines r ON r.document_id=NEW.id AND r.position=l.position WHERE l.document_id=NEW.reversal_of AND (r.position IS NULL OR r.account_id<>l.account_id OR COALESCE(r.partner_id,'')<>COALESCE(l.partner_id,'') OR r.debit<>l.credit OR r.credit<>l.debit OR r.partner_name<>l.partner_name OR r.account_name<>l.account_name OR r.description<>l.description)) THEN RAISE(ABORT,'opening reversal must copy original lines') END;
END;
DROP TRIGGER document_posting;
CREATE TRIGGER document_posting BEFORE UPDATE OF status ON documents WHEN NEW.status='posted' BEGIN
 SELECT CASE WHEN (SELECT COUNT(*) FROM document_lines WHERE document_id=NEW.id)<2 THEN RAISE(ABORT,'entry needs two lines') END;
 SELECT CASE WHEN (SELECT COALESCE(SUM(debit),0) FROM document_lines WHERE document_id=NEW.id)!=(SELECT COALESCE(SUM(credit),0) FROM document_lines WHERE document_id=NEW.id) THEN RAISE(ABORT,'entry is unbalanced') END;
 SELECT CASE WHEN NOT EXISTS(SELECT 1 FROM periods WHERE period=substr(NEW.date,1,7) AND closed=0) THEN RAISE(ABORT,'period is not open') END;
 SELECT CASE WHEN NEW.source_type<>'opening_adjustment_reversal' AND EXISTS(SELECT 1 FROM document_lines l JOIN accounts a ON a.id=l.account_id LEFT JOIN partners p ON p.id=l.partner_id WHERE l.document_id=NEW.id AND (a.active=0 OR l.debit+l.credit=0 OR (a.requires_partner=1 AND (p.id IS NULL OR p.active=0)))) THEN RAISE(ABORT,'invalid account or partner') END;
END;

UPDATE meta SET revision=revision+1 WHERE id=1;
