CREATE TABLE account_hierarchy (
 account_id TEXT NOT NULL REFERENCES accounts(id),
 revision INTEGER NOT NULL CHECK(revision>=0),
 parent_id TEXT REFERENCES accounts(id),
 CHECK(parent_id IS NULL OR parent_id<>account_id),
 PRIMARY KEY(account_id,revision)
) STRICT;
CREATE INDEX account_hierarchy_parent ON account_hierarchy(parent_id,account_id);
CREATE VIEW account_current_parents AS
 SELECT h.account_id,h.parent_id,h.revision FROM account_hierarchy h
 WHERE h.revision=(SELECT MAX(v.revision) FROM account_hierarchy v WHERE v.account_id=h.account_id);
CREATE TRIGGER account_hierarchy_cycle BEFORE INSERT ON account_hierarchy WHEN NEW.parent_id IS NOT NULL BEGIN
 SELECT CASE WHEN EXISTS(
  WITH RECURSIVE ancestors(id) AS (
   SELECT NEW.parent_id UNION SELECT p.parent_id FROM account_current_parents p JOIN ancestors a ON p.account_id=a.id WHERE p.parent_id IS NOT NULL
  ) SELECT 1 FROM ancestors WHERE id=NEW.account_id
 ) THEN RAISE(ABORT,'account hierarchy cycle') END;
END;
CREATE TRIGGER account_hierarchy_immutable_update BEFORE UPDATE ON account_hierarchy BEGIN SELECT RAISE(ABORT,'account hierarchy history is immutable'); END;
CREATE TRIGGER account_hierarchy_immutable_delete BEFORE DELETE ON account_hierarchy BEGIN SELECT RAISE(ABORT,'account hierarchy history is immutable'); END;
CREATE TRIGGER account_hierarchy_order BEFORE INSERT ON account_hierarchy WHEN EXISTS(SELECT 1 FROM account_hierarchy WHERE account_id=NEW.account_id AND revision>=NEW.revision) BEGIN SELECT RAISE(ABORT,'account hierarchy revision must increase'); END;
