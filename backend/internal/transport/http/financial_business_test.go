package httptransport_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func (api businessAPI) at(value string) {
	api.t.Helper()
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		api.t.Fatal(err)
	}
	*api.now = at
}

func (api businessAPI) anchor(token string, amount int64, at string) {
	api.t.Helper()
	api.call(http.MethodPost, "/balance/anchor", token, map[string]any{
		"amount": map[string]any{"amount": amount, "currency": "RUB"}, "anchored_at": at,
	}, http.StatusCreated)
}

func (api businessAPI) settings(token string, cost int64, policy, custom string) {
	api.t.Helper()
	body := map[string]any{"base_day_cost": map[string]any{"amount": cost, "currency": "RUB"}, "horizon_policy": policy}
	if custom != "" {
		body["horizon_custom_date"] = custom
	}
	api.call(http.MethodPut, "/settings", token, body, http.StatusOK)
}

func (api businessAPI) create(token, kind, day string, amount int64) string {
	api.t.Helper()
	body := businessEvent(amount, day)
	body["kind"] = kind
	created := api.call(http.MethodPost, "/events", token, body, http.StatusCreated)
	return businessField[string](api.t, created, "id")
}

func (api businessAPI) occurrence(token, id, date string, body map[string]any, want int) map[string]json.RawMessage {
	api.t.Helper()
	return api.call(http.MethodPut, "/events/"+id+"/occurrences/"+date, token, body, want)
}

func (api businessAPI) balance(token string, want int64) {
	api.t.Helper()
	value := api.call(http.MethodGet, "/balance", token, nil, http.StatusOK)
	businessMoney(api.t, value, "computed_balance", want)
}

func businessMoney(t *testing.T, value map[string]json.RawMessage, field string, want int64) {
	t.Helper()
	amount := businessField[map[string]json.RawMessage](t, value, field)
	got := businessField[int64](t, amount, "amount")
	if got != want || businessField[string](t, amount, "currency") != "RUB" {
		t.Fatalf("%s: got %s, want %d kopecks RUB", field, value[field], want)
	}
}

func (api businessAPI) days(token, from, to string) []map[string]json.RawMessage {
	api.t.Helper()
	value := api.call(http.MethodGet, "/calendar?from="+from+"&to="+to, token, nil, http.StatusOK)
	days := businessField[[]map[string]json.RawMessage](api.t, value, "days")
	first, firstErr := time.Parse("2006-01-02", from)
	last, lastErr := time.Parse("2006-01-02", to)
	if firstErr != nil || lastErr != nil {
		api.t.Fatal("invalid test date fixture")
	}
	want := int(last.Sub(first).Hours()/24) + 1
	if len(days) != want {
		api.t.Fatalf("calendar %s..%s returned %d days, want %d", from, to, len(days), want)
	}
	for i, day := range days {
		if got := businessField[string](api.t, day, "date"); got != first.AddDate(0, 0, i).Format("2006-01-02") {
			api.t.Fatalf("calendar day %d has wrong date: %q", i, got)
		}
	}
	return days
}

func TestBusinessCalendarWithoutAnchorAndUnknownHistory(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-unknown")
	id := api.create(token, "EXPENSE", "2026-07-10", 150_000)
	value := api.call(http.MethodGet, "/calendar?from=2026-07-09&to=2026-07-11", token, nil, http.StatusOK)
	if string(value["summary"]) != "null" {
		t.Fatalf("unanchored summary: %s", value["summary"])
	}
	days := businessField[[]map[string]json.RawMessage](t, value, "days")
	if len(days) != 3 {
		t.Fatalf("calendar has %d days", len(days))
	}
	for _, day := range days {
		if string(day["expected_balance"]) != "null" || string(day["money_for_life"]) != "null" {
			t.Fatalf("unknown amounts must be null: %v", day)
		}
		businessMoney(t, day, "day_cost", 0)
	}
	items := businessField[[]map[string]json.RawMessage](t, days[1], "events")
	if len(items) != 1 || businessField[string](t, items[0], "event_id") != id || businessField[string](t, items[0], "status") != "PLANNED" {
		t.Fatalf("past plan absent: %s", days[1]["events"])
	}
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	days = api.days(token, "2026-07-09", "2026-07-11")
	for _, day := range days[:2] {
		if string(day["expected_balance"]) != "null" {
			t.Fatalf("first anchor invented prior history: %v", day)
		}
	}
	api.balance(token, 1_000_000)
}

