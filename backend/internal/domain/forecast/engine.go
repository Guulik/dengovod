package forecast

import (
	"sort"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/domain"
)

type Input struct {
	Anchor    domain.BalanceAnchor
	Settings  domain.UserSettings
	Events    []domain.FinancialEvent
	Overrides []domain.DayCostOverride
	RangeFrom domain.CalendarDate
	RangeTo   domain.CalendarDate
	AsOf      time.Time
}

type Output struct {
	Days    []domain.DayProjection
	Summary domain.ForecastSummary
}

func ExpandEvents(events []domain.FinancialEvent, from, to domain.CalendarDate) map[string][]domain.EventOccurrence {
	result := make(map[string][]domain.EventOccurrence)
	for _, event := range events {
		add := func(date domain.CalendarDate) {
			if date.Before(from) || date.After(to) || (event.Recurrence.Until != nil && date.After(*event.Recurrence.Until)) {
				return
			}
			result[date.String()] = append(result[date.String()], domain.EventOccurrence{
				EventID: event.ID, Date: date, Kind: event.Kind, Necessity: event.Necessity,
				Amount: event.Amount, CategoryID: event.CategoryID, Comment: event.Comment,
			})
		}
		rule := event.Recurrence
		switch rule.Type {
		case domain.RecurrenceNone:
			add(rule.StartDate)
		case domain.RecurrenceEveryNDays:
			if rule.IntervalDays == nil || *rule.IntervalDays < 1 {
				continue
			}
			for d := rule.StartDate; !d.After(to) && (rule.Until == nil || !d.After(*rule.Until)); d = d.AddDays(*rule.IntervalDays) {
				add(d)
			}
		case domain.RecurrenceMonthlyOnDay:
			if rule.DayOfMonth == nil {
				continue
			}
			cursor := time.Date(rule.StartDate.Year, time.Month(rule.StartDate.Month), 1, 0, 0, 0, 0, domain.Moscow)
			limit := to.Time()
			for !cursor.After(limit) {
				last := time.Date(cursor.Year(), cursor.Month()+1, 0, 0, 0, 0, 0, domain.Moscow).Day()
				day := *rule.DayOfMonth
				if day > last {
					day = last
				}
				d := domain.CalendarDate{Year: cursor.Year(), Month: int(cursor.Month()), Day: day}
				if !d.Before(rule.StartDate) {
					add(d)
				}
				cursor = cursor.AddDate(0, 1, 0)
			}
		}
	}
	return result
}

func DayCost(date domain.CalendarDate, settings domain.UserSettings, overrides []domain.DayCostOverride) domain.Money {
	amount := settings.BaseDayCost.Amount
	var latest time.Time
	for _, override := range overrides {
		if !date.Before(override.DateFrom) && !date.After(override.DateTo) &&
			(latest.IsZero() || override.CreatedAt.After(latest)) {
			amount = override.DailyAmount.Amount
			latest = override.CreatedAt
		}
	}
	return domain.Rub(amount)
}

func ResolveHorizon(settings domain.UserSettings, events []domain.FinancialEvent, asOf domain.CalendarDate) (domain.CalendarDate, bool) {
	endOfMonth := domain.DateFromTime(time.Date(asOf.Year, time.Month(asOf.Month)+1, 0, 0, 0, 0, 0, domain.Moscow))
	switch settings.HorizonPolicy {
	case domain.HorizonCustomDate:
		if settings.HorizonCustomDate != nil {
			return *settings.HorizonCustomDate, true
		}
	case domain.HorizonNextIncome:
		var dates []domain.CalendarDate
		for _, event := range events {
			if event.Kind != domain.EventKindIncome {
				continue
			}
			if next, ok := nextOccurrence(event.Recurrence, asOf); ok {
				dates = append(dates, next)
			}
		}
		sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
		if len(dates) > 0 {
			return dates[0], false
		}
	}
	return endOfMonth, true
}

func nextOccurrence(rule domain.RecurrenceRule, from domain.CalendarDate) (domain.CalendarDate, bool) {
	var result domain.CalendarDate
	switch rule.Type {
	case domain.RecurrenceNone:
		result = rule.StartDate
	case domain.RecurrenceEveryNDays:
		if rule.IntervalDays == nil || *rule.IntervalDays < 1 {
			return result, false
		}
		result = rule.StartDate
		if result.Before(from) {
			elapsed := int(from.Time().Sub(result.Time()).Hours() / 24)
			steps := (elapsed + *rule.IntervalDays - 1) / *rule.IntervalDays
			result = result.AddDays(steps * *rule.IntervalDays)
		}
	case domain.RecurrenceMonthlyOnDay:
		if rule.DayOfMonth == nil {
			return result, false
		}
		cursor := time.Date(from.Year, time.Month(from.Month), 1, 0, 0, 0, 0, domain.Moscow)
		startMonth := time.Date(rule.StartDate.Year, time.Month(rule.StartDate.Month), 1, 0, 0, 0, 0, domain.Moscow)
		if cursor.Before(startMonth) {
			cursor = startMonth
		}
		for {
			last := time.Date(cursor.Year(), cursor.Month()+1, 0, 0, 0, 0, 0, domain.Moscow).Day()
			day := *rule.DayOfMonth
			if day > last {
				day = last
			}
			result = domain.CalendarDate{Year: cursor.Year(), Month: int(cursor.Month()), Day: day}
			if !result.Before(rule.StartDate) && !result.Before(from) {
				break
			}
			cursor = cursor.AddDate(0, 1, 0)
		}
	default:
		return result, false
	}
	if result.Before(from) || (rule.Until != nil && result.After(*rule.Until)) {
		return domain.CalendarDate{}, false
	}
	return result, true
}

