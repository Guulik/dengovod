package forecast

import (
	"context"
	"errors"
	"testing"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/google/uuid"
)

func completedDayInput() Input {
	input := baseInput("2026-11-16T12:00:00Z", "2026-11-16T06:00:00Z", 2000000, 140000, domain.HorizonCustomDate, "2026-11-18")
	input.Anchor.Sequence = 2
	input.RangeFrom, input.RangeTo = date("2026-11-16"), date("2026-11-18")
	return input
}

func TestCompletedDaysAccrueAtMoscowMidnightWithoutChangingRawLedger(t *testing.T) {
	for _, test := range []struct {
		now  string
		want int64
	}{
		{"2026-11-16T20:59:59Z", 2000000}, {"2026-11-16T21:00:00Z", 1860000},
		{"2026-11-17T21:00:00Z", 1720000}, {"2026-11-22T21:00:00Z", 1020000},
	} {
		t.Run(test.now, func(t *testing.T) {
			input := completedDayInput()
			input.AsOf = at(test.now)
			for repeat := 0; repeat < 2; repeat++ {
				got, _, err := EstimatedBalance(context.Background(), input)
				if err != nil {
					t.Fatal(err)
				}
				if got.Amount != test.want {
					t.Fatalf("estimated=%d want=%d", got.Amount, test.want)
				}
			}
			raw, _, err := ActualBalance(context.Background(), input)
			if err != nil || raw.Amount != 2000000 {
				t.Fatalf("raw ledger changed: %+v %v", raw, err)
			}
		})
	}
}