func TestBusinessIncomePlanConfirmationAndNextDayCarry(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-carry")
	api.settings(token, 40_000, "CUSTOM_DATE", "2026-07-12")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	id := api.create(token, "INCOME", "2026-07-11", 870_000)
	api.balance(token, 1_000_000)
	days := api.days(token, "2026-07-11", "2026-07-12")
	businessMoney(t, days[0], "expected_balance", 1_830_000)
	businessMoney(t, days[1], "expected_balance", 1_790_000)
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "current_balance", 1_000_000)
	businessMoney(t, forecast, "money_for_life", 1_790_000)
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.balance(token, 1_870_000)
	days = api.days(token, "2026-07-11", "2026-07-12")
	businessMoney(t, days[0], "expected_balance", 1_830_000)
	businessMoney(t, days[1], "expected_balance", 1_790_000)
	api.at("2026-07-12T12:00:00Z")
	api.balance(token, 1_870_000)
	days = api.days(token, "2026-07-12", "2026-07-12")
	businessMoney(t, days[0], "expected_balance", 1_830_000)
}

func TestBusinessExactAnchorBoundaryUsesLedgerOrder(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-sequence")
	first := api.create(token, "INCOME", "2026-07-11", 100)
	api.occurrence(token, first, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.anchor(token, 1_000, "2026-07-11T12:00:00Z")
	api.balance(token, 1_000)
	second := api.create(token, "INCOME", "2026-07-11", 200)
	api.occurrence(token, second, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.balance(token, 1_200)
	api.anchor(token, 1_200, "2026-07-11T12:00:00Z")
	api.anchor(token, 1_200, "2026-07-11T12:00:00Z")
	api.balance(token, 1_200)
}

func TestBusinessMultipleAnchorsPreserveHistoryAndAbsorbEarlierFacts(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-history")
	api.anchor(token, 1_000_000, "2026-07-09T08:00:00Z")
	old := api.create(token, "EXPENSE", "2026-07-09", 150_000)
	api.occurrence(token, old, "2026-07-09", map[string]any{"status": "CONFIRMED", "effective_at": "2026-07-09T09:00:00Z"}, http.StatusOK)
	api.anchor(token, 2_000_000, "2026-07-10T08:00:00Z")
	current := api.create(token, "INCOME", "2026-07-11", 300_000)
	api.occurrence(token, current, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.balance(token, 2_300_000)
	days := api.days(token, "2026-07-09", "2026-07-11")
	businessMoney(t, days[0], "expected_balance", 850_000)
	businessMoney(t, days[1], "expected_balance", 2_000_000)
	businessMoney(t, days[2], "expected_balance", 2_300_000)
	api.occurrence(token, old, "2026-07-09", map[string]any{"status": "PLANNED"}, http.StatusOK)
	api.balance(token, 2_300_000)
	api.anchor(token, 2_500_000, "2026-07-11T12:00:00Z")
	days = api.days(token, "2026-07-09", "2026-07-11")
	// Later control points must remain independent of the modified segment.
	businessMoney(t, days[1], "expected_balance", 2_000_000)
	businessMoney(t, days[2], "expected_balance", 2_500_000)
}

func TestBusinessMovedFactAppearsOnlyOnEffectiveDay(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-moved")
	api.anchor(token, 1_000_000, "2026-07-09T08:00:00Z")
	id := api.create(token, "INCOME", "2026-07-10", 870_000)
	api.occurrence(token, id, "2026-07-10", map[string]any{"status": "CONFIRMED", "effective_at": "2026-07-11T09:00:00Z"}, http.StatusOK)
	days := api.days(token, "2026-07-10", "2026-07-11")
	if got := len(businessField[[]json.RawMessage](t, days[0], "events")); got != 0 {
		t.Fatalf("moved fact still appears on planned day: %s", days[0]["events"])
	}
	items := businessField[[]map[string]json.RawMessage](t, days[1], "events")
	if len(items) != 1 || businessField[string](t, items[0], "occurrence_date") != "2026-07-10" || businessField[string](t, items[0], "status") != "CONFIRMED" {
		t.Fatalf("moved snapshot: %s", days[1]["events"])
	}
	businessMoney(t, days[0], "expected_balance", 1_000_000)
	businessMoney(t, days[1], "expected_balance", 1_870_000)
	api.balance(token, 1_870_000)
}

func TestBusinessDailyReserveOnlyUsesExplicitEverydayFacts(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-reserve")
	api.settings(token, 40_000, "CUSTOM_DATE", "2026-07-11")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	marked := api.create(token, "EXPENSE", "2026-07-11", 15_000)
	api.occurrence(token, marked, "2026-07-11", map[string]any{"status": "CONFIRMED", "counts_toward_day_budget": true}, http.StatusOK)
	api.balance(token, 985_000)
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "day_cost_remaining", 25_000)
	businessMoney(t, forecast, "money_for_life", 960_000)
	unmarked := api.create(token, "EXPENSE", "2026-07-11", 10_000)
	api.occurrence(token, unmarked, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.balance(token, 975_000)
	forecast = api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "day_cost_remaining", 25_000)
	businessMoney(t, forecast, "money_for_life", 950_000)
	extra := api.create(token, "EXPENSE", "2026-07-11", 30_000)
	api.occurrence(token, extra, "2026-07-11", map[string]any{"status": "CONFIRMED", "counts_toward_day_budget": true}, http.StatusOK)
	forecast = api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "day_cost_remaining", 0)
	businessMoney(t, forecast, "money_for_life", 945_000)
	api.balance(token, 945_000)
	day := api.days(token, "2026-07-11", "2026-07-11")[0]
	businessMoney(t, day, "expected_balance", 945_000)
}

func TestBusinessConfirmationIdempotencyAndExplicitUndo(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-repeat")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	id := api.create(token, "EXPENSE", "2026-07-11", 150_000)
	first := api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.balance(token, 850_000)
	api.at("2026-07-11T13:00:00Z")
	again := api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	if string(first["effective_at"]) != string(again["effective_at"]) {
		t.Fatalf("repeat changed effective_at: first=%s again=%s", first["effective_at"], again["effective_at"])
	}
	api.balance(token, 850_000)
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED", "effective_at": "2026-07-11T13:00:00Z"}, http.StatusConflict)
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED", "counts_toward_day_budget": true}, http.StatusConflict)
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CANCELLED"}, http.StatusConflict)
	for i := 0; i < 2; i++ {
		api.occurrence(token, id, "2026-07-11", map[string]any{"status": "PLANNED"}, http.StatusOK)
		api.balance(token, 1_000_000)
	}
	for i := 0; i < 2; i++ {
		api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CANCELLED"}, http.StatusOK)
	}
	day := api.days(token, "2026-07-11", "2026-07-11")[0]
	businessMoney(t, day, "expected_balance", 1_000_000)
	items := businessField[[]map[string]json.RawMessage](t, day, "events")
	if len(items) != 1 || businessField[string](t, items[0], "status") != "CANCELLED" {
		t.Fatalf("cancelled plan not visible for restoration: %s", day["events"])
	}
}

func TestBusinessConcurrentConfirmationCreatesOneFact(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-concurrent")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	id := api.create(token, "INCOME", "2026-07-11", 870_000)
	const count = 12
	responses := make(chan *httptest.ResponseRecorder, count)
	start := make(chan struct{})
	var group sync.WaitGroup
	for i := 0; i < count; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			request := httptest.NewRequest(http.MethodPut, "/api/v1/events/"+id+"/occurrences/2026-07-11", bytes.NewBufferString(`{"status":"CONFIRMED"}`))
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			api.router.ServeHTTP(response, request)
			responses <- response
		}()
	}
	close(start)
	group.Wait()
	close(responses)
	for response := range responses {
		if response.Code != http.StatusOK {
			t.Errorf("concurrent confirmation: %d %s", response.Code, response.Body.String())
		}
	}
	api.balance(token, 1_870_000)
	day := api.days(token, "2026-07-11", "2026-07-11")[0]
	if got := len(businessField[[]json.RawMessage](t, day, "events")); got != 1 {
		t.Fatalf("confirmation created %d occurrences", got)
	}
}

