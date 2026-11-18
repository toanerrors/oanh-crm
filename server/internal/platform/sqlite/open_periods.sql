DROP TRIGGER document_posting;
CREATE TRIGGER document_posting BEFORE UPDATE OF status ON documents WHEN NEW.status='posted' BEGIN
 SELECT CASE WHEN (SELECT COUNT(*) FROM document_lines WHERE document_id=NEW.id)<2 THEN RAISE(ABORT,'entry needs two lines') END;
 SELECT CASE WHEN (SELECT COALESCE(SUM(debit),0) FROM document_lines WHERE document_id=NEW.id)!=(SELECT COALESCE(SUM(credit),0) FROM document_lines WHERE document_id=NEW.id) THEN RAISE(ABORT,'entry is unbalanced') END;
 SELECT CASE WHEN EXISTS(SELECT 1 FROM periods WHERE period=substr(NEW.date,1,7) AND closed=1) THEN RAISE(ABORT,'period is closed') END;
 SELECT CASE WHEN NEW.source_type<>'opening_adjustment_reversal' AND EXISTS(SELECT 1 FROM document_lines l JOIN accounts a ON a.id=l.account_id LEFT JOIN partners p ON p.id=l.partner_id WHERE l.document_id=NEW.id AND (a.active=0 OR l.debit+l.credit=0 OR (a.requires_partner=1 AND (p.id IS NULL OR p.active=0)))) THEN RAISE(ABORT,'invalid account or partner') END;
END;
