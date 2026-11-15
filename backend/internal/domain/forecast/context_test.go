package forecast

import (
	"context"
	"errors"
	"testing"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/google/uuid"
)

func TestCalculationHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	input := baseInput("2026-07-11T12:00:00Z", "2026-07-11T08:00:00Z", 1000, 100, domain.HorizonEndOfMonth, "")
	if _, err := CalculateContext(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel ignored: %v", err)
	}
}
func TestUnknownCalendarDoesNotAccumulateAnUnknownForecast(t *testing.T) {
	input := Input{Settings: domain.UserSettings{HorizonPolicy: domain.HorizonEndOfMonth}, AsOf: at("2026-07-11T12:00:00Z"), RangeFrom: date("2026-07-11"), RangeTo: date("2026-07-12")}
	input.Events = []domain.FinancialEvent{event(domain.EventKindIncome, domain.NecessityRequired, domain.MaxMoney, "2026-07-11"), event(domain.EventKindIncome, domain.NecessityRequired, domain.MaxMoney, "2026-07-12")}
	output, err := CalculateContext(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if output.Summary != nil || len(output.Days) != 2 {
		t.Fatalf("unknown output: %+v", output)
	}
	for _, day := range output.Days {
		if day.ExpectedBalance != nil || day.MoneyForLife != nil || day.IncomeTotal.Amount != domain.MaxMoney {
			t.Fatalf("unknown amounts or day totals: %+v", day)
		}
	}
}
func TestHistoricalMoneyForLifeUsesSubsequentFactsAndControlPoints(t *testing.T) {
	input := baseInput("2026-07-11T12:00:00Z", "2026-07-09T08:00:00Z", 10000, 0, domain.HorizonCustomDate, "2026-07-12")
	input.RangeFrom = date("2026-07-09")
	input.RangeTo = date("2026-07-12")
	executed := at("2026-07-10T09:00:00Z")
	input.Occurrences = []domain.EventOccurrence{{EventID: uuid.New(), Date: date("2026-07-10"), Status: domain.OccurrenceConfirmed, EffectiveAt: &executed, Sequence: 2, Kind: domain.EventKindIncome, Necessity: domain.NecessityRequired, Amount: domain.Rub(8700)}}
	output, err := CalculateContext(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if output.Days[0].ExpectedBalance.Amount != 10000 || output.Days[0].MoneyForLife.Amount != 18700 {
		t.Fatalf("later fact lost in MFL: %+v", output.Days[0])
	}
	input.Anchors = []domain.BalanceAnchor{input.Anchor, {ID: uuid.New(), Amount: domain.Rub(20000), AnchoredAt: at("2026-07-11T08:00:00Z"), Sequence: 3}}
	output, err = CalculateContext(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if output.Days[0].ExpectedBalance.Amount != 10000 || output.Days[0].MoneyForLife.Amount != 20000 {
		t.Fatalf("later control point did not reset MFL: %+v", output.Days[0])
	}
}