func TestBusinessOccurrenceOwnershipAndInvalidConfirmations(t *testing.T) {
	api := newBusinessAPI(t)
	owner, other := api.login("business-fact-owner"), api.login("business-fact-other")
	api.anchor(owner, 1_000_000, "2026-07-11T08:00:00Z")
	id := api.create(owner, "INCOME", "2026-07-11", 870_000)
	api.occurrence(other, id, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusNotFound)
	api.occurrence(owner, "11111111-1111-4111-8111-111111111111", "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusNotFound)
	api.occurrence(owner, id, "2026-07-10", map[string]any{"status": "CONFIRMED"}, http.StatusNotFound)
	api.occurrence(owner, id, "2026-02-30", map[string]any{"status": "CONFIRMED"}, http.StatusBadRequest)
	api.occurrence(owner, id, "2026-07-11", map[string]any{"status": "UNKNOWN"}, http.StatusBadRequest)
	api.occurrence(owner, id, "2026-07-11", map[string]any{"status": "CONFIRMED", "effective_at": "2026-07-11T13:00:00Z"}, http.StatusBadRequest)
	api.occurrence(owner, id, "2026-07-11", map[string]any{"status": "CONFIRMED", "counts_toward_day_budget": true}, http.StatusBadRequest)
	future := api.create(owner, "INCOME", "2026-07-12", 100)
	api.occurrence(owner, future, "2026-07-12", map[string]any{"status": "CONFIRMED", "effective_at": "2026-07-11T12:00:00Z"}, http.StatusBadRequest)
	api.balance(owner, 1_000_000)
}

func TestBusinessSeriesEditAndDeletePreserveConfirmedSnapshot(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-snapshot")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	body := businessEvent(870_000, "2026-07-11")
	body["kind"] = "INCOME"
	body["recurrence"] = map[string]any{"type": "MONTHLY_ON_DAY", "start_date": "2026-07-11", "day_of_month": 11}
	created := api.call(http.MethodPost, "/events", token, body, http.StatusCreated)
	id := businessField[string](t, created, "id")
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.call(http.MethodPut, "/events/"+id, token, map[string]any{
		"amount": map[string]any{"amount": 990_000, "currency": "RUB"}, "comment": "New series value",
		"recurrence": map[string]any{"type": "MONTHLY_ON_DAY", "start_date": "2026-07-12", "day_of_month": 12},
	}, http.StatusOK)
	day := api.days(token, "2026-07-11", "2026-07-11")[0]
	items := businessField[[]map[string]json.RawMessage](t, day, "events")
	if len(items) != 1 || businessField[string](t, items[0], "comment") != "Independent QA" {
		t.Fatalf("series edit lost confirmed snapshot: %s", day["events"])
	}
	businessMoney(t, items[0], "amount", 870_000)
	repeated := api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	businessMoney(t, repeated, "amount", 870_000)
	api.balance(token, 1_870_000)
	future := api.days(token, "2026-08-12", "2026-08-12")[0]
	futureItems := businessField[[]map[string]json.RawMessage](t, future, "events")
	if len(futureItems) != 1 {
		t.Fatalf("future series occurrence absent: %s", future["events"])
	}
	businessMoney(t, futureItems[0], "amount", 990_000)
	api.call(http.MethodDelete, "/events/"+id, token, nil, http.StatusNoContent)
	day = api.days(token, "2026-07-11", "2026-07-11")[0]
	items = businessField[[]map[string]json.RawMessage](t, day, "events")
	if len(items) != 1 || !businessField[bool](t, items[0], "series_deleted") {
		t.Fatalf("deletion lost fact or deleted marker: %s", day["events"])
	}
	businessMoney(t, items[0], "amount", 870_000)
	api.balance(token, 1_870_000)
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "PLANNED"}, http.StatusConflict)
	future = api.days(token, "2026-08-12", "2026-08-12")[0]
	if got := len(businessField[[]json.RawMessage](t, future, "events")); got != 0 {
		t.Fatalf("deleted series still has future plans: %s", future["events"])
	}
}

