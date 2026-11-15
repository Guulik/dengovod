package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/repository/sqlite"
	"github.com/alshatilov/dengovod/backend/internal/service"
)

func TestCalendarFutureBoundIsExplicitValidationError(t *testing.T) {
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := service.New(store)
	app.Now = func() time.Time { return time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC) }
	_, token, err := app.Login(context.Background(), "own-bounds")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(app)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/calendar?from=9999-12-31&to=9999-12-31", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusBadRequest || body.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("unbounded future response: %d %s", response.Code, response.Body.String())
	}
}

func TestRequestDeadlineBoundsWorkAndKeepsEarlierCallerDeadline(t *testing.T) {
	for _, parentDeadline := range []bool{false, true} {
		ctx := context.Background()
		var cancel context.CancelFunc
		if parentDeadline {
			ctx, cancel = context.WithTimeout(ctx, time.Second)
			defer cancel()
		}
		var handlerContext context.Context
		requestDeadline(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerContext = r.Context()
			deadline, ok := handlerContext.Deadline()
			if !ok {
				t.Fatal("request has no deadline")
			}
			limit := 5 * time.Second
			if parentDeadline {
				limit = time.Second
			}
			if deadline.After(time.Now().Add(limit)) {
				t.Fatalf("deadline exceeded bound: %v", deadline)
			}
		})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
		if handlerContext.Err() != context.Canceled {
			t.Fatal("request context was not released after completion")
		}
	}
}
