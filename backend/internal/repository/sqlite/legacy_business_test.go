package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/repository/sqlite"
	"github.com/alshatilov/dengovod/backend/internal/service"
	httptransport "github.com/alshatilov/dengovod/backend/internal/transport/http"
	"github.com/google/uuid"
)

// Frozen pre-versioning schema: intentionally independent of production embeds.
// The fixture includes dormant tables so migrations cannot silently discard data
// which the current repository API does not expose.
const businessLegacySchema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    login TEXT NOT NULL UNIQUE COLLATE NOCASE,
    created_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    base_day_cost INTEGER NOT NULL DEFAULT 0,
    horizon_policy TEXT NOT NULL DEFAULT 'END_OF_MONTH',
    horizon_custom_date TEXT,
    currency TEXT NOT NULL DEFAULT 'RUB',
    updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS anchors (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount INTEGER NOT NULL,
    anchored_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_anchors_user ON anchors(user_id, anchored_at DESC, created_at DESC);
CREATE TABLE IF NOT EXISTS categories (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('INCOME','EXPENSE')),
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    UNIQUE(user_id, name, kind)
);
CREATE TABLE IF NOT EXISTS financial_events (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('INCOME','EXPENSE')),
    necessity TEXT NOT NULL CHECK(necessity IN ('REQUIRED','OPTIONAL')),
    amount INTEGER NOT NULL CHECK(amount > 0),
    category_id TEXT REFERENCES categories(id) ON DELETE SET NULL,
    comment TEXT NOT NULL DEFAULT '',
    recurrence_type TEXT NOT NULL CHECK(recurrence_type IN ('NONE','MONTHLY_ON_DAY','EVERY_N_DAYS')),
    start_date TEXT NOT NULL,
    day_of_month INTEGER,
    interval_days INTEGER,
    until_date TEXT,
    cron_expr TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_user ON financial_events(user_id);