func TestBusinessNextIncomeSkipsConfirmedAndCancelledOccurrences(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-horizon")
	api.settings(token, 40_000, "NEXT_INCOME", "")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	body := businessEvent(870_000, "2026-07-11")
	body["kind"] = "INCOME"
	body["recurrence"] = map[string]any{"type": "MONTHLY_ON_DAY", "start_date": "2026-07-11", "day_of_month": 11}
	created := api.call(http.MethodPost, "/events", token, body, http.StatusCreated)
	id := businessField[string](t, created, "id")
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	if day := businessField[string](t, forecast, "horizon_date"); day != "2026-07-11" {
		t.Fatalf("today's expected income not selected: %s", day)
	}
	businessMoney(t, forecast, "day_cost_remaining", 0)
	businessMoney(t, forecast, "money_for_life", 1_000_000)
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	forecast = api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	if day := businessField[string](t, forecast, "horizon_date"); day != "2026-08-11" {
		t.Fatalf("confirmed income remains horizon: %s", day)
	}
	businessMoney(t, forecast, "day_cost_remaining", 31*40_000)
	businessMoney(t, forecast, "money_for_life", 630_000)
	api.occurrence(token, id, "2026-08-11", map[string]any{"status": "CANCELLED"}, http.StatusOK)
	forecast = api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	if day := businessField[string](t, forecast, "horizon_date"); day != "2026-09-11" {
		t.Fatalf("cancelled income remains horizon: %s", day)
	}
}

