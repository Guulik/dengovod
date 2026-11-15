package sqlite

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/alshatilov/dengovod/backend/internal/domain/forecast"
	"github.com/google/uuid"
)

var ledgerNow = time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

func ledgerDate(value string) domain.CalendarDate {
	d, err := domain.ParseDate(value)
	if err != nil {
		panic(err)
	}
	return d
}
func ledgerStore(t *testing.T) (*Store, domain.User) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	u, err := s.ProvisionUser(context.Background(), "own-ledger")
	if err != nil {
		t.Fatal(err)
	}
	return s, u
}
func ledgerEvent(t *testing.T, s *Store, u domain.User, kind domain.EventKind, amount int64) domain.FinancialEvent {
	t.Helper()
	e := domain.FinancialEvent{ID: uuid.New(), UserID: u.ID, Kind: kind, Necessity: domain.NecessityRequired, Amount: domain.Rub(amount), Recurrence: domain.RecurrenceRule{Type: domain.RecurrenceNone, StartDate: ledgerDate("2026-07-11")}, CreatedAt: ledgerNow, UpdatedAt: ledgerNow}
	if err := s.CreateEvent(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	return e
}
func ledgerAnchor(t *testing.T, s *Store, u domain.User, amount int64, at time.Time) {
	t.Helper()
	if err := s.CreateAnchor(context.Background(), domain.BalanceAnchor{ID: uuid.New(), UserID: u.ID, Amount: domain.Rub(amount), AnchoredAt: at, CreatedAt: ledgerNow}); err != nil {
		t.Fatal(err)
	}
}
func ledgerBalance(t *testing.T, s *Store, u domain.User) int64 {
	t.Helper()
	snapshot, err := s.FinancialSnapshot(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	amount, _, err := forecast.ActualBalance(context.Background(), forecast.Input{Anchors: snapshot.Anchors, Occurrences: snapshot.Occurrences, LegacyFacts: snapshot.LegacyFacts, AsOf: ledgerNow})
	if err != nil {
		t.Fatal(err)
	}
	return amount.Amount
}
func TestConcurrentConfirmationAcrossConnectionsHasOneSequenceAndSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	user, err := first.ProvisionUser(context.Background(), "concurrent-own")
	if err != nil {
		t.Fatal(err)
	}
	ledgerAnchor(t, first, user, 1000, ledgerNow)
	event := ledgerEvent(t, first, user, domain.EventKindIncome, 200)
	var wg sync.WaitGroup
	results := make(chan domain.OccurrenceResult, 24)
	failures := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := first
			if i%2 == 1 {
				s = second
			}
			r, err := s.ChangeOccurrence(context.Background(), user.ID, event.ID, event.Recurrence.StartDate, domain.OccurrenceChange{Status: domain.OccurrenceConfirmed, Now: ledgerNow})
			if err != nil {
				failures <- err
			} else {
				results <- r
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var sequence int64
	for r := range results {
		if r.Status != domain.OccurrenceConfirmed || r.Amount.Amount != 200 {
			t.Fatalf("bad snapshot: %+v", r)
		}
		if sequence == 0 {
			sequence = r.Sequence
		}
		if r.Sequence != sequence {
			t.Fatalf("multiple fact sequences: %d and %d", sequence, r.Sequence)
		}
	}
	if got := ledgerBalance(t, first, user); got != 1200 {
		t.Fatalf("actual balance=%d", got)
	}
	if got := snapshot(t, first.db, `SELECT count(*) FROM occurrence_states`); len(got) != 1 || !strings.Contains(got[0], "{1}") {
		t.Fatalf("duplicate states: %v", got)
	}
}

func TestUndoOverflowRollsBackFactAndSequence(t *testing.T) {
	s, user := ledgerStore(t)
	ledgerAnchor(t, s, user, domain.MaxMoney-50, ledgerNow.Add(-time.Hour))
	expense := ledgerEvent(t, s, user, domain.EventKindExpense, 100)
	income := ledgerEvent(t, s, user, domain.EventKindIncome, 100)
	for i, e := range []domain.FinancialEvent{expense, income} {
		at := ledgerNow.Add(time.Duration(i-2) * time.Second)
		if _, err := s.ChangeOccurrence(context.Background(), user.ID, e.ID, e.Recurrence.StartDate, domain.OccurrenceChange{Status: domain.OccurrenceConfirmed, Now: ledgerNow, EffectiveAt: &at}); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshot(t, s.db, `SELECT * FROM occurrence_states ORDER BY event_id`)
	if _, err := s.ChangeOccurrence(context.Background(), user.ID, expense.ID, expense.Recurrence.StartDate, domain.OccurrenceChange{Status: domain.OccurrencePlanned, Now: ledgerNow}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("undo should overflow: %v", err)
	}
	after := snapshot(t, s.db, `SELECT * FROM occurrence_states ORDER BY event_id`)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("failed undo changed facts: %v -> %v", before, after)
	}
	if got := ledgerBalance(t, s, user); got != domain.MaxMoney-50 {
		t.Fatalf("balance corrupted: %d", got)
	}
}

func TestLedgerOrderPreservesNanosecondsBeforeSequence(t *testing.T) {
	s, user := ledgerStore(t)
	base := ledgerNow.Add(-time.Second)
	ledgerAnchor(t, s, user, domain.MaxMoney-50, base.Add(100*time.Nanosecond))
	expense := ledgerEvent(t, s, user, domain.EventKindExpense, 100)
	expenseAt := base.Add(300 * time.Nanosecond)
	if _, err := s.ChangeOccurrence(context.Background(), user.ID, expense.ID, expense.Recurrence.StartDate, domain.OccurrenceChange{Status: domain.OccurrenceConfirmed, Now: ledgerNow, EffectiveAt: &expenseAt}); err != nil {
		t.Fatal(err)
	}
	income := ledgerEvent(t, s, user, domain.EventKindIncome, 100)
	incomeAt := base.Add(200 * time.Nanosecond)
	if _, err := s.ChangeOccurrence(context.Background(), user.ID, income.ID, income.Recurrence.StartDate, domain.OccurrenceChange{Status: domain.OccurrenceConfirmed, Now: ledgerNow, EffectiveAt: &incomeAt}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("income must overflow before the later expense regardless of sequence: %v", err)
	}
	if got := ledgerBalance(t, s, user); got != domain.MaxMoney-150 {
		t.Fatalf("failed write leaked: %d", got)
	}
}

func TestLegacyStandaloneFactsUseFinancialDateAndAreAbsorbedAtAnchor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-facts.db")
	db := migrationDB(t, path)
	initial, err := fs.ReadFile(migrations, "migrations/001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, string(initial))
	execSQL(t, db, `INSERT INTO users VALUES('11111111-1111-1111-1111-111111111111','standalone','2026-07-01T00:00:00Z');
 INSERT INTO settings VALUES('11111111-1111-1111-1111-111111111111',123,'END_OF_MONTH',NULL,'RUB','2026-07-11T00:00:00Z');
 INSERT INTO anchors VALUES('22222222-2222-2222-2222-222222222222','11111111-1111-1111-1111-111111111111',1000,'2026-07-10T09:00:00Z','2026-07-10T09:00:00Z');
 INSERT INTO actual_transactions VALUES('33333333-3333-3333-3333-333333333333','11111111-1111-1111-1111-111111111111','INCOME',200,NULL,'2026-07-10','included',NULL,'2026-07-11T00:00:00Z');
 INSERT INTO actual_transactions VALUES('44444444-4444-4444-4444-444444444444','11111111-1111-1111-1111-111111111111','EXPENSE',300,NULL,'2026-07-11','after anchor',NULL,'2026-07-01T00:00:00Z');`)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u, err := s.ProvisionUser(context.Background(), "standalone")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.FinancialSnapshot(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.NeedsBalanceReview || len(snap.LegacyFacts) != 2 || len(snap.Occurrences) != 0 || len(snap.Events) != 0 {
		t.Fatalf("legacy standalone facts lost or guessed an occurrence: %+v", snap)
	}
	if got := ledgerBalance(t, s, u); got != 700 {
		t.Fatalf("legacy facts balance=%d, want 700", got)
	}
	if !snap.LegacyFacts[0].EffectiveAt.Equal(time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("created_at replaced financial date: %+v", snap.LegacyFacts[0])
	}
	if snap.LegacyFacts[0].Sequence >= snap.Anchors[0].Sequence {
		t.Fatal("legacy equal-time fact must precede anchor")
	}
	if len(snap.DayCosts) != 1 || snap.DayCosts[0].EffectiveFrom.String() != "0001-01-01" || snap.DayCosts[0].Amount.Amount != 123 {
		t.Fatalf("legacy day cost baseline: %+v", snap.DayCosts)
	}
	ledgerAnchor(t, s, u, 700, ledgerNow)
	snap, err = s.FinancialSnapshot(context.Background(), u.ID)
	if err != nil || snap.NeedsBalanceReview {
		t.Fatalf("reconciliation did not clear review flag: %+v %v", snap, err)
	}
	if got := snapshot(t, s.db, `SELECT count(*) FROM actual_transactions`); len(got) != 1 || !strings.Contains(got[0], "{2}") {
		t.Fatalf("legacy source rows changed: %v", got)
	}
}

func TestUnsafeLegacyMoneyFailsMigrationWithoutTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unsafe.db")
	db := migrationDB(t, path)
	initial, err := fs.ReadFile(migrations, "migrations/001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, string(initial))
	execSQL(t, db, `INSERT INTO users VALUES('11111111-1111-1111-1111-111111111111','unsafe','2026-07-11T00:00:00Z');INSERT INTO anchors VALUES('22222222-2222-2222-2222-222222222222','11111111-1111-1111-1111-111111111111',9007199254740992,'2026-07-11T00:00:00Z','2026-07-11T00:00:00Z');`)
	s, err := Open(path)
	if err == nil {
		s.Close()
		t.Fatal("unsafe legacy money accepted")
	}
	if !strings.Contains(err.Error(), "legacy_money_out_of_range") {
		t.Fatal(err)
	}
	if got := snapshot(t, db, `SELECT amount FROM anchors`); len(got) != 1 || !strings.Contains(got[0], "9007199254740992") {
		t.Fatalf("legacy amount truncated: %v", got)
	}
	if got := snapshot(t, db, `SELECT name FROM sqlite_schema WHERE name='legacy_facts'`); len(got) != 0 {
		t.Fatalf("failed migration left DDL: %v", got)
	}
}
