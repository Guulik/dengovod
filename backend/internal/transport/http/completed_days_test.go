package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/repository/sqlite"
	"github.com/alshatilov/dengovod/backend/internal/service"
)

type elapsedAPI struct {
	t      *testing.T
	router http.Handler
	token  string
	now    *time.Time
}

func elapsedAt(value string) time.Time {
	v, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return v
}
func newElapsedAPI(t *testing.T, now string) elapsedAPI {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "completed-days.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	clock := elapsedAt(now)
	app := service.New(store)
	app.Now = func() time.Time { return clock }
	_, token, err := app.Login(context.Background(), "own-completed-days")
	if err != nil {
		t.Fatal(err)
	}
	return elapsedAPI{t: t, router: NewRouter(app), token: token, now: &clock}
}
func (a elapsedAPI) call(method, path string, body any, status int) map[string]json.RawMessage {
	a.t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(data))
	r.Header.Set("Authorization", "Bearer "+a.token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, r)
	if w.Code != status {
		a.t.Fatalf("%s %s: %d %s, want %d", method, path, w.Code, w.Body.String(), status)
	}
	if status == http.StatusNoContent {
		return nil
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		a.t.Fatal(err)
	}
	return result
}
func elapsedField[T any](t *testing.T, body map[string]json.RawMessage, key string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(body[key], &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func elapsedMoney(amount int64) map[string]any {
	return map[string]any{"amount": amount, "currency": "RUB"}
}
func (a elapsedAPI) amount(body map[string]json.RawMessage, key string, want int64) {
	a.t.Helper()
	v := elapsedField[moneyDTO](a.t, body, key)
	if v.Amount != want {
		a.t.Fatalf("%s=%d, want %d", key, v.Amount, want)
	}
}
func (a elapsedAPI) balance(want int64) {
	a.t.Helper()
	a.amount(a.call(http.MethodGet, "/balance", nil, http.StatusOK), "computed_balance", want)
}
func (a elapsedAPI) settings(cost int64) {
	a.t.Helper()
	a.call(http.MethodPut, "/settings", map[string]any{"base_day_cost": elapsedMoney(cost), "horizon_policy": "CUSTOM_DATE", "horizon_custom_date": "2026-11-18"}, http.StatusOK)
}
func (a elapsedAPI) anchor(amount int64, at string) {
	a.t.Helper()
	a.call(http.MethodPost, "/balance/anchor", map[string]any{"amount": elapsedMoney(amount), "anchored_at": at}, http.StatusCreated)
}
func (a elapsedAPI) expense(amount int64) string {
	a.t.Helper()
	body := a.call(http.MethodPost, "/events", map[string]any{"kind": "EXPENSE", "necessity": "REQUIRED", "amount": elapsedMoney(amount), "recurrence": map[string]any{"type": "NONE", "start_date": "2026-11-16"}}, http.StatusCreated)
	return elapsedField[string](a.t, body, "id")
}

func TestCompletedDaysHTTPBalancesForecastAndCalendarUseSameEstimate(t *testing.T) {
	a := newElapsedAPI(t, "2026-11-16T09:00:00Z")
	a.settings(140000)
	a.anchor(2000000, "2026-11-16T06:00:00Z")
	a.balance(2000000)
	id := a.expense(20000)
	path := "/events/" + id + "/occurrences/2026-11-16"
	confirm := map[string]any{"status": "CONFIRMED", "effective_at": "2026-11-16T07:00:00Z", "counts_toward_day_budget": true}
	a.call(http.MethodPut, path, confirm, http.StatusOK)
	a.balance(1980000)
	*a.now = elapsedAt("2026-11-16T21:00:00Z")
	for repeat := 0; repeat < 2; repeat++ {
		a.balance(1860000)
	}
	forecast := a.call(http.MethodGet, "/forecast", nil, http.StatusOK)
	a.amount(forecast, "current_balance", 1860000)
	a.amount(forecast, "money_for_life", 1580000)
	calendar := a.call(http.MethodGet, "/calendar?from=2026-11-16&to=2026-11-18", nil, http.StatusOK)
	days := elapsedField[[]map[string]json.RawMessage](t, calendar, "days")
	for i, want := range []int64{1860000, 1720000, 1580000} {
		a.amount(days[i], "expected_balance", want)
		a.amount(days[i], "money_for_life", 1580000)
	}
	for _, query := range []string{"/calendar?from=2026-10-01&to=2026-10-02", "/calendar?from=2026-11-16&to=2026-11-16"} {
		calendar = a.call(http.MethodGet, query, nil, http.StatusOK)
		summary := elapsedField[map[string]json.RawMessage](t, calendar, "summary")
		for _, key := range []string{"current_balance", "money_for_life", "day_cost_remaining"} {
			if string(summary[key]) != string(forecast[key]) {
				t.Fatalf("visible range changed summary %s", key)
			}
		}
	}
	overrideBody := map[string]any{"date_from": "2026-11-16", "date_to": "2026-11-16", "daily_amount": elapsedMoney(100000)}
	override := a.call(http.MethodPost, "/day-cost-overrides", overrideBody, http.StatusCreated)
	overridePath := "/day-cost-overrides/" + elapsedField[string](t, override, "id")
	a.balance(1900000)
	overrideBody["daily_amount"] = elapsedMoney(0)
	a.call(http.MethodPut, overridePath, overrideBody, http.StatusOK)
	a.balance(1980000)
	a.call(http.MethodDelete, overridePath, nil, http.StatusNoContent)
	a.balance(1860000)
	a.call(http.MethodPut, path, map[string]any{"status": "PLANNED"}, http.StatusOK)
	a.balance(1860000)
	a.call(http.MethodPut, path, confirm, http.StatusOK)
	a.call(http.MethodPut, path, confirm, http.StatusOK)
	a.balance(1860000)
	a.settings(200000)
	a.balance(1860000)
	calendar = a.call(http.MethodGet, "/calendar?from=2026-11-16&to=2026-11-17", nil, http.StatusOK)
	days = elapsedField[[]map[string]json.RawMessage](t, calendar, "days")
	a.amount(days[0], "day_cost", 140000)
	a.amount(days[1], "day_cost", 200000)
	a.anchor(1500000, "2026-11-16T21:00:00Z")
	a.balance(1500000)
	*a.now = elapsedAt("2026-11-17T21:00:00Z")
	a.balance(1300000)
	overrideBody["daily_amount"] = elapsedMoney(1)
	a.call(http.MethodPost, "/day-cost-overrides", overrideBody, http.StatusCreated)
	a.balance(1300000)
	forecast = a.call(http.MethodGet, "/forecast", nil, http.StatusOK)
	a.amount(forecast, "current_balance", 1300000)
	a.amount(forecast, "money_for_life", 1100000)
}

func TestCompletedDaysHTTPMovedFactUndoAndBackwardAsOf(t *testing.T) {
	a := newElapsedAPI(t, "2026-11-17T12:00:00Z")
	a.settings(140000)
	// The last known base cost starts today. A dated override specifies yesterday.
	a.call(http.MethodPost, "/day-cost-overrides", map[string]any{"date_from": "2026-11-16", "date_to": "2026-11-16", "daily_amount": elapsedMoney(140000)}, http.StatusCreated)
	a.anchor(2000000, "2026-11-16T06:00:00Z")
	a.balance(1860000)
	id := a.expense(200000)
	path := "/events/" + id + "/occurrences/2026-11-16"
	confirm := map[string]any{"status": "CONFIRMED", "effective_at": "2026-11-17T07:00:00Z", "counts_toward_day_budget": true}
	a.call(http.MethodPut, path, confirm, http.StatusOK)
	a.balance(1660000)
	*a.now = elapsedAt("2026-11-17T21:00:00Z")
	a.balance(1660000)
	calendar := a.call(http.MethodGet, "/calendar?from=2026-11-16&to=2026-11-17", nil, http.StatusOK)
	days := elapsedField[[]map[string]json.RawMessage](t, calendar, "days")
	a.amount(days[0], "expected_balance", 1860000)
	a.amount(days[1], "expected_balance", 1660000)
	if events := elapsedField[[]json.RawMessage](t, days[0], "events"); len(events) != 0 {
		t.Fatal("moved fact still shown on original date")
	}
	a.call(http.MethodPut, path, map[string]any{"status": "PLANNED"}, http.StatusOK)
	a.balance(1720000)
	a.call(http.MethodPut, path, confirm, http.StatusOK)
	a.balance(1660000)
	*a.now = elapsedAt("2026-11-16T09:00:00Z")
	a.balance(2000000)
}

func TestCompletedDaysHTTPOverrideCreationPrioritySurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "override-order.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	now := elapsedAt("2026-11-16T09:00:00Z")
	newApp := func() *service.App { app := service.New(store); app.Now = func() time.Time { return now }; return app }
	app := newApp()
	_, token, err := app.Login(context.Background(), "override-order")
	if err != nil {
		t.Fatal(err)
	}
	a := elapsedAPI{t: t, router: NewRouter(app), token: token, now: &now}
	a.settings(140000)
	a.anchor(2000000, "2026-11-16T06:00:00Z")
	post := func(from, to string, cost int64) map[string]json.RawMessage {
		return a.call(http.MethodPost, "/day-cost-overrides", map[string]any{"date_from": from, "date_to": to, "daily_amount": elapsedMoney(cost)}, http.StatusCreated)
	}
	old := post("2026-11-16", "2026-11-18", 140000)
	post("2026-11-16", "2026-11-16", 100000)
	assertCost := func(want int64) {
		t.Helper()
		calendar := a.call(http.MethodGet, "/calendar?from=2026-11-16&to=2026-11-16", nil, http.StatusOK)
		days := elapsedField[[]map[string]json.RawMessage](t, calendar, "days")
		a.amount(days[0], "day_cost", want)
	}
	assertCost(100000)
	now = elapsedAt("2026-11-16T08:00:00Z")
	newest := post("2026-11-16", "2026-11-16", 80000)
	if _, ok := newest["priority"]; ok {
		t.Fatal("internal priority leaked into public DTO")
	}
	assertCost(80000)
	oldPath := "/day-cost-overrides/" + elapsedField[string](t, old, "id")
	a.call(http.MethodPut, oldPath, map[string]any{"date_from": "2026-11-16", "date_to": "2026-11-18", "daily_amount": elapsedMoney(200000)}, http.StatusOK)
	assertCost(80000)
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a.router = NewRouter(newApp())
	now = elapsedAt("2026-11-16T21:00:00Z")
	for repeat := 0; repeat < 3; repeat++ {
		assertCost(80000)
		a.balance(1920000)
	}
}
