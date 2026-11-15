package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alshatilov/dengovod/backend/internal/repository/sqlite"
	"github.com/alshatilov/dengovod/backend/internal/service"
)

func TestCategoryDTOIncludesNullableSystemCode(t *testing.T) {
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := service.New(store)
	user, token, err := app.Login(context.Background(), "salary-dto")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(app)
	for _, filter := range []string{"", "?kind=INCOME", "?kind=EXPENSE"} {
		t.Run(filter, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/categories"+filter, nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("categories: %d %s", response.Code, response.Body.String())
			}
			var body struct {
				Categories []map[string]json.RawMessage `json:"categories"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			wantCount, wantSalary := 9, 1
			if filter == "?kind=INCOME" {
				wantCount = 3
			}
			if filter == "?kind=EXPENSE" {
				wantCount, wantSalary = 6, 0
			}
			if len(body.Categories) != wantCount {
				t.Fatalf("got %d categories, want %d", len(body.Categories), wantCount)
			}
			salaries := 0
			for _, category := range body.Categories {
				code, exists := category["system_code"]
				if !exists {
					t.Fatal("system_code omitted instead of explicit null")
				}
				if string(code) == `"SALARY"` {
					salaries++
					if string(category["name"]) != `"Зарплата"` || string(category["kind"]) != `"INCOME"` || string(category["is_default"]) != `true` {
						t.Fatalf("salary DTO: %s", category)
					}
				} else if string(code) != "null" {
					t.Fatalf("unexpected system code %s", code)
				}
			}
			if salaries != wantSalary {
				t.Fatalf("got %d salaries, want %d", salaries, wantSalary)
			}
		})
	}
	// The added DTO field does not change repository identifiers.
	categories, err := store.ListCategories(context.Background(), user.ID)
	if err != nil || len(categories) != 9 {
		t.Fatalf("categories: %+v, %v", categories, err)
	}
}