func Calculate(input Input) Output {
	today := domain.DateFromTime(input.AsOf)
	anchorDate := domain.DateFromTime(input.Anchor.AnchoredAt)
	from := input.RangeFrom
	if from.Before(anchorDate) {
		// Balance before an anchor is unknowable. We intentionally return projections only
		// from max(requestedFrom, anchorDate), instead of inventing nullable balances.
		from = anchorDate
	}
	horizon, horizonInclusive := ResolveHorizon(input.Settings, input.Events, today)
	calcTo := input.RangeTo
	if horizon.After(calcTo) {
		calcTo = horizon
	}
	if today.After(calcTo) {
		calcTo = today
	}
	occurrences := ExpandEvents(input.Events, anchorDate, calcTo)

	balance := input.Anchor.Amount.Amount
	days := make([]domain.DayProjection, 0)
	var currentBalance int64 = balance
	for d := anchorDate; !d.After(calcTo); d = d.AddDays(1) {
		values := occurrences[d.String()]
		cost := DayCost(d, input.Settings, input.Overrides).Amount
		var income, required, optional int64
		for _, occurrence := range values {
			if occurrence.Kind == domain.EventKindIncome {
				income += occurrence.Amount.Amount
			} else if occurrence.Necessity == domain.NecessityOptional {
				optional += occurrence.Amount.Amount
			} else {
				required += occurrence.Amount.Amount
			}
		}
		// The anchor amount already includes all activity on its Moscow calendar date.
		if d.After(anchorDate) {
			balance += income - required - optional - cost
		}
		if d.String() == today.String() {
			currentBalance = balance
		}
		if !d.Before(from) && !d.After(input.RangeTo) {
			mfl := balance
			for future := d; !future.After(horizon); future = future.AddDays(1) {
				if !horizonInclusive && future.String() == horizon.String() {
					break
				}
				mfl -= DayCost(future, input.Settings, input.Overrides).Amount
				for _, occurrence := range occurrences[future.String()] {
					if occurrence.Kind == domain.EventKindExpense && occurrence.Necessity == domain.NecessityRequired {
						mfl -= occurrence.Amount.Amount
					}
				}
			}
			days = append(days, domain.DayProjection{
				Date: d, ExpectedBalance: domain.Rub(balance), DayCost: domain.Rub(cost), MoneyForLife: domain.Rub(mfl),
				Events: values, RequiredTotal: domain.Rub(required), OptionalTotal: domain.Rub(optional), IncomeTotal: domain.Rub(income),
			})
		}
	}

	var requiredRemaining, optionalRemaining, dayCostRemaining int64
	for d := today; !d.After(horizon); d = d.AddDays(1) {
		if !horizonInclusive && d.String() == horizon.String() {
			break
		}
		dayCostRemaining += DayCost(d, input.Settings, input.Overrides).Amount
		for _, occurrence := range occurrences[d.String()] {
			if occurrence.Kind != domain.EventKindExpense {
				continue
			}
			if occurrence.Necessity == domain.NecessityRequired {
				requiredRemaining += occurrence.Amount.Amount
			} else {
				optionalRemaining += occurrence.Amount.Amount
			}
		}
	}
	lastIncluded := horizon
	if !horizonInclusive {
		lastIncluded = horizon.AddDays(-1)
	}
	daysUntil := 0
	if !lastIncluded.Before(today) {
		daysUntil = int(lastIncluded.Time().Sub(today.Time()).Hours()/24) + 1
	}
	return Output{
		Days: days,
		Summary: domain.ForecastSummary{
			CurrentBalance: domain.Rub(currentBalance),
			MoneyForLife:   domain.Rub(currentBalance - requiredRemaining - dayCostRemaining),
			HorizonDate:    horizon, HorizonPolicy: input.Settings.HorizonPolicy,
			RequiredRemaining: domain.Rub(requiredRemaining), DayCostRemaining: domain.Rub(dayCostRemaining),
			OptionalRemaining: domain.Rub(optionalRemaining), DaysUntilHorizon: daysUntil,
		},
	}
}
