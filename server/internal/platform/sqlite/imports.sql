CREATE TABLE excel_imports (
 id TEXT PRIMARY KEY,
 actor_id TEXT NOT NULL REFERENCES users(id),
 epoch TEXT NOT NULL,
 owner_only INTEGER NOT NULL CHECK(owner_only IN (0,1)),
 filename TEXT NOT NULL CHECK(length(filename) BETWEEN 1 AND 180),
 file_hash TEXT NOT NULL CHECK(length(file_hash)=64),
 workbook TEXT NOT NULL CHECK(json_valid(workbook) AND length(workbook)<=4194304),
 state TEXT NOT NULL CHECK(state IN ('staged','imported')),
 configuration TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(configuration)),
 fingerprint TEXT,
 result TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(result)),
 created_at TEXT NOT NULL,
 imported_at TEXT
) STRICT;
CREATE INDEX excel_imports_actor ON excel_imports(actor_id,created_at);
CREATE UNIQUE INDEX excel_imports_source ON excel_imports(fingerprint) WHERE state='imported';
