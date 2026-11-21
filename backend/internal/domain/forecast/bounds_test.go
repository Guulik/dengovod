package forecast

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/google/uuid"
)

func TestAncientUnknownCalendarExpandsOnlyVisibleAndForecastWindows(t *testing.T) {
	input := baseInput("2026-07-11T12:00:00Z", "2026-07-11T08:00:00Z", 10000, 0, domain.HorizonEndOfMonth, "")
	input.RangeFrom = date("0001-01-01")
	input.RangeTo = input.RangeFrom
	interval := 1
	e := event(domain.EventKindExpense, domain.NecessityRequired, 1, "0001-01-01")
	e.Recurrence.Type = domain.RecurrenceEveryNDays
	e.Recurrence.IntervalDays = &interval
	input.Events = []domain.FinancialEvent{e}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	output, err := CalculateContext(ctx, input)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8<<20 {
		t.Fatalf("one ancient day allocated %d bytes; invisible centuries must not be materialized", allocated)
	}
	if len(output.Days) != 1 || output.Days[0].ExpectedBalance != nil || output.Days[0].MoneyForLife != nil || len(output.Days[0].Events) != 1 {
		t.Fatalf("ancient operations/null lost: %+v", output)
	}
	if output.Summary == nil || output.Summary.CurrentBalance.Amount != 10000 || output.Summary.RequiredRemaining.Amount != 21 {
		t.Fatalf("summary changed with visible ancient window: %+v", output.Summary)
	}
}

func TestAncientKnownHistoryUsesSparseReserveIntegration(t *testing.T) {
	input := baseInput("2026-07-11T12:00:00Z", "0001-01-01T08:00:00Z", 1000000, 1, domain.HorizonCustomDate, "2026-07-12")
	input.RangeFrom = date("0001-01-01")
	input.RangeTo = input.RangeFrom
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	output, err := CalculateContext(ctx, input)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8<<20 {
		t.Fatalf("sparse known history allocated %d bytes", allocated)
	}
	want := int64(1000000) - domain.DaysBetween(input.RangeFrom, date("2026-07-12")) - 1
	if len(output.Days) != 1 || output.Days[0].ExpectedBalance.Amount != 999999 || output.Days[0].MoneyForLife.Amount != want {
		t.Fatalf("sparse historical MFL: %+v, want %d", output.Days, want)
	}
	if output.Summary.MoneyForLife.Amount != want {
		t.Fatalf("current summary reserved invisible history: %+v", output.Summary)
	}
}

