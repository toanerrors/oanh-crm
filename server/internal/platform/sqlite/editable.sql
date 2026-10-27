DROP TRIGGER posted_document_update;
CREATE TRIGGER posted_document_update BEFORE UPDATE ON documents WHEN OLD.status='posted' AND NOT (NEW.status='draft' AND EXISTS(SELECT 1 FROM audit_context WHERE id=1)) BEGIN SELECT RAISE(ABORT,'posted document is immutable'); END;
