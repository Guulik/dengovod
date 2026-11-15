package sqlite

import (
	"context"
	"testing"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/google/uuid"
)

func TestProvisionUserAssignsStableSalaryIdentityPerUser(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	salaryIDs := map[uuid.UUID]bool{}
	for _, login := range []string{"alice", "bob"} {
		user, err := store.ProvisionUser(ctx, login)
		if err != nil {
			t.Fatal(err)
		}
		categories, err := store.ListCategories(ctx, user.ID)
		if err != nil || len(categories) != 9 {
			t.Fatalf("default categories: %+v, %v", categories, err)
		}
		salaries := 0
		for _, category := range categories {
			if category.UserID != user.ID {
				t.Fatalf("cross-user category: %+v", category)
			}
			if category.SystemCode != nil {
				salaries++
				if *category.SystemCode != domain.CategorySystemSalary || category.Kind != domain.EventKindIncome || category.Name != "Зарплата" {
					t.Fatalf("incorrect system category: %+v", category)
				}
				if salaryIDs[category.ID] {
					t.Fatal("users share a salary UUID")
				}
				salaryIDs[category.ID] = true
			}
		}
		if salaries != 1 {
			t.Fatalf("got %d system categories, want 1", salaries)
		}
		before := snapshot(t, store.db, `SELECT * FROM categories ORDER BY id`)
		if _, err := store.ProvisionUser(ctx, login); err != nil {
			t.Fatal(err)
		}
		after := snapshot(t, store.db, `SELECT * FROM categories ORDER BY id`)
		if len(before) != len(after) {
			t.Fatal("repeated provisioning duplicated categories")
		}
		for i := range before {
			if before[i] != after[i] {
				t.Fatal("repeated provisioning changed a category")
			}
		}
		if _, err := store.db.Exec(`INSERT INTO categories(id,user_id,name,kind,is_default,created_at,system_code)
            VALUES(?,?,'Second salary','INCOME',0,CURRENT_TIMESTAMP,'SALARY')`, uuid.New(), user.ID); err == nil {
			t.Fatal("duplicate salary identity accepted for one user")
		}
		if _, err := store.db.Exec(`INSERT INTO categories(id,user_id,name,kind,is_default,created_at,system_code)
            VALUES(?,?,'Expense salary','EXPENSE',0,CURRENT_TIMESTAMP,'SALARY')`, uuid.New(), user.ID); err == nil {
			t.Fatal("expense category accepted with salary code")
		}
	}
}
