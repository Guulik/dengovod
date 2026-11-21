package httptransport_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const dv8Ruble int64 = 100

func dv8Setup(t *testing.T, at string, dayCost int64, horizon string) (businessAPI, string) {
	t.Helper()
	api := newBusinessAPI(t)
	api.at(at)
	token := api.login(t.Name())
	api.settings(token, dayCost, "CUSTOM_DATE", horizon)
	api.anchor(token, 20_000*dv8Ruble, at)
	return api, token
}

func dv8Override(api businessAPI, token, from, to string, amount int64) string {
	api.t.Helper()
	value := api.call(http.MethodPost, "/day-cost-overrides", token, map[string]any{
		"date_from": from, "date_to": to,
		"daily_amount": map[string]any{"amount": amount, "currency": "RUB"},
	}, http.StatusCreated)
	return businessField[string](api.t, value, "id")
}

func dv8UpdateOverride(api businessAPI, token, id, from, to string, amount int64) {
	api.t.Helper()
	api.call(http.MethodPut, "/day-cost-overrides/"+id, token, map[string]any{
		"date_from": from, "date_to": to,
		"daily_amount": map[string]any{"amount": amount, "currency": "RUB"},
	}, http.StatusOK)
}

// Read the same fixed clock through all three public financial endpoints. A
// different visible calendar month must not change the current summary.
func dv8ReadState(api businessAPI, token string, want int64) map[string]json.RawMessage {
	api.t.Helper()
	balance := api.call(http.MethodGet, "/balance", token, nil, http.StatusOK)
	businessMoney(api.t, balance, "computed_balance", want)
	var asOf time.Time
	if err := json.Unmarshal(balance["as_of"], &asOf); err != nil || !asOf.Equal(*api.now) {
		api.t.Fatalf("balance as_of differs from fixture clock: %s, err=%v", balance["as_of"], err)
	}
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(api.t, forecast, "current_balance", want)
	moscow := api.now.In(time.FixedZone("Europe/Moscow", 3*60*60))
	for _, day := range []string{moscow.Format("2006-01-02"), moscow.AddDate(0, 1, 0).Format("2006-01-02")} {
		calendar := api.call(http.MethodGet, "/calendar?from="+day+"&to="+day, token, nil, http.StatusOK)
		summary := businessField[map[string]json.RawMessage](api.t, calendar, "summary")
		if !reflect.DeepEqual(summary, forecast) {
			api.t.Fatalf("visible day %s changed summary: calendar=%s forecast=%v", day, calendar["summary"], forecast)
		}
	}
	return forecast
}

func TestBusinessDV8CompletedDaysReduceBalanceWithoutReservingTwice(t *testing.T) {
	api, token := dv8Setup(t, "2026-11-16T09:00:00Z", 1_400*dv8Ruble, "2026-11-18")
	forecast := dv8ReadState(api, token, 20_000*dv8Ruble)
	businessMoney(t, forecast, "day_cost_remaining", 4_200*dv8Ruble)
	businessMoney(t, forecast, "money_for_life", 15_800*dv8Ruble)
	day := api.days(token, "2026-11-16", "2026-11-16")[0]
	businessMoney(t, day, "expected_balance", 18_600*dv8Ruble)
	api.at("2026-11-17T09:00:00Z")
	forecast = dv8ReadState(api, token, 18_600*dv8Ruble)
	businessMoney(t, forecast, "day_cost_remaining", 2_800*dv8Ruble)
	businessMoney(t, forecast, "money_for_life", 15_800*dv8Ruble)
	api.at("2026-11-18T09:00:00Z")
	forecast = dv8ReadState(api, token, 17_200*dv8Ruble)
	businessMoney(t, forecast, "day_cost_remaining", 1_400*dv8Ruble)
	businessMoney(t, forecast, "money_for_life", 15_800*dv8Ruble)
	days := api.days(token, "2026-11-16", "2026-11-18")
	for i, want := range []int64{18_600, 17_200, 15_800} {
		businessMoney(t, days[i], "expected_balance", want*dv8Ruble)
		businessMoney(t, days[i], "money_for_life", 15_800*dv8Ruble)
	}
}

