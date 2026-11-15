package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func migrationDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func execSQL(t *testing.T, db *sql.DB, script string) {
	t.Helper()
	if _, err := db.Exec(script); err != nil {
		t.Fatal(err)
	}
}

func snapshot(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result []string
	for rows.Next() {
		values := make([]any, len(columns))
		args := make([]any, len(columns))
		for i := range values {
			args[i] = &values[i]
		}
		if err := rows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		result = append(result, fmt.Sprintf("%#v", values))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestLegacyMigrationPreservesAllUserDataAndReopenIsStable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy := migrationDB(t, path)
	initial, err := fs.ReadFile(migrations, "migrations/001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, legacy, string(initial))
	execSQL(t, legacy, `
INSERT INTO users VALUES
 ('11111111-1111-1111-1111-111111111111','legacy','2026-07-10T09:00:00Z'),
 ('22222222-2222-2222-2222-222222222222','other','2026-07-10T09:00:00Z'),
 ('33333333-3333-3333-3333-333333333333','custom','2026-07-10T09:00:00Z');
INSERT INTO settings VALUES ('11111111-1111-1111-1111-111111111111',12345,'CUSTOM_DATE','2026-08-10','RUB','2026-07-10T09:00:00Z');
INSERT INTO categories VALUES
 ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa','11111111-1111-1111-1111-111111111111','Зарплата','INCOME',1,'2026-07-10T09:00:00Z'),
 ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb','22222222-2222-2222-2222-222222222222','Зарплата','INCOME',1,'2026-07-10T09:00:00Z'),
 ('cccccccc-cccc-cccc-cccc-cccccccccccc','33333333-3333-3333-3333-333333333333','Зарплата','INCOME',0,'2026-07-10T09:00:00Z'),
 ('dddddddd-dddd-dddd-dddd-dddddddddddd','11111111-1111-1111-1111-111111111111','Зарплата','EXPENSE',1,'2026-07-10T09:00:00Z');
INSERT INTO anchors VALUES ('anchor','11111111-1111-1111-1111-111111111111',987654,'2026-07-10T09:00:00Z','2026-07-10T09:00:00Z');
INSERT INTO financial_events VALUES ('event','11111111-1111-1111-1111-111111111111','INCOME','REQUIRED',100001,'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa','legacy income','MONTHLY_ON_DAY','2026-07-10',10,NULL,'2027-07-10',NULL,'2026-07-10T09:00:00Z','2026-07-10T09:00:00Z');
INSERT INTO day_cost_overrides VALUES ('day','11111111-1111-1111-1111-111111111111','2026-07-10','2026-07-15',987,'holiday','2026-07-10T09:00:00Z');
INSERT INTO event_occurrence_overrides VALUES ('occurrence','event','2026-08-10',777,'exception',0,'2026-07-10T09:00:00Z');
INSERT INTO actual_transactions VALUES ('actual','11111111-1111-1111-1111-111111111111','INCOME',123456,'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa','2026-07-10','received','event','2026-07-10T09:00:00Z');
`)
	queries := []string{
		`SELECT * FROM users ORDER BY id`, `SELECT user_id,base_day_cost,horizon_policy,horizon_custom_date,currency,updated_at FROM settings ORDER BY user_id`,
		`SELECT id,user_id,name,kind,is_default,created_at FROM categories ORDER BY id`,
		`SELECT id,user_id,amount,anchored_at,created_at FROM anchors ORDER BY id`, `SELECT id,user_id,kind,necessity,amount,category_id,comment,recurrence_type,start_date,day_of_month,interval_days,until_date,cron_expr,created_at,updated_at FROM financial_events ORDER BY id`,
		`SELECT * FROM day_cost_overrides ORDER BY id`, `SELECT * FROM event_occurrence_overrides ORDER BY id`,
		`SELECT * FROM actual_transactions ORDER BY id`,
	}
	before := make([][]string, len(queries))
	for i, query := range queries {
		before[i] = snapshot(t, legacy, query)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	for i, query := range queries {
		if got := snapshot(t, store.db, query); !reflect.DeepEqual(got, before[i]) {
			t.Fatalf("migration changed data for %s: before=%v after=%v", query, before[i], got)
		}
	}
	codes := snapshot(t, store.db, `SELECT id,system_code FROM categories ORDER BY id`)
	if len(codes) != 4 || !strings.Contains(codes[0], `"SALARY"`) || !strings.Contains(codes[1], `"SALARY"`) ||
		!strings.Contains(codes[2], `(nil)`) || !strings.Contains(codes[3], `(nil)`) {
		t.Fatalf("unexpected legacy category codes: %v", codes)
	}
	// Once assigned, identity survives a display-name change and another Open.
	execSQL(t, store.db, `UPDATE categories SET name='Оклад' WHERE id='aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'`)
	queries = append(queries, `SELECT * FROM categories ORDER BY id`, `SELECT * FROM schema_migrations ORDER BY version`)
	stable := make([][]string, len(queries))
	for i, query := range queries {
		stable[i] = snapshot(t, store.db, query)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for i, query := range queries {
		if got := snapshot(t, store.db, query); !reflect.DeepEqual(got, stable[i]) {
			t.Fatalf("reopen changed %s: before=%v after=%v", query, stable[i], got)
		}
	}
	user, err := store.ProvisionUser(ctx, "LEGACY")
	if err != nil || user.Login != "legacy" {
		t.Fatalf("existing login: %+v, %v", user, err)
	}
	categories, err := store.ListCategories(ctx, user.ID)
	if err != nil || len(categories) != 2 {
		t.Fatalf("categories after migration: %+v, %v", categories, err)
	}
	for _, category := range categories {
		if category.Name == "Оклад" && (category.SystemCode == nil || *category.SystemCode != "SALARY") {
			t.Fatalf("salary identity lost: %+v", category)
		}
	}
	if got := snapshot(t, store.db, `PRAGMA foreign_key_check`); len(got) != 0 {
		t.Fatalf("broken references: %v", got)
	}
}

func TestMigrationsRollbackFailedStepAndResume(t *testing.T) {
	db := migrationDB(t, ":memory:")
	source := fstest.MapFS{
		"migrations/001_initial.sql": {Data: []byte(`CREATE TABLE ledger(id INTEGER PRIMARY KEY, amount INTEGER); INSERT INTO ledger VALUES(1,100);`)},
		"migrations/002_change.sql":  {Data: []byte(`ALTER TABLE ledger ADD COLUMN note TEXT; UPDATE ledger SET amount=200; CREATE TABLE partial(id INTEGER); INSERT INTO missing_table VALUES(1);`)},
	}
	if err := applyMigrations(context.Background(), db, source); err == nil || !strings.Contains(err.Error(), "002_change.sql") {
		t.Fatalf("expected failing migration: %v", err)
	}
	if got := snapshot(t, db, `SELECT * FROM ledger`); !reflect.DeepEqual(got, []string{`[]interface {}{1, 100}`}) {
		t.Fatalf("failed migration persisted data/column changes: %v", got)
	}
	if got := snapshot(t, db, `SELECT name FROM sqlite_schema WHERE name='partial'`); len(got) != 0 {
		t.Fatalf("failed DDL persisted: %v", got)
	}
	if got := snapshot(t, db, `SELECT version FROM schema_migrations`); !reflect.DeepEqual(got, []string{`[]interface {}{1}`}) {
		t.Fatalf("failed migration marked applied or earlier step rolled back: %v", got)
	}
	source["migrations/002_change.sql"].Data = []byte(`ALTER TABLE ledger ADD COLUMN note TEXT; UPDATE ledger SET amount=200,note='done';`)
	if err := applyMigrations(context.Background(), db, source); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, db, `SELECT * FROM ledger`); !reflect.DeepEqual(got, []string{`[]interface {}{1, 200, "done"}`}) {
		t.Fatalf("retry did not apply corrected step: %v", got)
	}
	before := snapshot(t, db, `SELECT * FROM schema_migrations ORDER BY version`)
	if err := applyMigrations(context.Background(), db, source); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, db, `SELECT * FROM schema_migrations ORDER BY version`); !reflect.DeepEqual(got, before) {
		t.Fatalf("reopen rewrote journal: before=%v after=%v", before, got)
	}
}

func TestInitialMigrationFailureLeavesNoSchema(t *testing.T) {
	db := migrationDB(t, ":memory:")
	source := fstest.MapFS{"migrations/001_initial.sql": {Data: []byte(`CREATE TABLE partial(id INTEGER); INSERT INTO missing VALUES(1);`)}}
	if err := applyMigrations(context.Background(), db, source); err == nil {
		t.Fatal("expected failure")
	}
	if got := snapshot(t, db, `SELECT name FROM sqlite_schema`); len(got) != 0 {
		t.Fatalf("initial failure left metadata or DDL: %v", got)
	}
}

func TestUnknownLegacySchemaIsRejectedWithoutMutation(t *testing.T) {
	for _, setup := range []string{
		`CREATE TABLE users(id TEXT PRIMARY KEY); INSERT INTO users VALUES('keep');`,
		`CREATE TABLE unrelated(value TEXT); INSERT INTO unrelated VALUES('keep');`,
		`CREATE TABLE sqlitex_backup(value TEXT); INSERT INTO sqlitex_backup VALUES('keep');`,
	} {
		t.Run(setup, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unknown.db")
			db := migrationDB(t, path)
			execSQL(t, db, setup)
			before := snapshot(t, db, `SELECT type,name,sql FROM sqlite_schema ORDER BY name`)
			store, err := Open(path)
			if err == nil {
				store.Close()
				t.Fatal("unknown schema accepted")
			}
			if !strings.Contains(err.Error(), "unrecognized legacy") {
				t.Fatal(err)
			}
			if got := snapshot(t, db, `SELECT type,name,sql FROM sqlite_schema ORDER BY name`); !reflect.DeepEqual(got, before) {
				t.Fatalf("rejected schema was modified: before=%v after=%v", before, got)
			}
		})
	}
}

func TestMigrationHistoryMustMatchBinary(t *testing.T) {
	for _, corrupt := range []string{
		`UPDATE schema_migrations SET checksum='modified' WHERE version=1`,
		`UPDATE schema_migrations SET name='renamed.sql' WHERE version=1`,
		`DELETE FROM schema_migrations WHERE version=1`,
		`INSERT INTO schema_migrations(version,name,checksum) VALUES(999,'999_future.sql','unknown')`,
	} {
		t.Run(corrupt, func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "history.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			execSQL(t, store.db, corrupt)
			before := snapshot(t, store.db, `SELECT * FROM schema_migrations ORDER BY version`)
			if err := applyMigrations(context.Background(), store.db, migrations); err == nil || !strings.Contains(err.Error(), "history") {
				t.Fatalf("expected history rejection: %v", err)
			}
			if got := snapshot(t, store.db, `SELECT * FROM schema_migrations ORDER BY version`); !reflect.DeepEqual(got, before) {
				t.Fatalf("history modified on rejection: before=%v after=%v", before, got)
			}
		})
	}
}