func TestBusinessDayCostHistoryAndOverridePriority(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-cost-history")
	api.at("2026-07-10T12:00:00Z")
	api.settings(token, 40_000, "CUSTOM_DATE", "2026-07-12")
	api.anchor(token, 1_000_000, "2026-07-10T08:00:00Z")
	api.at("2026-07-11T12:00:00Z")
	api.settings(token, 60_000, "CUSTOM_DATE", "2026-07-12")
	api.call(http.MethodPost, "/day-cost-overrides", token, map[string]any{
		"date_from": "2026-07-12", "date_to": "2026-07-12", "daily_amount": map[string]any{"amount": 25_000, "currency": "RUB"}, "comment": "QA override",
	}, http.StatusCreated)
	days := api.days(token, "2026-07-10", "2026-07-12")
	businessMoney(t, days[0], "day_cost", 40_000)
	businessMoney(t, days[1], "day_cost", 60_000)
	businessMoney(t, days[2], "day_cost", 25_000)
	api.balance(token, 1_000_000)
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "day_cost_remaining", 85_000)
	businessMoney(t, forecast, "money_for_life", 915_000)
}

func TestBusinessUndoFactAfterRecurrenceEditDoesNotInventPlan(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-orphan-undo")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	id := api.create(token, "INCOME", "2026-07-11", 870_000)
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.call(http.MethodPut, "/events/"+id, token, map[string]any{
		"recurrence": map[string]any{"type": "NONE", "start_date": "2026-07-12"},
	}, http.StatusOK)
	api.balance(token, 1_870_000)
	for i := 0; i < 2; i++ {
		result := api.occurrence(token, id, "2026-07-11", map[string]any{"status": "PLANNED"}, http.StatusOK)
		if _, present := result["plan_restored"]; !present || businessField[bool](t, result, "plan_restored") {
			t.Fatalf("removed recurrence key must not restore a phantom plan: %v", result)
		}
	}
	api.balance(token, 1_000_000)
	day := api.days(token, "2026-07-11", "2026-07-11")[0]
	if got := len(businessField[[]json.RawMessage](t, day, "events")); got != 0 {
		t.Fatalf("orphaned fact became phantom plan: %s", day["events"])
	}
	day = api.days(token, "2026-07-12", "2026-07-12")[0]
	if got := len(businessField[[]json.RawMessage](t, day, "events")); got != 1 {
		t.Fatalf("current recurrence lost: %s", day["events"])
	}
}

func TestBusinessCreateAsFactRetriesOnlyConfirmation(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-two-step")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	id := api.create(token, "INCOME", "2026-07-11", 870_000)
	// Simulate a failed second step without another POST. The first event must
	// remain usable as a plan, and a retried PUT must not add another event.
	payload := map[string]any{"status": "CONFIRMED", "effective_at": "2026-07-11T12:00:00Z"}
	api.occurrence("invalid-session", id, "2026-07-11", payload, http.StatusUnauthorized)
	api.balance(token, 1_000_000)
	day := api.days(token, "2026-07-11", "2026-07-11")[0]
	items := businessField[[]map[string]json.RawMessage](t, day, "events")
	if len(items) != 1 || businessField[string](t, items[0], "status") != "PLANNED" {
		t.Fatalf("partial creation did not preserve plan: %s", day["events"])
	}
	api.occurrence(token, id, "2026-07-11", payload, http.StatusOK)
	// Reissue the same payload as after an unknown network outcome.
	api.occurrence(token, id, "2026-07-11", payload, http.StatusOK)
	api.balance(token, 1_870_000)
	listed := api.call(http.MethodGet, "/events", token, nil, http.StatusOK)
	if got := len(businessField[[]json.RawMessage](t, listed, "events")); got != 1 {
		t.Fatalf("confirmation retry duplicated series: %s", listed["events"])
	}
}

func TestBusinessOptionalPlansAndCalendarSummaryStayConsistent(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-summary")
	api.settings(token, 40_000, "CUSTOM_DATE", "2026-07-12")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	api.create(token, "INCOME", "2026-07-12", 500_000)
	api.create(token, "EXPENSE", "2026-07-12", 200_000)
	body := businessEvent(300_000, "2026-07-12")
	body["necessity"] = "OPTIONAL"
	api.call(http.MethodPost, "/events", token, body, http.StatusCreated)
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "money_for_life", 1_220_000)
	businessMoney(t, forecast, "optional_remaining", 300_000)
	businessMoney(t, forecast, "required_remaining", 200_000)
	for _, query := range []string{"from=2026-07-11&to=2026-07-12", "from=2026-08-01&to=2026-08-01"} {
		calendar := api.call(http.MethodGet, "/calendar?"+query, token, nil, http.StatusOK)
		summary := businessField[map[string]json.RawMessage](t, calendar, "summary")
		for _, key := range []string{"current_balance", "money_for_life", "horizon_date", "required_remaining", "day_cost_remaining", "optional_remaining", "days_until_horizon"} {
			if string(summary[key]) != string(forecast[key]) {
				t.Fatalf("calendar range %s changed summary %s: calendar=%s forecast=%s", query, key, summary[key], forecast[key])
			}
		}
	}
	days := api.days(token, "2026-07-11", "2026-07-12")
	businessMoney(t, days[0], "expected_balance", 960_000)
	businessMoney(t, days[0], "money_for_life", 1_220_000)
	businessMoney(t, days[1], "expected_balance", 920_000)
	businessMoney(t, days[1], "money_for_life", 920_000)
}

