package forecast

import (
	"context"
	"sort"

	"github.com/alshatilov/dengovod/backend/internal/domain"
)

// EstimatedBalance keeps the stored ledger intact and subtracts only the
// unspent budgets of completed Moscow days since the latest reconciliation.
// Today's budget belongs to the forecast until the next local midnight.
func EstimatedBalance(ctx context.Context, input Input) (domain.Money, *domain.BalanceAnchor, error) {
	raw, anchor, err := ActualBalance(ctx, input)
	if err != nil {
		return domain.Money{}, nil, err
	}
	amount, err := estimateThrough(ctx, input, raw.Amount, anchor, domain.DateFromTime(input.AsOf).AddDays(-1))
	if err != nil {
		return domain.Money{}, nil, err
	}
	return domain.Rub(amount), anchor, nil
}

func estimateThrough(ctx context.Context, input Input, raw int64, anchor *domain.BalanceAnchor, through domain.CalendarDate) (int64, error) {
	reserve, err := reserveBetween(ctx, input, domain.DateFromTime(anchor.AnchoredAt), through)
	if err != nil {
		return 0, err
	}
	return domain.AddMoney(raw, -reserve)
}

// historicalGap represents invisible past days without expanding recurrences
// or walking calendar days. Overdue plans have no financial effect. A control
// point resets earlier effects; remaining reserve is integrated between the
// finite dates on which base costs, overrides or marked actual spending change.
func historicalGap(ctx context.Context, input Input, ledger []ledgerPoint, from, to domain.CalendarDate) (transform, error) {
	result := transform{}
	if to.Before(from) {
		return result, nil
	}
	start, end := from.Time(), to.AddDays(1).Time()
	first := sort.Search(len(ledger), func(i int) bool { return !ledger[i].at.Before(start) })
	stop := sort.Search(len(ledger), func(i int) bool { return !ledger[i].at.Before(end) })
	lastAnchor := -1
	for i := first; i < stop; i++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if ledger[i].anchor != nil {
			lastAnchor = i
		}
	}
	if lastAnchor >= 0 {
		result = transform{true, ledger[lastAnchor].amount}
		from = domain.DateFromTime(ledger[lastAnchor].at)
		first = lastAnchor + 1
	}
	for i := first; i < stop; i++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var err error
		result.value, err = domain.AddMoney(result.value, ledger[i].amount)
		if err != nil {
			return transform{}, err
		}
	}
	reserve, err := reserveBetween(ctx, input, from, to)
	if err != nil {
		return transform{}, err
	}
	result.value, err = domain.AddMoney(result.value, -reserve)
	return result, err
}

func reserveBetween(ctx context.Context, input Input, from, to domain.CalendarDate) (int64, error) {
	if to.Before(from) {
		return 0, nil
	}
	end := to.AddDays(1)
	boundaries := map[domain.CalendarDate]bool{from: true, end: true}
	add := func(date domain.CalendarDate) {
		if date.After(from) && date.Before(end) {
			boundaries[date] = true
		}
	}
	for _, cost := range input.DayCosts {
		add(cost.EffectiveFrom)
	}
	for _, override := range input.Overrides {
		add(override.DateFrom)
		add(override.DateTo.AddDays(1))
	}
	marked := map[domain.CalendarDate]int64{}
	for _, o := range input.Occurrences {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if o.Status != domain.OccurrenceConfirmed || o.EffectiveAt == nil || o.EffectiveAt.After(input.AsOf) || o.Kind != domain.EventKindExpense || !o.CountsTowardDayBudget {
			continue
		}
		date := domain.DateFromTime(*o.EffectiveAt)
		if date.Before(from) || date.After(to) {
			continue
		}
		amount, err := domain.AddMoney(marked[date], o.Amount.Amount)
		if err != nil {
			return 0, err
		}
		marked[date] = amount
		add(date)
		add(date.AddDays(1))
	}
	dates := make([]domain.CalendarDate, 0, len(boundaries))
	for date := range boundaries {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	var total int64
	for i := 0; i+1 < len(dates); i++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		cost := dayCost(dates[i], input).Amount - marked[dates[i]]
		if cost < 0 {
			cost = 0
		}
		days := domain.DaysBetween(dates[i], dates[i+1])
		if cost > 0 && days > domain.MaxMoney/cost {
			return 0, domain.CalculationOverflow()
		}
		var err error
		total, err = domain.AddMoney(total, cost*days)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}