func TestMigrationFilesMustBeConsecutive(t *testing.T) {
	for _, names := range [][]string{
		{}, {"002_gap.sql"}, {"001_initial.sql", "003_gap.sql"},
		{"001_initial.sql", "001_duplicate.sql"}, {"invalid.sql"},
	} {
		t.Run(fmt.Sprint(names), func(t *testing.T) {
			source := fstest.MapFS{}
			for _, name := range names {
				source["migrations/"+name] = &fstest.MapFile{Data: []byte(`CREATE TABLE ignored(id INTEGER);`)}
			}
			db := migrationDB(t, ":memory:")
			if err := applyMigrations(context.Background(), db, source); err == nil {
				t.Fatal("invalid sequence accepted")
			}
			if got := snapshot(t, db, `SELECT name FROM sqlite_schema`); len(got) != 0 {
				t.Fatalf("invalid sequence modified schema: %v", got)
			}
		})
	}
}

func TestMigrationChecksumIsPortableAcrossLineEndings(t *testing.T) {
	path := filepath.Join(t.TempDir(),"portable.db")
	lf:=fstest.MapFS{
		"migrations/001_initial.sql":{Data:[]byte("CREATE TABLE values_to_keep(value INTEGER);\nINSERT INTO values_to_keep VALUES(42);\n")},
		"migrations/002_next.sql":{Data:[]byte("ALTER TABLE values_to_keep ADD COLUMN note TEXT;\nUPDATE values_to_keep SET note='keep';\n")},
	}
	crlf:=fstest.MapFS{}
	for name,file:=range lf{crlf[name]=&fstest.MapFile{Data:[]byte(strings.ReplaceAll(string(file.Data),"\n","\r\n"))}}
	for _,first:=range []fs.FS{lf,crlf}{
		db:=migrationDB(t,path)
		if err:=applyMigrations(context.Background(),db,first);err!=nil{t.Fatal(err)}
		before:=snapshot(t,db,`SELECT * FROM schema_migrations ORDER BY version`)
		if err:=db.Close();err!=nil{t.Fatal(err)}
		db=migrationDB(t,path)
		for _,source:=range []fs.FS{crlf,lf}{
			if err:=applyMigrations(context.Background(),db,source);err!=nil{t.Fatal(err)}
			if got:=snapshot(t,db,`SELECT * FROM schema_migrations ORDER BY version`);!reflect.DeepEqual(got,before){t.Fatalf("line ending change rewrote history: %v -> %v",before,got)}
			if got:=snapshot(t,db,`SELECT * FROM values_to_keep`);!reflect.DeepEqual(got,[]string{`[]interface {}{42, "keep"}`}){t.Fatalf("data changed: %v",got)}
		}
		if err:=db.Close();err!=nil{t.Fatal(err)}
	}
}