func TestBusinessPastDefaultExecutionUsesMoscowNoon(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-noon")
	api.anchor(token, 1_000_000, "2026-07-10T08:00:00Z")
	id := api.create(token, "EXPENSE", "2026-07-10", 150_000)
	result := api.occurrence(token, id, "2026-07-10", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	value := businessField[string](t, result, "effective_at")
	got, err := time.Parse(time.RFC3339, value)
	if err != nil || !got.Equal(time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("past default must be Moscow noon, got %q (err %v)", value, err)
	}
	api.balance(token, 850_000)
	// A later reconciliation includes this earlier financial fact even though
	// the record itself was created after that reconciliation's effective time.
	api.anchor(token, 1_000_000, "2026-07-10T10:00:00Z")
	api.balance(token, 1_000_000)
}

func TestBusinessMoneySafeIntegerAndOverflowRollback(t *testing.T) {
	const maxSafe int64 = 9_007_199_254_740_991
	api := newBusinessAPI(t)
	token := api.login("business-overflow")
	for _, amount := range []int64{maxSafe + 1, 9_223_372_036_854_775_807} {
		api.call(http.MethodPost, "/events", token, businessEvent(amount, "2026-07-11"), http.StatusBadRequest)
		api.call(http.MethodPost, "/balance/anchor", token, map[string]any{
			"amount": map[string]any{"amount": amount, "currency": "RUB"},
		}, http.StatusBadRequest)
	}
	api.anchor(token, maxSafe-100, "2026-07-11T08:00:00Z")
	id := api.create(token, "INCOME", "2026-07-11", 200)
	// An overflowing plan must not corrupt the independently known actual sum.
	api.balance(token, maxSafe-100)
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusConflict)
	problem := businessField[map[string]json.RawMessage](t, forecast, "error")
	if code := businessField[string](t, problem, "code"); code != "CALCULATION_OVERFLOW" {
		t.Fatalf("overflow error code: %q", code)
	}
	for i := 0; i < 2; i++ {
		result := api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusConflict)
		problem = businessField[map[string]json.RawMessage](t, result, "error")
		if code := businessField[string](t, problem, "code"); code != "CALCULATION_OVERFLOW" {
			t.Fatalf("confirmation overflow code: %q", code)
		}
		api.balance(token, maxSafe-100)
	}
	// Cancellation is allowed only for a plan; success proves the rejected
	// confirmation did not leave a committed fact behind.
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CANCELLED"}, http.StatusOK)
	forecast = api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "current_balance", maxSafe-100)
}

func TestBusinessHistoricalMoneyForLifeIncludesLaterFactsAndAnchorResets(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-history-mfl")
	api.settings(token, 0, "CUSTOM_DATE", "2026-07-12")
	api.anchor(token, 1_000_000, "2026-07-09T08:00:00Z")
	id := api.create(token, "INCOME", "2026-07-10", 870_000)
	api.occurrence(token, id, "2026-07-10", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	days := api.days(token, "2026-07-09", "2026-07-12")
	businessMoney(t, days[0], "expected_balance", 1_000_000)
	businessMoney(t, days[0], "money_for_life", 1_870_000)
	api.anchor(token, 2_000_000, "2026-07-11T08:00:00Z")
	days = api.days(token, "2026-07-09", "2026-07-12")
	businessMoney(t, days[0], "expected_balance", 1_000_000)
	businessMoney(t, days[0], "money_for_life", 2_000_000)
	businessMoney(t, days[1], "expected_balance", 1_870_000)
	businessMoney(t, days[1], "money_for_life", 2_000_000)
	businessMoney(t, days[2], "expected_balance", 2_000_000)
}

func TestBusinessInclusiveMonthEndAndExpiredCustomHorizon(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-month-end")
	api.at("2026-07-30T12:00:00Z")
	api.settings(token, 40_000, "END_OF_MONTH", "")
	api.anchor(token, 1_000_000, "2026-07-30T08:00:00Z")
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "day_cost_remaining", 80_000)
	businessMoney(t, forecast, "money_for_life", 920_000)
	if got := businessField[int](t, forecast, "days_until_horizon"); got != 2 {
		t.Fatalf("month end must include both remaining days: %d", got)
	}
	api.settings(token, 40_000, "CUSTOM_DATE", "2026-07-31")
	api.at("2026-08-01T12:00:00Z")
	api.balance(token, 1_000_000)
	forecast = api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "day_cost_remaining", 0)
	businessMoney(t, forecast, "money_for_life", 1_000_000)
	if got := businessField[int](t, forecast, "days_until_horizon"); got != 0 {
		t.Fatalf("expired custom horizon creates a nonzero period: %d", got)
	}
}

