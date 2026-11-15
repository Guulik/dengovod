package httptransport_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/repository/sqlite"
	"github.com/alshatilov/dengovod/backend/internal/service"
	httptransport "github.com/alshatilov/dengovod/backend/internal/transport/http"
)

// These tests exercise the public HTTP boundary and a real, isolated SQLite file.
// Financial DTOs awaiting approval are intentionally absent from this baseline.
type businessAPI struct {
	t      *testing.T
	router http.Handler
	now    *time.Time
}

func newBusinessAPI(t *testing.T) businessAPI {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	app := service.New(store)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	app.Now = func() time.Time { return now }
	return businessAPI{t: t, router: httptransport.NewRouter(app), now: &now}
}

func persistentBusinessAPI(t *testing.T, path string, now *time.Time) (businessAPI, *sqlite.Store) {
	t.Helper()
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Tests explicitly close each handle before reopening. Cleanup also covers
	// assertion failures, and a second Close of an already closed handle is safe.
	t.Cleanup(func() { _ = store.Close() })
	app := service.New(store)
	app.Now = func() time.Time { return *now }
	return businessAPI{t: t, router: httptransport.NewRouter(app), now: now}, store
}

func (api businessAPI) call(method, path, token string, body any, want int) map[string]json.RawMessage {
	api.t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			api.t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	api.router.ServeHTTP(response, request)
	if response.Code != want {
		api.t.Fatalf("%s %s: got %d, want %d: %s", method, path, response.Code, want, response.Body.String())
	}
	if want == http.StatusNoContent {
		return nil
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		api.t.Fatalf("%s %s: invalid JSON: %v", method, path, err)
	}
	return result
}

func businessField[T any](t *testing.T, value map[string]json.RawMessage, key string) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(value[key], &result); err != nil {
		t.Fatalf("field %q: %v", key, err)
	}
	return result
}

func (api businessAPI) login(login string) string {
	api.t.Helper()
	result := api.call(http.MethodPost, "/auth/login", "", map[string]any{"login": login}, http.StatusOK)
	return businessField[string](api.t, result, "token")
}

func businessEvent(amount int64, day string) map[string]any {
	return map[string]any{
		"kind": "EXPENSE", "necessity": "REQUIRED", "comment": "Independent QA",
		"amount":     map[string]any{"amount": amount, "currency": "RUB"},
		"recurrence": map[string]any{"type": "NONE", "start_date": day},
	}
}

func TestBusinessPastOperationCRUDWithoutAnchor(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-past")
	for _, path := range []string{"/balance", "/forecast"} {
		result := api.call(http.MethodGet, path, token, nil, http.StatusConflict)
		problem := businessField[map[string]json.RawMessage](t, result, "error")
		if code := businessField[string](t, problem, "code"); code != "ANCHOR_REQUIRED" {
			t.Fatalf("%s returned error %q", path, code)
		}
	}
	created := api.call(http.MethodPost, "/events", token, businessEvent(150_000, "2026-07-10"), http.StatusCreated)
	id := businessField[string](t, created, "id")
	path := "/events/" + id
	read := api.call(http.MethodGet, path, token, nil, http.StatusOK)
	recurrence := businessField[map[string]json.RawMessage](t, read, "recurrence")
	if day := businessField[string](t, recurrence, "start_date"); day != "2026-07-10" {
		t.Fatalf("past date changed: %s", day)
	}
	updated := api.call(http.MethodPut, path, token, map[string]any{
		"amount": map[string]any{"amount": 160_001, "currency": "RUB"}, "comment": "Edited past operation",
	}, http.StatusOK)
	amount := businessField[map[string]json.RawMessage](t, updated, "amount")
	if got := businessField[int64](t, amount, "amount"); got != 160_001 {
		t.Fatalf("updated amount: %d", got)
	}
	listed := api.call(http.MethodGet, "/events", token, nil, http.StatusOK)
	items := businessField[[]map[string]json.RawMessage](t, listed, "events")
	if len(items) != 1 || businessField[string](t, items[0], "id") != id {
		t.Fatalf("past operation missing from list: %s", listed["events"])
	}
	api.call(http.MethodDelete, path, token, nil, http.StatusNoContent)
	api.call(http.MethodGet, path, token, nil, http.StatusNotFound)
	listed = api.call(http.MethodGet, "/events", token, nil, http.StatusOK)
	if got := len(businessField[[]json.RawMessage](t, listed, "events")); got != 0 {
		t.Fatalf("deleted operation remains: %s", listed["events"])
	}
}

