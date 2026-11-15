package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type migration struct {
	version                int
	name, script, checksum string
}

var migrationName = regexp.MustCompile(`^([0-9]{3,})_[a-z0-9_]+\.sql$`)
var schemaToken = regexp.MustCompile(`'(?:''|[^'])*'|"(?:""|[^"])*"|[A-Za-z_][A-Za-z0-9_]*|[0-9]+|[^\s]`)

func loadMigrations(source fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(source, "migrations")
	if err != nil {
		return nil, err
	}
	var result []migration
	for _, entry := range entries {
		match := migrationName.FindStringSubmatch(entry.Name())
		if entry.IsDir() || match == nil {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("invalid migration version %q: %w", entry.Name(), err)
		}
		script, err := fs.ReadFile(source, "migrations/"+entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, migration{version, entry.Name(), string(script), fmt.Sprintf("%x", sha256.Sum256(script))})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].version < result[j].version })
	if len(result) == 0 {
		return nil, fmt.Errorf("no migrations found")
	}
	for i, item := range result {
		if item.version != i+1 {
			return nil, fmt.Errorf("migration versions must be consecutive from 1: got %d at position %d", item.version, i+1)
		}
	}
	return result, nil
}

func applyMigrations(ctx context.Context, db *sql.DB, source fs.FS) error {
	items, err := loadMigrations(source)
	if err != nil {
		return err
	}
	for {
		done, err := applyNextMigration(ctx, db, items)
		if err != nil || done {
			return err
		}
	}
}

func applyNextMigration(ctx context.Context, db *sql.DB, items []migration) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// Metadata and the first migration share a transaction, including on legacy databases.
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER PRIMARY KEY,
        name TEXT NOT NULL,
        checksum TEXT NOT NULL,
        applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
    )`); err != nil {
		return false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT version,name,checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return false, err
	}
	applied := 0
	for rows.Next() {
		var version int
		var name, checksum string
		if err = rows.Scan(&version, &name, &checksum); err != nil {
			break
		}
		if applied >= len(items) || version != applied+1 || name != items[applied].name || checksum != items[applied].checksum {
			err = fmt.Errorf("unrecognized migration history at version %d", version)
			break
		}
		applied++
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return false, err
	}
	if applied == len(items) {
		return true, tx.Commit()
	}
	if applied == 0 {
		if err = validateLegacySchema(ctx, tx, items[0].script); err != nil {
			return false, err
		}
	}
	next := items[applied]
	if _, err = tx.ExecContext(ctx, next.script); err != nil {
		return false, fmt.Errorf("migration %s: %w", next.name, err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum) VALUES(?,?,?)`, next.version, next.name, next.checksum); err != nil {
		return false, fmt.Errorf("record migration %s: %w", next.name, err)
	}
	return false, tx.Commit()
}

type schemaReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func schemaDefinition(ctx context.Context, db schemaReader) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT type,name,sql FROM sqlite_schema
        WHERE name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations' ORDER BY type,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var kind, name, definition string
		if err = rows.Scan(&kind, &name, &definition); err != nil {
			return nil, err
		}
		result = append(result, kind+":"+name+":"+strings.Join(schemaToken.FindAllString(definition, -1), "\x00"))
	}
	return result, rows.Err()
}

func validateLegacySchema(ctx context.Context, tx *sql.Tx, initial string) error {
	actual, err := schemaDefinition(ctx, tx)
	if err != nil || len(actual) == 0 {
		return err
	}
	// Compare with a disposable database built from the immutable initial migration.
	// Partial, modified and unrelated schemas must not be silently adopted.
	reference, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer reference.Close()
	reference.SetMaxOpenConns(1)
	if _, err = reference.ExecContext(ctx, initial); err != nil {
		return err
	}
	expected, err := schemaDefinition(ctx, reference)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("unrecognized legacy database schema; refusing to migrate")
	}
	return nil
}