func dv8LedgerCounts(t *testing.T, path string) []int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	counts := make([]int, 4)
	for i, query := range []string{
		`SELECT count(*) FROM actual_transactions`, `SELECT count(*) FROM occurrence_states`,
		`SELECT count(*) FROM financial_events`, `SELECT count(*) FROM anchors`,
	} {
		if err := db.QueryRow(query).Scan(&counts[i]); err != nil {
			t.Fatal(err)
		}
	}
	return counts
}

func TestBusinessDV8WeekAwayAndReopenDoNotMaterializeFacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dv8-week-away.db")
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	api, store := persistentBusinessAPI(t, path, &now)
	token := api.login(t.Name())
	api.settings(token, 1_400*dv8Ruble, "CUSTOM_DATE", "2026-11-30")
	api.anchor(token, 20_000*dv8Ruble, "2026-11-16T09:00:00Z")
	before := dv8LedgerCounts(t, path)
	if !reflect.DeepEqual(before, []int{0, 0, 0, 1}) {
		t.Fatalf("unexpected fixture ledger: %v", before)
	}
	api.at("2026-11-23T09:00:00Z")
	for i := 0; i < 3; i++ {
		dv8ReadState(api, token, 10_200*dv8Ruble)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	api, store = persistentBusinessAPI(t, path, &now)
	dv8ReadState(api, token, 10_200*dv8Ruble)
	day := api.days(token, "2026-11-23", "2026-11-23")[0]
	businessMoney(t, day, "expected_balance", 8_800*dv8Ruble)
	if after := dv8LedgerCounts(t, path); !reflect.DeepEqual(before, after) {
		t.Fatalf("reading estimates wrote ledger rows: before=%v after=%v", before, after)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBusinessDV8MoscowMidnightMonthYearAndLeapBoundaries(t *testing.T) {
	for _, pair := range [][2]string{
		{"2026-11-16", "2026-11-17"}, {"2026-01-31", "2026-02-01"},
		{"2026-12-31", "2027-01-01"}, {"2024-02-28", "2024-02-29"}, {"2024-02-29", "2024-03-01"},
	} {
		t.Run(pair[0], func(t *testing.T) {
			api, token := dv8Setup(t, pair[0]+"T09:00:00Z", 1_400*dv8Ruble, pair[1])
			api.at(pair[0] + "T20:59:59Z")
			dv8ReadState(api, token, 20_000*dv8Ruble)
			api.at(pair[0] + "T21:00:00Z")
			dv8ReadState(api, token, 18_600*dv8Ruble)
			dv8ReadState(api, token, 18_600*dv8Ruble)
			days := api.days(token, pair[0], pair[1])
			businessMoney(t, days[0], "expected_balance", 18_600*dv8Ruble)
			businessMoney(t, days[1], "expected_balance", 17_200*dv8Ruble)
		})
	}
}

func TestBusinessDV8PastCurrentFutureOverridesZeroAndDeletion(t *testing.T) {
	api, token := dv8Setup(t, "2026-11-16T09:00:00Z", 1_400*dv8Ruble, "2026-11-20")
	api.at("2026-11-17T09:00:00Z")
	dv8ReadState(api, token, 18_600*dv8Ruble)
	past := dv8Override(api, token, "2026-11-16", "2026-11-16", 1_000*dv8Ruble)
	dv8ReadState(api, token, 19_000*dv8Ruble)
	current := dv8Override(api, token, "2026-11-17", "2026-11-17", 800*dv8Ruble)
	dv8ReadState(api, token, 19_000*dv8Ruble)
	future := dv8Override(api, token, "2026-11-18", "2026-11-18", 600*dv8Ruble)
	dv8ReadState(api, token, 19_000*dv8Ruble)
	days := api.days(token, "2026-11-17", "2026-11-18")
	businessMoney(t, days[0], "expected_balance", 18_200*dv8Ruble)
	businessMoney(t, days[1], "expected_balance", 17_600*dv8Ruble)
	dv8UpdateOverride(api, token, past, "2026-11-16", "2026-11-16", 0)
	dv8ReadState(api, token, 20_000*dv8Ruble)
	api.call(http.MethodDelete, "/day-cost-overrides/"+past, token, nil, http.StatusNoContent)
	dv8ReadState(api, token, 18_600*dv8Ruble)
	dv8UpdateOverride(api, token, future, "2026-11-18", "2026-11-18", 0)
	api.at("2026-11-18T09:00:00Z")
	dv8ReadState(api, token, 17_800*dv8Ruble)
	day := api.days(token, "2026-11-18", "2026-11-18")[0]
	businessMoney(t, day, "expected_balance", 17_800*dv8Ruble)
	api.at("2026-11-19T09:00:00Z")
	dv8ReadState(api, token, 17_800*dv8Ruble)
	api.call(http.MethodDelete, "/day-cost-overrides/"+current, token, nil, http.StatusNoContent)
	dv8ReadState(api, token, 17_200*dv8Ruble)
}

func TestBusinessDV8BaseHistoryAndNewReconciliationResetPriorDays(t *testing.T) {
	api, token := dv8Setup(t, "2026-11-16T09:00:00Z", 1_400*dv8Ruble, "2026-11-20")
	api.at("2026-11-17T12:00:00Z")
	dv8ReadState(api, token, 18_600*dv8Ruble)
	api.settings(token, 2_000*dv8Ruble, "CUSTOM_DATE", "2026-11-20")
	dv8ReadState(api, token, 18_600*dv8Ruble)
	days := api.days(token, "2026-11-16", "2026-11-17")
	businessMoney(t, days[0], "day_cost", 1_400*dv8Ruble)
	businessMoney(t, days[0], "expected_balance", 18_600*dv8Ruble)
	businessMoney(t, days[1], "day_cost", 2_000*dv8Ruble)
	api.anchor(token, 15_000*dv8Ruble, "2026-11-17T12:00:00Z")
	dv8ReadState(api, token, 15_000*dv8Ruble)
	api.at("2026-11-18T09:00:00Z")
	dv8ReadState(api, token, 13_000*dv8Ruble)
	dv8Override(api, token, "2026-11-16", "2026-11-16", 0)
	dv8ReadState(api, token, 13_000*dv8Ruble)
	days = api.days(token, "2026-11-16", "2026-11-18")
	businessMoney(t, days[0], "expected_balance", 20_000*dv8Ruble)
	businessMoney(t, days[1], "expected_balance", 13_000*dv8Ruble)
	businessMoney(t, days[2], "expected_balance", 11_000*dv8Ruble)
	api.anchor(token, 11_000*dv8Ruble, "2026-11-18T09:00:00Z")
	dv8ReadState(api, token, 11_000*dv8Ruble)
	api.at("2026-11-19T09:00:00Z")
	dv8ReadState(api, token, 9_000*dv8Ruble)
}

func TestBusinessDV8MarkedAndUnmarkedFactsReplaceOnlyTheirDailyEstimate(t *testing.T) {
	for _, test := range []struct {
		name   string
		amount int64
		marked bool
		now    int64
		next   int64
	}{
		{"marked_200", 200, true, 19_800, 18_600},
		{"marked_2000", 2_000, true, 18_000, 18_000},
		{"unmarked_2000", 2_000, false, 18_000, 16_600},
	} {
		t.Run(test.name, func(t *testing.T) {
			api, token := dv8Setup(t, "2026-11-16T09:00:00Z", 1_400*dv8Ruble, "2026-11-18")
			id := api.create(token, "EXPENSE", "2026-11-16", test.amount*dv8Ruble)
			payload := map[string]any{"status": "CONFIRMED", "counts_toward_day_budget": test.marked}
			api.occurrence(token, id, "2026-11-16", payload, http.StatusOK)
			api.occurrence(token, id, "2026-11-16", payload, http.StatusOK)
			dv8ReadState(api, token, test.now*dv8Ruble)
			day := api.days(token, "2026-11-16", "2026-11-16")[0]
			businessMoney(t, day, "expected_balance", test.next*dv8Ruble)
			api.at("2026-11-17T09:00:00Z")
			dv8ReadState(api, token, test.next*dv8Ruble)
			for i := 0; i < 2; i++ {
				api.occurrence(token, id, "2026-11-16", map[string]any{"status": "PLANNED"}, http.StatusOK)
				dv8ReadState(api, token, 18_600*dv8Ruble)
			}
			payload["effective_at"] = "2026-11-16T09:00:00Z"
			api.occurrence(token, id, "2026-11-16", payload, http.StatusOK)
			dv8ReadState(api, token, test.next*dv8Ruble)
			day = api.days(token, "2026-11-16", "2026-11-16")[0]
			if got := len(businessField[[]json.RawMessage](t, day, "events")); got != 1 {
				t.Fatalf("confirmation cycle duplicated occurrences: %s", day["events"])
			}
		})
	}
}

func TestBusinessDV8MarkedFactsBeforeAndAfterIntradayAnchor(t *testing.T) {
	for _, test := range []struct {
		name          string
		before, after int64
		wantNow       int64
		wantNext      int64
	}{
		{"before_only", 200, 0, 15_000, 13_800},
		{"after_only", 0, 200, 14_800, 13_600},
		{"both_sides", 200, 300, 14_700, 13_800},
		{"before_above_budget", 2_000, 0, 15_000, 15_000},
		{"after_above_budget", 0, 2_000, 13_000, 13_000},
	} {
		t.Run(test.name, func(t *testing.T) {
			api, token := dv8Setup(t, "2026-11-16T08:00:00Z", 1_400*dv8Ruble, "2026-11-18")
			if test.before > 0 {
				api.at("2026-11-16T09:00:00Z")
				id := api.create(token, "EXPENSE", "2026-11-16", test.before*dv8Ruble)
				api.occurrence(token, id, "2026-11-16", map[string]any{"status": "CONFIRMED", "counts_toward_day_budget": true}, http.StatusOK)
			}
			api.at("2026-11-16T12:00:00Z")
			api.anchor(token, 15_000*dv8Ruble, "2026-11-16T12:00:00Z")
			dv8ReadState(api, token, 15_000*dv8Ruble)
			if test.after > 0 {
				api.at("2026-11-16T13:00:00Z")
				id := api.create(token, "EXPENSE", "2026-11-16", test.after*dv8Ruble)
				api.occurrence(token, id, "2026-11-16", map[string]any{"status": "CONFIRMED", "counts_toward_day_budget": true}, http.StatusOK)
			}
			dv8ReadState(api, token, test.wantNow*dv8Ruble)
			day := api.days(token, "2026-11-16", "2026-11-16")[0]
			businessMoney(t, day, "expected_balance", test.wantNext*dv8Ruble)
			api.at("2026-11-17T09:00:00Z")
			dv8ReadState(api, token, test.wantNext*dv8Ruble)
		})
	}
}

func TestBusinessDV8MovedMarkedExpenseUsesEffectiveDayBudget(t *testing.T) {
	api, token := dv8Setup(t, "2026-11-16T09:00:00Z", 1_400*dv8Ruble, "2026-11-18")
	id := api.create(token, "EXPENSE", "2026-11-16", 200*dv8Ruble)
	api.occurrence(token, id, "2026-11-16", map[string]any{"status": "CONFIRMED", "counts_toward_day_budget": true}, http.StatusOK)
	api.at("2026-11-17T09:00:00Z")
	dv8ReadState(api, token, 18_600*dv8Ruble)
	api.occurrence(token, id, "2026-11-16", map[string]any{"status": "PLANNED"}, http.StatusOK)
	dv8ReadState(api, token, 18_600*dv8Ruble)
	api.occurrence(token, id, "2026-11-16", map[string]any{
		"status": "CONFIRMED", "counts_toward_day_budget": true, "effective_at": "2026-11-17T09:00:00Z",
	}, http.StatusOK)
	dv8ReadState(api, token, 18_400*dv8Ruble)
	days := api.days(token, "2026-11-16", "2026-11-17")
	businessMoney(t, days[0], "expected_balance", 18_600*dv8Ruble)
	businessMoney(t, days[1], "expected_balance", 17_200*dv8Ruble)
	if got := len(businessField[[]json.RawMessage](t, days[0], "events")); got != 0 {
		t.Fatalf("moved expense remained in original day: %s", days[0]["events"])
	}
	items := businessField[[]map[string]json.RawMessage](t, days[1], "events")
	if len(items) != 1 || businessField[string](t, items[0], "occurrence_date") != "2026-11-16" {
		t.Fatalf("moved expense lost original key: %s", days[1]["events"])
	}
	api.at("2026-11-18T09:00:00Z")
	dv8ReadState(api, token, 17_200*dv8Ruble)
}

func TestBusinessDV8MidnightReconciliationStartsANewDailySegment(t *testing.T) {
	api, token := dv8Setup(t, "2026-11-16T09:00:00Z", 1_400*dv8Ruble, "2026-11-18")
	api.at("2026-11-16T20:59:59Z")
	dv8ReadState(api, token, 20_000*dv8Ruble)
	api.at("2026-11-16T21:00:00Z")
	dv8ReadState(api, token, 18_600*dv8Ruble)
	api.anchor(token, 15_000*dv8Ruble, "2026-11-16T21:00:00Z")
	api.anchor(token, 15_000*dv8Ruble, "2026-11-16T21:00:00Z")
	dv8ReadState(api, token, 15_000*dv8Ruble)
	api.at("2026-11-17T20:59:59Z")
	dv8ReadState(api, token, 15_000*dv8Ruble)
	api.at("2026-11-17T21:00:00Z")
	dv8ReadState(api, token, 13_600*dv8Ruble)
	days := api.days(token, "2026-11-16", "2026-11-18")
	businessMoney(t, days[0], "expected_balance", 18_600*dv8Ruble)
	businessMoney(t, days[1], "expected_balance", 13_600*dv8Ruble)
}

func TestBusinessDV8SparseAncientAnchorAccumulatesKnownCompletedCost(t *testing.T) {
	api := newBusinessAPI(t)
	api.at("2026-11-17T09:00:00Z")
	token := api.login(t.Name())
	api.settings(token, 1, "CUSTOM_DATE", "2026-11-17")
	api.anchor(token, 20_000*dv8Ruble, "0001-01-01T01:00:00Z")
	dv8Override(api, token, "0001-01-01", "2026-11-16", 1)
	// Gregorian date difference independently checked with Python datetime:
	// (date(2026, 11, 17) - date(1, 1, 1)).days == 739936.
	const wantNow int64 = 2_000_000 - 739_936
	balance := api.boundedGet("/balance", token, http.StatusOK)
	businessMoney(t, balance, "computed_balance", wantNow)
	forecast := api.boundedGet("/forecast", token, http.StatusOK)
	businessMoney(t, forecast, "current_balance", wantNow)
	businessMoney(t, forecast, "money_for_life", wantNow-1)
	calendar := api.boundedGet("/calendar?from=0001-01-01&to=0001-01-01", token, http.StatusOK)
	days := businessField[[]map[string]json.RawMessage](t, calendar, "days")
	if len(days) != 1 {
		t.Fatalf("ancient single-day request returned %d days", len(days))
	}
	businessMoney(t, days[0], "expected_balance", 1_999_999)
	businessMoney(t, days[0], "money_for_life", wantNow-1)
	summary := businessField[map[string]json.RawMessage](t, calendar, "summary")
	if !reflect.DeepEqual(summary, forecast) {
		t.Fatal("ancient visible date changed current summary")
	}
}

func TestBusinessDV8DailyCostWithoutAnchorKeepsBalancesUnknown(t *testing.T) {
	api := newBusinessAPI(t)
	api.at("2026-11-16T09:00:00Z")
	token := api.login(t.Name())
	api.settings(token, 1_400*dv8Ruble, "CUSTOM_DATE", "2026-11-20")
	api.create(token, "EXPENSE", "2026-11-15", 200*dv8Ruble)
	api.at("2026-11-20T09:00:00Z")
	for _, path := range []string{"/balance", "/forecast"} {
		api.call(http.MethodGet, path, token, nil, http.StatusConflict)
	}
	calendar := api.call(http.MethodGet, "/calendar?from=2026-11-15&to=2026-11-20", token, nil, http.StatusOK)
	if !bytes.Equal(calendar["summary"], []byte("null")) {
		t.Fatalf("unanchored estimates created a summary: %s", calendar["summary"])
	}
	days := businessField[[]map[string]json.RawMessage](t, calendar, "days")
	if len(days) != 6 {
		t.Fatalf("expected six unanchored days, got %d", len(days))
	}
	for i, day := range days {
		if string(day["expected_balance"]) != "null" || string(day["money_for_life"]) != "null" {
			t.Fatalf("daily estimate invented pre-anchor balance: %v", day)
		}
		if i > 0 {
			businessMoney(t, day, "day_cost", 1_400*dv8Ruble)
		}
	}
	if got := len(businessField[[]json.RawMessage](t, days[0], "events")); got != 1 {
		t.Fatalf("unanchored past operation disappeared: %s", days[0]["events"])
	}
}