func TestBusinessCalendarRangeAndFutureAnchorValidation(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-ranges")
	days := api.days(token, "2026-07-11", "2027-07-11")
	if len(days) != 366 {
		t.Fatalf("inclusive 366-day range: got %d", len(days))
	}
	for _, query := range []string{
		"from=2026-07-11&to=2027-07-12",
		"from=2026-07-12&to=2026-07-11",
		"from=2026-02-30&to=2026-07-11",
	} {
		api.call(http.MethodGet, "/calendar?"+query, token, nil, http.StatusBadRequest)
	}
	api.call(http.MethodPost, "/balance/anchor", token, map[string]any{
		"amount": map[string]any{"amount": 1_000_000, "currency": "RUB"}, "anchored_at": "2026-07-11T12:00:01Z",
	}, http.StatusBadRequest)
	api.call(http.MethodGet, "/balance", token, nil, http.StatusConflict)
}

func TestBusinessRepeatFactUsesSnapshotKindAndBudgetAfterSeriesEdit(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-kind-snapshot")
	api.settings(token, 40_000, "CUSTOM_DATE", "2026-07-11")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	id := api.create(token, "EXPENSE", "2026-07-11", 15_000)
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED", "counts_toward_day_budget": true}, http.StatusOK)
	api.call(http.MethodPut, "/events/"+id, token, map[string]any{
		"kind": "INCOME", "amount": map[string]any{"amount": 870_000, "currency": "RUB"},
	}, http.StatusOK)
	result := api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED", "counts_toward_day_budget": true}, http.StatusOK)
	if kind := businessField[string](t, result, "kind"); kind != "EXPENSE" || !businessField[bool](t, result, "counts_toward_day_budget") {
		t.Fatalf("repeated confirmation uses edited series instead of saved expense: %v", result)
	}
	businessMoney(t, result, "amount", 15_000)
	api.balance(token, 985_000)
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "day_cost_remaining", 25_000)
	businessMoney(t, forecast, "money_for_life", 960_000)
}

func TestBusinessRequiredExpenseNeverChargesPlanAndFactTogether(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-expense-cycle")
	api.settings(token, 40_000, "CUSTOM_DATE", "2026-07-11")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	id := api.create(token, "EXPENSE", "2026-07-11", 150_000)
	api.balance(token, 1_000_000)
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "money_for_life", 810_000)
	businessMoney(t, forecast, "required_remaining", 150_000)
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.balance(token, 850_000)
	forecast = api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "money_for_life", 810_000)
	businessMoney(t, forecast, "required_remaining", 0)
	result := api.occurrence(token, id, "2026-07-11", map[string]any{"status": "PLANNED"}, http.StatusOK)
	if _, present := result["plan_restored"]; !present || !businessField[bool](t, result, "plan_restored") {
		t.Fatalf("unchanged recurrence must restore plan: %v", result)
	}
	api.balance(token, 1_000_000)
	forecast = api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "money_for_life", 810_000)
	businessMoney(t, forecast, "required_remaining", 150_000)
	api.occurrence(token, id, "2026-07-11", map[string]any{"status": "CANCELLED"}, http.StatusOK)
	forecast = api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "money_for_life", 960_000)
}