func TestCompletedDaysCalendarAndMFLReserveEachDayOnce(t *testing.T) {
	input := completedDayInput()
	input.AsOf = at("2026-11-17T09:00:00Z")
	output, err := CalculateContext(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if output.Summary.CurrentBalance.Amount != 1860000 || output.Summary.MoneyForLife.Amount != 1580000 || output.Summary.DayCostRemaining.Amount != 280000 {
		t.Fatalf("completed reserve duplicated in summary: %+v", output.Summary)
	}
	for i, want := range []int64{1860000, 1720000, 1580000} {
		if output.Days[i].ExpectedBalance.Amount != want || output.Days[i].MoneyForLife.Amount != 1580000 {
			t.Fatalf("day %d: %+v", i, output.Days[i])
		}
	}
	short := input
	short.RangeTo = short.RangeFrom
	sparse, err := CalculateContext(context.Background(), short)
	if err != nil {
		t.Fatal(err)
	}
	if *sparse.Summary != *output.Summary || *sparse.Days[0].MoneyForLife != *output.Days[0].MoneyForLife {
		t.Fatalf("visible range changed calculations: full=%+v short=%+v", output, sparse)
	}
}

func TestCompletedDaysMarkedFactsAroundIntradayAnchor(t *testing.T) {
	for _, test := range []struct {
		name          string
		before, after int64
		marked        bool
		want          int64
	}{
		{"before-anchor-included", 20000, 0, true, 1880000},
		{"after-anchor", 0, 20000, true, 1860000},
		{"both-sides", 20000, 20000, true, 1880000},
		{"above-budget", 0, 200000, true, 1800000},
		{"unmarked-extra", 0, 200000, false, 1660000},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := completedDayInput()
			input.AsOf = at("2026-11-17T09:00:00Z")
			for i, amount := range []int64{test.before, test.after} {
				if amount == 0 {
					continue
				}
				executed := at([]string{"2026-11-16T05:00:00Z", "2026-11-16T07:00:00Z"}[i])
				input.Occurrences = append(input.Occurrences, domain.EventOccurrence{EventID: uuid.New(), Date: date("2026-11-16"), Status: domain.OccurrenceConfirmed, EffectiveAt: &executed, Sequence: int64(1 + 2*i), Kind: domain.EventKindExpense, Necessity: domain.NecessityRequired, Amount: domain.Rub(amount), CountsTowardDayBudget: test.marked})
			}
			got, _, err := EstimatedBalance(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if got.Amount != test.want {
				t.Fatalf("estimated=%d want=%d", got.Amount, test.want)
			}
			output, err := CalculateContext(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if output.Summary.CurrentBalance.Amount != got.Amount || output.Days[0].ExpectedBalance.Amount != got.Amount {
				t.Fatalf("calendar/summary disagree with estimate: %+v", output)
			}
		})
	}
}

func TestCompletedDaysUseEffectiveDateAndLatestAnchorSegment(t *testing.T) {
	input := completedDayInput()
	input.AsOf = at("2026-11-18T09:00:00Z")
	input.Anchors = []domain.BalanceAnchor{input.Anchor, {ID: uuid.New(), Amount: domain.Rub(1500000), AnchoredAt: at("2026-11-17T09:00:00Z"), Sequence: 5}}
	input.DayCosts = []domain.DayCostVersion{{ID: 1, EffectiveFrom: date("2026-11-16"), Amount: domain.Rub(140000)}, {ID: 2, EffectiveFrom: date("2026-11-17"), Amount: domain.Rub(200000)}}
	executed := at("2026-11-16T08:00:00Z")
	input.Occurrences = []domain.EventOccurrence{{EventID: uuid.New(), Date: date("2026-11-17"), Status: domain.OccurrenceConfirmed, EffectiveAt: &executed, Sequence: 3, Kind: domain.EventKindExpense, Necessity: domain.NecessityRequired, Amount: domain.Rub(20000), CountsTowardDayBudget: true}}
	input.Overrides = []domain.DayCostOverride{{ID: uuid.New(), DateFrom: date("2026-11-16"), DateTo: date("2026-11-16"), DailyAmount: domain.Rub(100000)}}
	got, _, err := EstimatedBalance(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount != 1300000 {
		t.Fatalf("old segment affected new anchor: %d", got.Amount)
	}
	output, err := CalculateContext(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if output.Days[0].ExpectedBalance.Amount != 1900000 || output.Days[1].ExpectedBalance.Amount != 1300000 || output.Summary.CurrentBalance.Amount != 1300000 {
		t.Fatalf("wrong financial date or day-cost history: %+v", output)
	}
	input.Overrides[0].DailyAmount = domain.Rub(0)
	got, _, err = EstimatedBalance(context.Background(), input)
	if err != nil || got.Amount != 1300000 {
		t.Fatalf("editing old day changed later reconciliation: %+v %v", got, err)
	}
}

func TestCompletedDaysCrossMonthYearAndLeapDay(t *testing.T) {
	for _, test := range []struct {
		anchor, now string
		days        int64
	}{
		{"2026-12-31T09:00:00Z", "2026-12-31T21:00:00Z", 1},
		{"2028-02-28T09:00:00Z", "2028-02-29T21:00:00Z", 2},
		{"2027-02-28T09:00:00Z", "2027-02-28T21:00:00Z", 1},
		{"2026-11-16T21:00:00Z", "2026-11-17T21:00:00Z", 1},
	} {
		input := Input{Anchor: domain.BalanceAnchor{AnchoredAt: at(test.anchor), Amount: domain.Rub(10000)}, Settings: domain.UserSettings{BaseDayCost: domain.Rub(100)}, AsOf: at(test.now)}
		got, _, err := EstimatedBalance(context.Background(), input)
		if err != nil || got.Amount != 10000-test.days*100 {
			t.Fatalf("boundary %+v: %+v %v", test, got, err)
		}
	}
}

func TestCompletedDaysEstimatePreservesUnknownAndCheckedArithmetic(t *testing.T) {
	input := completedDayInput()
	input.Anchor = domain.BalanceAnchor{}
	if _, _, err := EstimatedBalance(context.Background(), input); !errors.Is(err, domain.ErrAnchorRequired) {
		t.Fatalf("invented unknown balance: %v", err)
	}
	input = completedDayInput()
	input.AsOf = at("2026-11-18T09:00:00Z")
	input.Settings.BaseDayCost = domain.Rub(domain.MaxMoney)
	if _, _, err := EstimatedBalance(context.Background(), input); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("reserve overflow accepted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := EstimatedBalance(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel ignored: %v", err)
	}
}

func TestCompletedDaysOverridePriorityPrecedesClockAndUUID(t *testing.T) {
	input := completedDayInput()
	input.AsOf = at("2026-11-17T09:00:00Z")
	older := domain.DayCostOverride{ID: uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff"), DateFrom: date("2026-11-16"), DateTo: date("2026-11-18"), DailyAmount: domain.Rub(140000), CreatedAt: at("2026-11-16T09:00:00Z"), Priority: 1}
	newer := domain.DayCostOverride{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"), DateFrom: date("2026-11-16"), DateTo: date("2026-11-16"), DailyAmount: domain.Rub(80000), CreatedAt: at("2026-11-16T08:00:00Z"), Priority: 2}
	for _, overrides := range [][]domain.DayCostOverride{{older, newer}, {newer, older}} {
		input.Overrides = overrides
		got, _, err := EstimatedBalance(context.Background(), input)
		if err != nil || got.Amount != 1920000 {
			t.Fatalf("creation order lost: %+v %v", got, err)
		}
	}
	older.Priority = 0
	newer.Priority = 0
	if got := DayCost(date("2026-11-16"), input.Settings, []domain.DayCostOverride{newer, older}); got.Amount != 140000 {
		t.Fatalf("in-memory fallback changed: %+v", got)
	}
}

func TestCompletedDaysRepresentableResetKeepsShortAndFullRangesEqual(t *testing.T) {
	input := baseInput("2026-07-11T12:00:00Z", "2026-07-01T08:00:00Z", 10000, 0, domain.HorizonCustomDate, "2026-07-11")
	input.RangeFrom, input.RangeTo = date("2026-07-01"), date("2026-07-11")
	input.Anchors = []domain.BalanceAnchor{input.Anchor, {ID: uuid.New(), Amount: domain.Rub(20000), AnchoredAt: at("2026-07-11T08:00:00Z"), Sequence: 1}}
	input.Overrides = []domain.DayCostOverride{{ID: uuid.New(), DateFrom: date("2026-07-02"), DateTo: date("2026-07-10"), DailyAmount: domain.Rub(100)}}
	full, err := CalculateContext(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.RangeTo = input.RangeFrom
	short, err := CalculateContext(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if full.Days[9].ExpectedBalance.Amount != 9100 || short.Days[0].MoneyForLife.Amount != 20000 {
		t.Fatalf("wrong completed costs or reset: full=%+v short=%+v", full, short)
	}
	if *short.Days[0].ExpectedBalance != *full.Days[0].ExpectedBalance || *short.Days[0].MoneyForLife != *full.Days[0].MoneyForLife || *short.Summary != *full.Summary {
		t.Fatalf("representable ranges disagree: full=%+v short=%+v", full, short)
	}
}
