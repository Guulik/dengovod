package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/alshatilov/dengovod/backend/internal/domain/forecast"
	"github.com/google/uuid"
)

func TestCompletedDaysSnapshotReadsArePersistentAndDoNotWriteFacts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "estimated.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	u, err := s.ProvisionUser(ctx, "estimated-snapshot")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := s.GetSettings(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	settings.BaseDayCost = domain.Rub(140000)
	settings.UpdatedAt = ledgerNow.AddDate(0, 0, -1)
	if err = s.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	ledgerAnchor(t, s, u, 2000000, ledgerNow.AddDate(0, 0, -1))
	o := domain.DayCostOverride{ID: uuid.New(), UserID: u.ID, DateFrom: ledgerDate("2026-07-10"), DateTo: ledgerDate("2026-07-10"), DailyAmount: domain.Rub(100000), CreatedAt: ledgerNow}
	if err = s.CreateOverride(ctx, o); err != nil {
		t.Fatal(err)
	}
	assertEstimate := func(want int64) {
		t.Helper()
		snap, err := s.FinancialSnapshot(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		input := forecast.Input{Settings: snap.Settings, Anchors: snap.Anchors, Occurrences: snap.Occurrences, LegacyFacts: snap.LegacyFacts, Overrides: snap.Overrides, DayCosts: snap.DayCosts, AsOf: ledgerNow}
		got, _, err := forecast.EstimatedBalance(ctx, input)
		if err != nil || got.Amount != want {
			t.Fatalf("snapshot estimate: %+v %v, want %d", got, err, want)
		}
		raw, _, err := forecast.ActualBalance(ctx, input)
		if err != nil || raw.Amount != 2000000 {
			t.Fatalf("raw ledger changed: %+v %v", raw, err)
		}
	}
	queries := []string{`SELECT * FROM actual_transactions`, `SELECT * FROM occurrence_states`, `SELECT * FROM ledger_clock`, `SELECT * FROM anchors`, `SELECT * FROM base_day_cost_history`}
	before := make([][]string, len(queries))
	for i, q := range queries {
		before[i] = snapshot(t, s.db, q)
	}
	for i := 0; i < 3; i++ {
		assertEstimate(1900000)
	}
	for i, q := range queries {
		if got := snapshot(t, s.db, q); !reflect.DeepEqual(got, before[i]) {
			t.Fatalf("read wrote synthetic financial data: %s before=%v after=%v", q, before[i], got)
		}
	}
	o.DailyAmount = domain.Rub(0)
	if err = s.UpdateOverride(ctx, o); err != nil {
		t.Fatal(err)
	}
	assertEstimate(2000000)
	if err = s.DeleteOverride(ctx, u.ID, o.ID); err != nil {
		t.Fatal(err)
	}
	assertEstimate(1860000)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	assertEstimate(1860000)
}