func TestBusinessEqualTimeOverridesAreStableAcrossReadsAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "override-order.db")
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	api, store := persistentBusinessAPI(t, path, &now)
	token := api.login("business-override-tie")
	api.settings(token, 40_000, "CUSTOM_DATE", "2026-07-12")
	api.anchor(token, 1_000_000, "2026-07-11T08:00:00Z")
	for _, amount := range []int64{25_000, 15_000} {
		api.call(http.MethodPost, "/day-cost-overrides", token, map[string]any{
			"date_from": "2026-07-11", "date_to": "2026-07-11", "daily_amount": map[string]any{"amount": amount, "currency": "RUB"},
		}, http.StatusCreated)
	}
	day := api.days(token, "2026-07-11", "2026-07-11")[0]
	amount := businessField[map[string]json.RawMessage](t, day, "day_cost")
	winner := businessField[int64](t, amount, "amount")
	// The contract requires a stable tie-break, without prescribing which tied
	// override wins. Base cost is never a valid result while either applies.
	if winner != 25_000 && winner != 15_000 {
		t.Fatalf("tied overrides did not override base cost: %d", winner)
	}
	for i := 0; i < 3; i++ {
		days := api.days(token, "2026-07-10", "2026-07-12")
		businessMoney(t, days[1], "day_cost", winner)
		businessMoney(t, days[2], "day_cost", 40_000)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	api, store = persistentBusinessAPI(t, path, &now)
	day = api.days(token, "2026-07-11", "2026-07-11")[0]
	businessMoney(t, day, "day_cost", winner)
	api.at("2026-07-11T13:00:00Z")
	api.call(http.MethodPost, "/day-cost-overrides", token, map[string]any{
		"date_from": "2026-07-11", "date_to": "2026-07-11", "daily_amount": map[string]any{"amount": 10_000, "currency": "RUB"},
	}, http.StatusCreated)
	day = api.days(token, "2026-07-11", "2026-07-11")[0]
	businessMoney(t, day, "day_cost", 10_000)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBusinessLiveLedgerOrderSurvivesReopenAtEqualTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live-ledger-order.db")
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	api, store := persistentBusinessAPI(t, path, &now)
	token := api.login("business-live-reopen")
	first := api.create(token, "INCOME", "2026-07-11", 100)
	api.occurrence(token, first, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.anchor(token, 1_000, "2026-07-11T12:00:00Z")
	second := api.create(token, "INCOME", "2026-07-11", 200)
	api.occurrence(token, second, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.balance(token, 1_200)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	api, store = persistentBusinessAPI(t, path, &now)
	api.balance(token, 1_200)
	third := api.create(token, "INCOME", "2026-07-11", 300)
	api.occurrence(token, third, "2026-07-11", map[string]any{"status": "CONFIRMED"}, http.StatusOK)
	api.balance(token, 1_500)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	api, store = persistentBusinessAPI(t, path, &now)
	api.balance(token, 1_500)
	day := api.days(token, "2026-07-11", "2026-07-11")[0]
	businessMoney(t, day, "expected_balance", 1_500)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBusinessMoneyLimitsOnSettingsOverridesAndEventUpdate(t *testing.T) {
	const maxSafe int64 = 9_007_199_254_740_991
	api := newBusinessAPI(t)
	token := api.login("business-money-dtos")
	api.settings(token, 0, "CUSTOM_DATE", "2026-07-11")
	api.anchor(token, 0, "2026-07-11T08:00:00Z")
	id := api.create(token, "EXPENSE", "2026-07-11", 100)
	for _, amount := range []int64{-1, maxSafe + 1} {
		api.call(http.MethodPut, "/settings", token, map[string]any{
			"base_day_cost": map[string]any{"amount": amount, "currency": "RUB"}, "horizon_policy": "CUSTOM_DATE", "horizon_custom_date": "2026-07-11",
		}, http.StatusBadRequest)
		api.call(http.MethodPost, "/day-cost-overrides", token, map[string]any{
			"date_from": "2026-07-11", "date_to": "2026-07-11", "daily_amount": map[string]any{"amount": amount, "currency": "RUB"},
		}, http.StatusBadRequest)
		api.call(http.MethodPut, "/events/"+id, token, map[string]any{
			"amount": map[string]any{"amount": amount, "currency": "RUB"},
		}, http.StatusBadRequest)
	}
	settings := api.call(http.MethodGet, "/settings", token, nil, http.StatusOK)
	businessMoney(t, settings, "base_day_cost", 0)
	read := api.call(http.MethodGet, "/events/"+id, token, nil, http.StatusOK)
	businessMoney(t, read, "amount", 100)
	day := api.days(token, "2026-07-11", "2026-07-11")[0]
	businessMoney(t, day, "day_cost", 0)
	api.create(token, "INCOME", "2026-07-11", maxSafe)
	forecast := api.call(http.MethodGet, "/forecast", token, nil, http.StatusOK)
	businessMoney(t, forecast, "money_for_life", maxSafe-100)
	api.balance(token, 0)
}