func TestSparseHistoricalGapMatchesContiguousCalculation(t *testing.T) {
	input := baseInput("2026-07-11T12:00:00Z", "2026-07-01T08:00:00Z", 10000, 100, domain.HorizonCustomDate, "2026-07-12")
	input.RangeFrom = date("2026-07-01")
	input.RangeTo = date("2026-07-12")
	input.DayCosts = []domain.DayCostVersion{{ID: 1, EffectiveFrom: date("0001-01-01"), Amount: domain.Rub(100)}, {ID: 2, EffectiveFrom: date("2026-07-05"), Amount: domain.Rub(200)}}
	input.Overrides = []domain.DayCostOverride{{ID: uuid.New(), DateFrom: date("2026-07-06"), DateTo: date("2026-07-08"), DailyAmount: domain.Rub(50)}}
	for i, amount := range []int64{500, 30, 300} {
		executed := at([]string{"2026-07-03T09:00:00Z", "2026-07-07T09:00:00Z", "2026-07-10T09:00:00Z"}[i])
		input.Occurrences = append(input.Occurrences, domain.EventOccurrence{EventID: uuid.New(), Date: domain.DateFromTime(executed), Status: domain.OccurrenceConfirmed, EffectiveAt: &executed, Sequence: int64(i + 1), Kind: domain.EventKindExpense, Necessity: domain.NecessityRequired, Amount: domain.Rub(amount), CountsTowardDayBudget: i == 1})
	}
	for _, reset := range []bool{false, true} {
		t.Run(map[bool]string{false: "additive", true: "anchor-reset"}[reset], func(t *testing.T) {
			if reset {
				input.Anchors = []domain.BalanceAnchor{input.Anchor, {ID: uuid.New(), Amount: domain.Rub(15000), AnchoredAt: at("2026-07-09T08:00:00Z"), Sequence: 5}}
			}
			full, err := CalculateContext(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			sparse := input
			sparse.RangeTo = sparse.RangeFrom
			got, err := CalculateContext(context.Background(), sparse)
			if err != nil {
				t.Fatal(err)
			}
			if got.Days[0].ExpectedBalance.Amount != full.Days[0].ExpectedBalance.Amount || got.Days[0].MoneyForLife.Amount != full.Days[0].MoneyForLife.Amount {
				t.Fatalf("gap changed history: full=%+v sparse=%+v", full.Days[0], got.Days[0])
			}
			if *got.Summary != *full.Summary {
				t.Fatalf("gap changed summary: full=%+v sparse=%+v", full.Summary, got.Summary)
			}
		})
	}
}

func TestSparseGapBeforeSuffixResetDoesNotOverflowDiscardedReserve(t *testing.T) {
	input := baseInput("2026-07-11T12:00:00Z", "2026-07-01T08:00:00Z", 10000, 0, domain.HorizonCustomDate, "2026-07-11")
	input.RangeFrom, input.RangeTo = date("2026-07-01"), date("2026-07-11")
	input.Anchors = []domain.BalanceAnchor{input.Anchor, {
		ID: uuid.New(), Amount: domain.Rub(20000), AnchoredAt: at("2026-07-11T08:00:00Z"), Sequence: 1,
	}}
	input.Overrides = []domain.DayCostOverride{{
		ID: uuid.New(), DateFrom: date("2026-07-02"), DateTo: date("2026-07-10"), DailyAmount: domain.Rub(domain.MaxMoney),
	}}
	// DV-8 makes completed costs part of the displayed historical balances.
	// Requesting these unrepresentable days must fail; a short window before
	// them still has a representable MFL reset by the later reconciliation.
	if _, err := CalculateContext(context.Background(), input); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("overflowing displayed history accepted: %v", err)
	}
	input.RangeTo = input.RangeFrom
	short, err := CalculateContext(context.Background(), input)
	if err != nil {
		t.Fatalf("discarded reserve before later anchor must not overflow: %v", err)
	}
	if len(short.Days) != 1 || short.Days[0].ExpectedBalance.Amount != 10000 || short.Days[0].MoneyForLife.Amount != 20000 {
		t.Fatalf("unexpected short historical projection: %+v", short.Days)
	}
	if short.Summary.CurrentBalance.Amount != 20000 || short.Summary.MoneyForLife.Amount != 20000 {
		t.Fatalf("old discarded estimates changed current segment: %+v", short.Summary)
	}
}

func TestNextIncomeAndCustomForecastStayWithin3660Days(t *testing.T) {
	input := baseInput("2026-07-11T12:00:00Z", "2026-07-11T08:00:00Z", 10000, 0, domain.HorizonNextIncome, "")
	input.Events = []domain.FinancialEvent{event(domain.EventKindIncome, domain.NecessityRequired, 100, "9999-12-31")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := CalculateContext(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if output.Summary.HorizonDate.String() != "2026-07-31" || output.Summary.DaysUntilHorizon != 21 {
		t.Fatalf("distant income did not fall back to month end: %+v", output.Summary)
	}
	limit := date("2026-07-11").AddDays(MaxForecastDays)
	input.Events[0].Recurrence.StartDate = limit
	output, err = CalculateContext(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if output.Summary.HorizonDate != limit || output.Summary.DaysUntilHorizon != MaxForecastDays {
		t.Fatalf("3660-day boundary excluded: %+v", output.Summary)
	}
	input.RangeFrom = limit.AddDays(1)
	input.RangeTo = input.RangeFrom
	if _, err = CalculateContext(ctx, input); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("future calendar outside bound accepted: %v", err)
	}
}
