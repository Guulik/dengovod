package httptransport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/repository/sqlite"
	"github.com/alshatilov/dengovod/backend/internal/service"
)

func TestHTTPVerticalSlice(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := service.New(store)
	app.Now = func() time.Time { return time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC) }
	router := NewRouter(app)

	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(data))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	decodeBody := func(response *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var value map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	login := call(http.MethodPost, "/api/v1/auth/login", "", map[string]any{"login": "ivan"})
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	token := decodeBody(login)["token"].(string)

	missing := call(http.MethodGet, "/api/v1/forecast", token, nil)
	if missing.Code != http.StatusConflict {
		t.Fatalf("missing anchor: %d %s", missing.Code, missing.Body.String())
	}
	errorObject := decodeBody(missing)["error"].(map[string]any)
	if errorObject["code"] != "ANCHOR_REQUIRED" {
		t.Fatalf("unexpected error: %v", errorObject)
	}

	settings := call(http.MethodPut, "/api/v1/settings", token, map[string]any{
		"base_day_cost":  map[string]any{"amount": 100, "currency": "RUB"},
		"horizon_policy": "CUSTOM_DATE", "horizon_custom_date": "2026-07-11",
	})
	if settings.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", settings.Code, settings.Body.String())
	}

	anchor := call(http.MethodPost, "/api/v1/balance/anchor", token, map[string]any{
		"amount":      map[string]any{"amount": 10_000, "currency": "RUB"},
		"anchored_at": "2026-07-10T09:00:00Z",
	})
	if anchor.Code != http.StatusCreated {
		t.Fatalf("anchor: %d %s", anchor.Code, anchor.Body.String())
	}

	before := call(http.MethodGet, "/api/v1/forecast", token, nil)
	if before.Code != http.StatusOK {
		t.Fatalf("forecast: %d %s", before.Code, before.Body.String())
	}
	beforeMFL := decodeBody(before)["money_for_life"].(map[string]any)["amount"].(float64)

	created := call(http.MethodPost, "/api/v1/events", token, map[string]any{
		"kind": "EXPENSE", "necessity": "REQUIRED",
		"amount":     map[string]any{"amount": 500, "currency": "RUB"},
		"comment":    "rent",
		"recurrence": map[string]any{"type": "NONE", "start_date": "2026-07-11"},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("event: %d %s", created.Code, created.Body.String())
	}

	after := call(http.MethodGet, "/api/v1/forecast", token, nil)
	if after.Code != http.StatusOK {
		t.Fatalf("forecast after: %d %s", after.Code, after.Body.String())
	}
	afterMFL := decodeBody(after)["money_for_life"].(map[string]any)["amount"].(float64)
	if beforeMFL-afterMFL != 500 {
		t.Fatalf("event did not affect MFL: before=%v after=%v", beforeMFL, afterMFL)
	}

	calendar := call(http.MethodGet, "/api/v1/calendar?from=2026-07-10&to=2026-07-11", token, nil)
	if calendar.Code != http.StatusOK {
		t.Fatalf("calendar: %d %s", calendar.Code, calendar.Body.String())
	}
	days := decodeBody(calendar)["days"].([]any)
	if len(days) != 2 || len(days[1].(map[string]any)["events"].([]any)) != 1 {
		t.Fatalf("unexpected calendar: %s", calendar.Body.String())
	}
}

func TestPastDayEventAffectsBalanceAndCalendar(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "past.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := service.New(store)
	app.Now = func() time.Time { return time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC) }
	router := NewRouter(app)

	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(data))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	decodeBody := func(response *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var value map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	login := call(http.MethodPost, "/api/v1/auth/login", "", map[string]any{"login": "anna"})
	token := decodeBody(login)["token"].(string)

	if code := call(http.MethodPost, "/api/v1/balance/anchor", token, map[string]any{
		"amount": map[string]any{"amount": 10_000, "currency": "RUB"}, "anchored_at": "2026-07-11T08:00:00Z",
	}).Code; code != http.StatusCreated {
		t.Fatalf("anchor: %d", code)
	}

	if code := call(http.MethodPost, "/api/v1/events", token, map[string]any{
		"kind": "EXPENSE", "necessity": "REQUIRED",
		"amount": map[string]any{"amount": 1_500, "currency": "RUB"}, "comment": "yesterday",
		"recurrence": map[string]any{"type": "NONE", "start_date": "2026-07-10"},
	}).Code; code != http.StatusCreated {
		t.Fatalf("event: %d", code)
	}

	balance := call(http.MethodGet, "/api/v1/balance", token, nil)
	if balance.Code != http.StatusOK {
		t.Fatalf("balance: %d %s", balance.Code, balance.Body.String())
	}
	if got := decodeBody(balance)["computed_balance"].(map[string]any)["amount"].(float64); got != 8_500 {
		t.Fatalf("computed balance: got %v want 8500", got)
	}

	calendar := call(http.MethodGet, "/api/v1/calendar?from=2026-07-09&to=2026-07-11", token, nil)
	if calendar.Code != http.StatusOK {
		t.Fatalf("calendar: %d %s", calendar.Code, calendar.Body.String())
	}
	days := decodeBody(calendar)["days"].([]any)
	if len(days) != 3 {
		t.Fatalf("expected 3 calendar days, got %d: %s", len(days), calendar.Body.String())
	}
	past := days[1].(map[string]any)
	if past["date"] != "2026-07-10" {
		t.Fatalf("expected past day 2026-07-10, got %v", past["date"])
	}
	if past["expected_balance"].(map[string]any)["amount"].(float64) != 8_500 {
		t.Fatalf("past day balance: %+v", past["expected_balance"])
	}
	if len(past["events"].([]any)) != 1 {
		t.Fatalf("past day should list the expense: %+v", past["events"])
	}
}