CREATE TABLE IF NOT EXISTS day_cost_overrides (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date_from TEXT NOT NULL,
    date_to TEXT NOT NULL,
    daily_amount INTEGER NOT NULL CHECK(daily_amount >= 0),
    comment TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    CHECK(date_from <= date_to)
);
CREATE INDEX IF NOT EXISTS idx_day_cost_user ON day_cost_overrides(user_id, date_from, date_to);
CREATE TABLE IF NOT EXISTS event_occurrence_overrides (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES financial_events(id) ON DELETE CASCADE,
    occurrence_date TEXT NOT NULL,
    amount INTEGER,
    comment TEXT,
    is_skipped INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    UNIQUE(event_id, occurrence_date)
);
CREATE TABLE IF NOT EXISTS actual_transactions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('INCOME','EXPENSE')),
    amount INTEGER NOT NULL CHECK(amount > 0),
    category_id TEXT REFERENCES categories(id) ON DELETE SET NULL,
    transaction_date TEXT NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    event_id TEXT REFERENCES financial_events(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL
);
INSERT INTO users VALUES ('10000000-0000-4000-8000-000000000001','Legacy QA','2026-07-01 09:00:00+00:00');
INSERT INTO settings VALUES ('10000000-0000-4000-8000-000000000001',40000,'CUSTOM_DATE','2026-07-31','RUB','2026-07-01 09:00:00+00:00');
INSERT INTO categories VALUES ('20000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','Зарплата','INCOME',1,'2026-07-01 09:00:00+00:00');
INSERT INTO categories VALUES ('20000000-0000-4000-8000-000000000002','10000000-0000-4000-8000-000000000001','Моя категория','EXPENSE',0,'2026-07-01 09:00:00+00:00');
INSERT INTO anchors VALUES ('30000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001',1000000,'2026-07-01 09:00:00+00:00','2026-07-01 09:00:00+00:00');
INSERT INTO anchors VALUES ('30000000-0000-4000-8000-000000000002','10000000-0000-4000-8000-000000000001',1100000,'2026-07-10 09:00:00+00:00','2026-07-10 09:00:00+00:00');
INSERT INTO financial_events VALUES ('40000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','INCOME','REQUIRED',870000,'20000000-0000-4000-8000-000000000001','Оклад','MONTHLY_ON_DAY','2026-01-31',31,NULL,'2026-12-31',NULL,'2026-07-01 09:00:00+00:00','2026-07-02 10:00:00+00:00');
INSERT INTO financial_events VALUES ('40000000-0000-4000-8000-000000000002','10000000-0000-4000-8000-000000000001','EXPENSE','OPTIONAL',150001,'20000000-0000-4000-8000-000000000002','Покупка','NONE','2026-07-09',NULL,NULL,NULL,NULL,'2026-07-09 10:00:00+00:00','2026-07-09 10:00:00+00:00');
INSERT INTO day_cost_overrides VALUES ('50000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','2026-07-10','2026-07-12',25000,'Поездка','2026-07-01 09:00:00+00:00');
INSERT INTO event_occurrence_overrides VALUES ('60000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','2026-07-31',880001,'Премия',0,'2026-07-02 09:00:00+00:00');
INSERT INTO actual_transactions VALUES ('70000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','EXPENSE',150001,'20000000-0000-4000-8000-000000000002','2026-07-09','Чек','40000000-0000-4000-8000-000000000002','2026-07-09 10:00:00+00:00');
`

var businessLegacyQueries = []string{
	`SELECT id,login,created_at FROM users ORDER BY id`,
	`SELECT user_id,base_day_cost,horizon_policy,horizon_custom_date,currency,updated_at FROM settings ORDER BY user_id`,
	`SELECT id,user_id,amount,anchored_at,created_at FROM anchors ORDER BY id`,
	`SELECT id,user_id,name,kind,is_default,created_at FROM categories ORDER BY id`,
	`SELECT id,user_id,kind,necessity,amount,category_id,comment,recurrence_type,start_date,day_of_month,interval_days,until_date,cron_expr,created_at,updated_at FROM financial_events ORDER BY id`,
	`SELECT id,user_id,date_from,date_to,daily_amount,comment,created_at FROM day_cost_overrides ORDER BY id`,
	`SELECT id,event_id,occurrence_date,amount,comment,is_skipped,created_at FROM event_occurrence_overrides ORDER BY id`,
	`SELECT id,user_id,kind,amount,category_id,transaction_date,comment,event_id,created_at FROM actual_transactions ORDER BY id`,
}

func businessRawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func businessLegacySnapshot(t *testing.T, path string) map[string][][]string {
	t.Helper()
	db := businessRawDB(t, path)
	defer db.Close()
	result := make(map[string][][]string)
	for _, query := range businessLegacyQueries {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatalf("snapshot %s: %v", query, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for i := range values {
				targets[i] = &values[i]
			}
			if err := rows.Scan(targets...); err != nil {
				t.Fatal(err)
			}
			row := make([]string, len(values))
			for i, value := range values {
				row[i] = fmt.Sprintf("%T:%v", value, value)
			}
			result[query] = append(result[query], row)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	return result
}

func TestBusinessLegacyOpenPreservesUserDataAndReopenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db := businessRawDB(t, path)
	if _, err := db.Exec(businessLegacySchema); err != nil {
		db.Close()
		t.Fatalf("create independent legacy fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before := businessLegacySnapshot(t, path)
	for attempt := 1; attempt <= 2; attempt++ {
		store, err := sqlite.Open(path)
		if err != nil {
			t.Fatalf("Open attempt %d: %v", attempt, err)
		}
		user, err := store.ProvisionUser(context.Background(), "LEGACY QA")
		if err != nil {
			store.Close()
			t.Fatal(err)
		}
		if user.ID != uuid.MustParse("10000000-0000-4000-8000-000000000001") {
			t.Fatalf("legacy identity changed: %s", user.ID)
		}
		events, err := store.ListEvents(context.Background(), user.ID)
		if err != nil || len(events) != 2 {
			t.Fatalf("read migrated events: count=%d err=%v", len(events), err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		after := businessLegacySnapshot(t, path)
		if !reflect.DeepEqual(before, after) {
			for query, rows := range before {
				if !reflect.DeepEqual(rows, after[query]) {
					t.Errorf("Open attempt %d changed legacy values\nquery: %s\nbefore: %v\nafter: %v", attempt, query, rows, after[query])
				}
			}
			t.FailNow()
		}
	}
}

func TestBusinessLegacyReviewFlagAndSalaryIdentityThroughAPI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-review.db")
	db := businessRawDB(t, path)
	if _, err := db.Exec(businessLegacySchema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	for attempt := 0; attempt < 2; attempt++ {
		store, err := sqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		app := service.New(store)
		app.Now = func() time.Time { return time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC) }
		router := httptransport.NewRouter(app)
		call := func(method, endpoint, body string, want int) map[string]json.RawMessage {
			t.Helper()
			request := httptest.NewRequest(method, "/api/v1"+endpoint, bytes.NewBufferString(body))
			request.Header.Set("Authorization", "Bearer 10000000-0000-4000-8000-000000000001")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != want {
				store.Close()
				t.Fatalf("%s %s: %d %s", method, endpoint, response.Code, response.Body.String())
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				store.Close()
				t.Fatal(err)
			}
			return result
		}
		balance := call(http.MethodGet, "/balance", "", http.StatusOK)
		wantReview := "true"
		if attempt == 1 {
			wantReview = "false"
		}
		if got := string(balance["needs_balance_review"]); got != wantReview {
			store.Close()
			t.Fatalf("review flag on open %d: got %s want %s", attempt+1, got, wantReview)
		}
		var amount struct {
			Amount int64 `json:"amount"`
		}
		if err := json.Unmarshal(balance["computed_balance"], &amount); err != nil || amount.Amount != 1_100_000 {
			store.Close()
			t.Fatalf("legacy plans/reserve changed reconciled actual balance: %s, err %v", balance["computed_balance"], err)
		}
		categories := call(http.MethodGet, "/categories", "", http.StatusOK)
		var items []struct {
			ID         string  `json:"id"`
			SystemCode *string `json:"system_code"`
		}
		if err := json.Unmarshal(categories["categories"], &items); err != nil {
			store.Close()
			t.Fatal(err)
		}
		found := false
		for _, item := range items {
			if item.ID == "20000000-0000-4000-8000-000000000001" && item.SystemCode != nil && *item.SystemCode == "SALARY" {
				found = true
			}
		}
		if !found {
			store.Close()
			t.Fatalf("migration lost salary UUID/system identity: %s", categories["categories"])
		}
		if attempt == 0 {
			updated := call(http.MethodPost, "/balance/anchor", `{"amount":{"amount":1100000,"currency":"RUB"}}`, http.StatusCreated)
			if string(updated["needs_balance_review"]) != "false" {
				store.Close()
				t.Fatalf("reconciliation did not reset review flag: %v", updated)
			}
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBusinessLegacyUnsafeMoneyIsRejectedWithoutTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-unsafe.db")
	db := businessRawDB(t, path)
	if _, err := db.Exec(businessLegacySchema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE anchors SET amount=9007199254740992 WHERE id='30000000-0000-4000-8000-000000000002'`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	before := businessLegacySnapshot(t, path)
	store, err := sqlite.Open(path)
	if err == nil {
		store.Close()
		t.Fatal("unsafe legacy amount accepted; migration must fail explicitly")
	}
	after := businessLegacySnapshot(t, path)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed unsafe-money migration changed legacy monetary values")
	}
}

