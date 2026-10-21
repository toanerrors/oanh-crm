package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type Store struct {
	Read, Write                  *sql.DB
	Root, Dataset, SQLiteVersion string
	Gate                         sync.RWMutex
	lock                         *os.File
}
type pointer struct {
	Dataset string `json:"dataset"`
}

func ID() string           { return rand.Text() }
func Now() string          { return time.Now().UTC().Format(time.RFC3339Nano) }
func Hash(b []byte) string { n := sha256.Sum256(b); return hex.EncodeToString(n[:]) }

func Open(ctx context.Context, root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	lock, err := lockRoot(filepath.Join(abs, "application.lock"))
	if err != nil {
		return nil, err
	}
	s := &Store{Root: abs, lock: lock}
	if err = s.openActive(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) openActive(ctx context.Context) error {
	var p pointer
	data, err := os.ReadFile(filepath.Join(s.Root, "active-dataset.json"))
	newDataset := os.IsNotExist(err)
	if os.IsNotExist(err) {
		p.Dataset = ID()
		data, _ = json.Marshal(p)
	} else if err != nil {
		return err
	} else if err = json.Unmarshal(data, &p); err != nil {
		return err
	}
	if p.Dataset == "" || strings.ContainsAny(p.Dataset, "/\\.") {
		return fmt.Errorf("invalid dataset pointer")
	}
	s.Dataset = filepath.Join(s.Root, "datasets", p.Dataset)
	if err = os.MkdirAll(filepath.Join(s.Dataset, "attachments"), 0700); err != nil {
		return err
	}
	path := filepath.Join(s.Dataset, "accounting.sqlite")
	if !newDataset {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("active database is missing: %w", err)
		}
	}
	s.Write, err = Connect(ctx, path, false)
	if err != nil {
		return err
	}
	opened := false
	defer func() {
		if !opened {
			if s.Read != nil {
				s.Read.Close()
			}
			if s.Write != nil {
				s.Write.Close()
			}
		}
	}()
	if _, err = s.Write.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return err
	}
	if err = s.Write.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&s.SQLiteVersion); err != nil {
		return err
	}
	var a, b, c int
	if _, err = fmt.Sscanf(s.SQLiteVersion, "%d.%d.%d", &a, &b, &c); err != nil || a < 3 || (a == 3 && (b < 51 || (b == 51 && c < 3))) {
		return fmt.Errorf("SQLite WAL requires version >=3.51.3")
	}
	if err = s.migrate(ctx); err != nil {
		return err
	}
	s.Read, err = Connect(ctx, path, true)
	if err != nil {
		return err
	}
	if err = Check(ctx, s.Read); err != nil {
		return err
	}
	if newDataset {
		if err = WriteFile(filepath.Join(s.Root, "active-dataset.json"), data); err != nil {
			return err
		}
	}
	opened = true
	return nil
}
func Connect(ctx context.Context, path string, readOnly bool) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(FULL)")
	if readOnly {
		q.Set("mode", "ro")
		q.Add("_pragma", "query_only(1)")
	} else {
		q.Set("_txlock", "immediate")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	if readOnly {
		db.SetMaxOpenConns(3)
		db.SetMaxIdleConns(3)
	} else {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.Write.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY,checksum TEXT NOT NULL) STRICT"); err != nil {
		return err
	}
	var checksum string
	var maximum int
	if err := s.Write.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&maximum); err != nil {
		return err
	}
	if maximum > 1 {
		return fmt.Errorf("database schema is newer than this application")
	}
	err := s.Write.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=1").Scan(&checksum)
	if err == nil {
		if checksum != Hash([]byte(schema)) {
			return fmt.Errorf("migration checksum mismatch")
		}
		return Check(ctx, s.Write)
	}
	if err != sql.ErrNoRows {
		return err
	}
	tx, err := s.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, schema); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO meta VALUES(1,?,'',?,0,'VND','')", ID(), ID()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations VALUES(1,?)", Hash([]byte(schema))); err != nil {
		return err
	}
	return tx.Commit()
}
func Check(ctx context.Context, db *sql.DB) error {
	if err := verifySchema(ctx, db); err != nil {
		return err
	}
	if err := verifySubledgers(ctx, db); err != nil {
		return err
	}
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("integrity check failed")
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("foreign key check failed")
	}
	return rows.Err()
}
func (s *Store) Close() error {
	var err error
	if s.Read != nil {
		err = s.Read.Close()
	}
	if s.Write != nil {
		if e := s.Write.Close(); err == nil {
			err = e
		}
	}
	if s.lock != nil {
		s.lock.Close()
	}
	return err
}
func WriteFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".staging-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return SyncDir(filepath.Dir(path))
}
func SyncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