func TestBusinessForeignUserCannotReadOrMutateOperations(t *testing.T) {
	api := newBusinessAPI(t)
	owner, other := api.login("business-owner"), api.login("business-other")
	created := api.call(http.MethodPost, "/events", owner, businessEvent(123_456, "2026-07-10"), http.StatusCreated)
	path := "/events/" + businessField[string](t, created, "id")
	api.call(http.MethodGet, path, other, nil, http.StatusNotFound)
	api.call(http.MethodPut, path, other, map[string]any{"comment": "unauthorized edit"}, http.StatusNotFound)
	api.call(http.MethodDelete, path, other, nil, http.StatusNotFound)
	read := api.call(http.MethodGet, path, owner, nil, http.StatusOK)
	if comment := businessField[string](t, read, "comment"); comment != "Independent QA" {
		t.Fatalf("foreign mutation changed owner data: %q", comment)
	}
	listed := api.call(http.MethodGet, "/events", other, nil, http.StatusOK)
	if got := len(businessField[[]json.RawMessage](t, listed, "events")); got != 0 {
		t.Fatalf("foreign events leaked: %s", listed["events"])
	}
	categories := api.call(http.MethodGet, "/categories", owner, nil, http.StatusOK)
	items := businessField[[]map[string]json.RawMessage](t, categories, "categories")
	if len(items) == 0 {
		t.Fatal("owner has no default categories")
	}
	input := businessEvent(100, "2026-07-10")
	input["category_id"] = businessField[string](t, items[0], "id")
	api.call(http.MethodPost, "/events", other, input, http.StatusBadRequest)
	listed = api.call(http.MethodGet, "/events", other, nil, http.StatusOK)
	if got := len(businessField[[]json.RawMessage](t, listed, "events")); got != 0 {
		t.Fatal("rejected foreign-category operation was persisted")
	}
}

func TestBusinessRejectedOperationDoesNotPersist(t *testing.T) {
	api := newBusinessAPI(t)
	token := api.login("business-validation")
	for _, amount := range []int64{0, -1} {
		t.Run(fmt.Sprintf("amount_%d", amount), func(t *testing.T) {
			subAPI := api
			subAPI.t = t
			result := subAPI.call(http.MethodPost, "/events", token, businessEvent(amount, "2026-07-10"), http.StatusBadRequest)
			problem := businessField[map[string]json.RawMessage](t, result, "error")
			if code := businessField[string](t, problem, "code"); code != "VALIDATION_ERROR" {
				t.Fatalf("unexpected error: %q", code)
			}
		})
	}
	api.call(http.MethodPost, "/events", token, businessEvent(100, "2026-02-29"), http.StatusBadRequest)
	listed := api.call(http.MethodGet, "/events", token, nil, http.StatusOK)
	if got := len(businessField[[]json.RawMessage](t, listed, "events")); got != 0 {
		t.Fatalf("rejected operations persisted: %s", listed["events"])
	}
}

func TestBusinessPrivateEndpointsRequireIdentity(t *testing.T) {
	api := newBusinessAPI(t)
	for _, token := range []string{"", "not-a-uuid", "11111111-1111-4111-8111-111111111111"} {
		for _, path := range []string{"/events", "/settings", "/categories", "/balance", "/forecast", "/calendar?from=2026-07-10&to=2026-07-11"} {
			api.call(http.MethodGet, path, token, nil, http.StatusUnauthorized)
		}
		api.call(http.MethodPost, "/events", token, businessEvent(100, "2026-07-10"), http.StatusUnauthorized)
	}
}