func TestBusinessLegacyFactsAtAnchorTimeAreIncludedAndReopenKeepsOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-order.db")
	db := businessRawDB(t, path)
	if _, err := db.Exec(businessLegacySchema + `
INSERT INTO actual_transactions VALUES ('80000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','INCOME',500000,NULL,'2026-07-10','Included at equal-time anchor',NULL,'2026-07-10 09:00:00+00:00');
INSERT INTO actual_transactions VALUES ('80000000-0000-4000-8000-000000000002','10000000-0000-4000-8000-000000000001','INCOME',70000,NULL,'2026-07-11','After anchor',NULL,'2026-07-11 09:00:00+00:00');
`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	before := businessLegacySnapshot(t, path)
	for attempt := 0; attempt < 2; attempt++ {
		store, err := sqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		app := service.New(store)
		app.Now = func() time.Time { return time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC) }
		balance, err := app.GetBalance(context.Background(), uuid.MustParse("10000000-0000-4000-8000-000000000001"))
		if err != nil || balance.ComputedBalance.Amount != 1_170_000 {
			store.Close()
			t.Fatalf("legacy balance on open %d: got %+v, err=%v, want 1170000 (equal-time fact absorbed)", attempt+1, balance, err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		if after := businessLegacySnapshot(t, path); !reflect.DeepEqual(before, after) {
			t.Fatal("legacy ledger conversion changed original rows")
		}
	}
}

func TestBusinessLegacyAggregateOverflowRollsBackFinancialMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-aggregate-overflow.db")
	db := businessRawDB(t, path)
	if _, err := db.Exec(businessLegacySchema + `
UPDATE anchors SET amount=9007199254740990 WHERE id='30000000-0000-4000-8000-000000000002';
INSERT INTO actual_transactions VALUES ('90000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','INCOME',2,NULL,'2026-07-11','Individually safe, aggregate unsafe',NULL,'2026-07-11 09:00:00+00:00');
`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before := businessLegacySnapshot(t, path)
	store, err := sqlite.Open(path)
	if err == nil {
		store.Close()
		t.Fatal("aggregate unsafe legacy balance accepted: 9007199254740990 + 2 exceeds MAX_SAFE_INTEGER")
	}
	if after := businessLegacySnapshot(t, path); !reflect.DeepEqual(before, after) {
		t.Fatal("rejected aggregate-overflow migration changed legacy rows or monetary values")
	}
	db = businessRawDB(t, path)
	defer db.Close()
	var committed int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version >= 3`).Scan(&committed); err != nil {
		t.Fatal(err)
	}
	if committed != 0 {
		t.Fatalf("financial migration was recorded despite aggregate overflow: %d committed versions >= 3", committed)
	}
	// v1/v2 may remain committed. No new financial table from v3 may survive.
	rows, err := db.Query(`SELECT name FROM sqlite_schema WHERE type='table'
AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	wantTables := []string{"actual_transactions", "anchors", "categories", "day_cost_overrides", "event_occurrence_overrides", "financial_events", "settings", "users"}
	if !reflect.DeepEqual(tables, wantTables) {
		t.Fatalf("financial migration left partial tables: got %v, want legacy tables %v", tables, wantTables)
	}
}
