package forecast

import (
	"testing"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/google/uuid"
)

func date(value string) domain.CalendarDate {
	d, err := domain.ParseDate(value)
	if err != nil {
		panic(err)
	}
	return d
}
func at(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}
func event(kind domain.EventKind, necessity domain.EventNecessity, amount int64, day string) domain.FinancialEvent {
	return domain.FinancialEvent{ID: uuid.New(), Kind: kind, Necessity: necessity, Amount: domain.Rub(amount),
		Recurrence: domain.RecurrenceRule{Type: domain.RecurrenceNone, StartDate: date(day)}}
}
func baseInput(asOf, anchor string, amount, cost int64, policy domain.HorizonPolicy, custom string) Input {
	var customDate *domain.CalendarDate
	if custom != "" {
		value := date(custom)
		customDate = &value
	}
	return Input{
		Anchor:    domain.BalanceAnchor{Amount: domain.Rub(amount), AnchoredAt: at(anchor)},
		Settings:  domain.UserSettings{BaseDayCost: domain.Rub(cost), HorizonPolicy: policy, HorizonCustomDate: customDate},
		RangeFrom: date(asOf[:10]), RangeTo: date(asOf[:10]), AsOf: at(asOf),
	}
}

func TestCalculateBoundariesAndNecessity(t *testing.T) {
	tests := []struct {
		name         string
		input        Input
		wantBalance  int64
		wantMFL      int64
		wantDayCost  int64
		wantRequired int64
		wantOptional int64
		wantDays     int
	}{
		{
			name:        "anchor day is unchanged but current day cost is reserved",
			input:       baseInput("2026-07-10T09:00:00Z", "2026-07-10T08:00:00Z", 10_000, 1_000, domain.HorizonCustomDate, "2026-07-10"),
			wantBalance: 10_000, wantMFL: 9_000, wantDayCost: 1_000, wantDays: 1,
		},
		{
			name: "next income date and income are excluded",
			input: func() Input {
				value := baseInput("2026-07-10T09:00:00Z", "2026-07-10T08:00:00Z", 10_000, 1_000, domain.HorizonNextIncome, "")
				value.Events = []domain.FinancialEvent{event(domain.EventKindIncome, domain.NecessityRequired, 50_000, "2026-07-12")}
				return value
			}(),
			wantBalance: 10_000, wantMFL: 8_000, wantDayCost: 2_000, wantDays: 2,
		},
		{
			name:        "end of month includes last day",
			input:       baseInput("2026-07-30T09:00:00Z", "2026-07-30T08:00:00Z", 10_000, 1_000, domain.HorizonEndOfMonth, ""),
			wantBalance: 10_000, wantMFL: 8_000, wantDayCost: 2_000, wantDays: 2,
		},
		{
			name: "required affects MFL optional is informational",
			input: func() Input {
				value := baseInput("2026-07-10T09:00:00Z", "2026-07-10T08:00:00Z", 10_000, 0, domain.HorizonCustomDate, "2026-07-10")
				value.Events = []domain.FinancialEvent{
					event(domain.EventKindExpense, domain.NecessityRequired, 2_000, "2026-07-10"),
					event(domain.EventKindExpense, domain.NecessityOptional, 3_000, "2026-07-10"),
				}
				return value
			}(),
			wantBalance: 10_000, wantMFL: 8_000, wantRequired: 2_000, wantOptional: 3_000, wantDays: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Calculate(test.input).Summary
			if got.CurrentBalance.Amount != test.wantBalance || got.MoneyForLife.Amount != test.wantMFL ||
				got.DayCostRemaining.Amount != test.wantDayCost || got.RequiredRemaining.Amount != test.wantRequired ||
				got.OptionalRemaining.Amount != test.wantOptional || got.DaysUntilHorizon != test.wantDays {
				t.Fatalf("unexpected summary: %+v", got)
			}
		})
	}
}

func TestAnchorAdjustmentsBeginNextCalendarDay(t *testing.T) {
	input := baseInput("2026-07-11T09:00:00Z", "2026-07-10T08:00:00Z", 1_000, 100, domain.HorizonCustomDate, "2026-07-11")
	input.RangeFrom, input.RangeTo = date("2026-07-10"), date("2026-07-11")
	output := Calculate(input)
	if len(output.Days) != 2 || output.Days[0].ExpectedBalance.Amount != 1_000 || output.Days[1].ExpectedBalance.Amount != 900 {
		t.Fatalf("unexpected projections: %+v", output.Days)
	}
}

func TestDayCostLatestOverrideWins(t *testing.T) {
	settings := domain.UserSettings{BaseDayCost: domain.Rub(100)}
	overrides := []domain.DayCostOverride{
		{DateFrom: date("2026-07-01"), DateTo: date("2026-07-20"), DailyAmount: domain.Rub(50), CreatedAt: at("2026-07-01T00:00:00Z")},
		{DateFrom: date("2026-07-10"), DateTo: date("2026-07-12"), DailyAmount: domain.Rub(25), CreatedAt: at("2026-07-02T00:00:00Z")},
	}
	if got := DayCost(date("2026-07-11"), settings, overrides).Amount; got != 25 {
		t.Fatalf("got %d", got)
	}
}

func TestExpandEventsRecurrence(t *testing.T) {
	day31, every3 := 31, 3
	tests := []struct {
		name     string
		event    domain.FinancialEvent
		from, to string
		want     []string
	}{
		{
			name: "monthly day clamps to month end",
			event: domain.FinancialEvent{ID: uuid.New(), Kind: domain.EventKindExpense, Necessity: domain.NecessityRequired, Amount: domain.Rub(1),
				Recurrence: domain.RecurrenceRule{Type: domain.RecurrenceMonthlyOnDay, StartDate: date("2026-01-31"), DayOfMonth: &day31}},
			from: "2026-01-01", to: "2026-03-31", want: []string{"2026-01-31", "2026-02-28", "2026-03-31"},
		},
		{
			name: "every N respects start until and range",
			event: domain.FinancialEvent{ID: uuid.New(), Kind: domain.EventKindExpense, Necessity: domain.NecessityRequired, Amount: domain.Rub(1),
				Recurrence: domain.RecurrenceRule{Type: domain.RecurrenceEveryNDays, StartDate: date("2026-07-01"), IntervalDays: &every3,
					Until: func() *domain.CalendarDate { value := date("2026-07-08"); return &value }()}},
			from: "2026-07-02", to: "2026-07-10", want: []string{"2026-07-04", "2026-07-07"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotMap := ExpandEvents([]domain.FinancialEvent{test.event}, date(test.from), date(test.to))
			got := []string{}
			for d := date(test.from); !d.After(date(test.to)); d = d.AddDays(1) {
				if len(gotMap[d.String()]) > 0 {
					got = append(got, d.String())
				}
			}
			if len(got) != len(test.want) {
				t.Fatalf("got %v want %v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("got %v want %v", got, test.want)
				}
			}
		})
	}
}
