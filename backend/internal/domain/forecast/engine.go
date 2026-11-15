package forecast

import (
	"context"
	"github.com/alshatilov/dengovod/backend/internal/domain"
	"sort"
	"time"
)

type Input struct {
	Anchor             domain.BalanceAnchor
	Anchors            []domain.BalanceAnchor
	Settings           domain.UserSettings
	Events             []domain.FinancialEvent
	Occurrences        []domain.EventOccurrence
	LegacyFacts        []domain.EventOccurrence
	Overrides          []domain.DayCostOverride
	DayCosts           []domain.DayCostVersion
	RangeFrom, RangeTo domain.CalendarDate
	AsOf               time.Time
}
type Output struct {
	Days    []domain.DayProjection
	Summary *domain.ForecastSummary
	Anchor  *domain.BalanceAnchor
}

func key(o domain.EventOccurrence) string { return o.EventID.String() + ":" + o.Date.String() }
func expand(ctx context.Context, events []domain.FinancialEvent, from, to domain.CalendarDate) (map[string][]domain.EventOccurrence, error) {
	result := map[string][]domain.EventOccurrence{}
	for _, event := range events {
		for cursor := from; !cursor.After(to); {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			d, ok := domain.NextOccurrence(event.Recurrence, cursor)
			if !ok || d.After(to) {
				break
			}
			result[d.String()] = append(result[d.String()], domain.PlannedOccurrence(event, d))
			if d.Year == 9999 && d.Month == 12 && d.Day == 31 {
				break
			}
			cursor = d.AddDays(1)
		}
	}
	return result, nil
}
func ExpandEvents(events []domain.FinancialEvent, from, to domain.CalendarDate) map[string][]domain.EventOccurrence {
	result, _ := expand(context.Background(), events, from, to)
	return result
}
func DayCost(date domain.CalendarDate, settings domain.UserSettings, overrides []domain.DayCostOverride) domain.Money {
	amount := settings.BaseDayCost
	var latest *domain.DayCostOverride
	for i := range overrides {
		o := &overrides[i]
		if date.Before(o.DateFrom) || date.After(o.DateTo) {
			continue
		}
		if latest == nil || o.CreatedAt.After(latest.CreatedAt) || (o.CreatedAt.Equal(latest.CreatedAt) && o.ID.String() > latest.ID.String()) {
			latest = o
		}
	}
	if latest != nil {
		amount = latest.DailyAmount
	}
	return amount
}
func dayCost(date domain.CalendarDate, input Input) domain.Money {
	settings := input.Settings
	var selected *domain.DayCostVersion
	for i := range input.DayCosts {
		v := &input.DayCosts[i]
		if !v.EffectiveFrom.After(date) && (selected == nil || v.EffectiveFrom.After(selected.EffectiveFrom) || (v.EffectiveFrom == selected.EffectiveFrom && v.ID > selected.ID)) {
			selected = v
		}
	}
	if selected != nil {
		settings.BaseDayCost = selected.Amount
	}
	return DayCost(date, settings, input.Overrides)
}
func resolveHorizon(ctx context.Context, input Input, states map[string]domain.EventOccurrence) (domain.CalendarDate, bool, error) {
	today := domain.DateFromTime(input.AsOf)
	end := domain.CalendarDate{Year: today.Year, Month: today.Month, Day: time.Date(today.Year, time.Month(today.Month)+1, 0, 0, 0, 0, 0, domain.Moscow).Day()}
	if input.Settings.HorizonPolicy == domain.HorizonCustomDate && input.Settings.HorizonCustomDate != nil {
		return *input.Settings.HorizonCustomDate, true, nil
	}
	if input.Settings.HorizonPolicy == domain.HorizonNextIncome {
		var nearest *domain.CalendarDate
		for _, event := range input.Events {
			if event.Kind != domain.EventKindIncome {
				continue
			}
			cursor := today
			for {
				if err := ctx.Err(); err != nil {
					return end, true, err
				}
				d, ok := domain.NextOccurrence(event.Recurrence, cursor)
				if !ok {
					break
				}
				if _, excluded := states[key(domain.PlannedOccurrence(event, d))]; !excluded {
					if nearest == nil || d.Before(*nearest) {
						v := d
						nearest = &v
					}
					break
				}
				if d.Year == 9999 && d.Month == 12 && d.Day == 31 {
					break
				}
				cursor = d.AddDays(1)
			}
		}
		if nearest != nil {
			return *nearest, false, nil
		}
	}
	return end, true, nil
}
func ResolveHorizon(settings domain.UserSettings, events []domain.FinancialEvent, asOf domain.CalendarDate) (domain.CalendarDate, bool) {
	d, inclusive, _ := resolveHorizon(context.Background(), Input{Settings: settings, Events: events, AsOf: asOf.Time()}, nil)
	return d, inclusive
}

type ledgerPoint struct {
	at     time.Time
	seq    int64
	amount int64
	anchor *domain.BalanceAnchor
}

func points(input Input) []ledgerPoint {
	result := []ledgerPoint{}
	anchors := input.Anchors
	if len(anchors) == 0 && !input.Anchor.AnchoredAt.IsZero() {
		anchors = []domain.BalanceAnchor{input.Anchor}
	}
	for _, a := range anchors {
		if !a.AnchoredAt.After(input.AsOf) {
			v := a
			result = append(result, ledgerPoint{a.AnchoredAt, a.Sequence, a.Amount.Amount, &v})
		}
	}
	for _, o := range append(append([]domain.EventOccurrence{}, input.Occurrences...), input.LegacyFacts...) {
		if o.Status != domain.OccurrenceConfirmed || o.EffectiveAt == nil || o.EffectiveAt.After(input.AsOf) {
			continue
		}
		amount := o.Amount.Amount
		if o.Kind == domain.EventKindExpense {
			amount = -amount
		}
		result = append(result, ledgerPoint{*o.EffectiveAt, o.Sequence, amount, nil})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].at.Equal(result[j].at) {
			return result[i].seq < result[j].seq
		}
		return result[i].at.Before(result[j].at)
	})
	return result
}
func ActualBalance(ctx context.Context, input Input) (domain.Money, *domain.BalanceAnchor, error) {
	var amount int64
	var anchor *domain.BalanceAnchor
	for _, p := range points(input) {
		if err := ctx.Err(); err != nil {
			return domain.Money{}, nil, err
		}
		if p.anchor != nil {
			amount = p.amount
			anchor = p.anchor
		} else if anchor != nil {
			var err error
			amount, err = domain.AddMoney(amount, p.amount)
			if err != nil {
				return domain.Money{}, nil, err
			}
		}
	}
	if anchor == nil {
		return domain.Money{}, nil, domain.ErrAnchorRequired
	}
	return domain.Rub(amount), anchor, nil
}

// A suffix transforms a balance by adding subsequent effects, or replacing it
// with a constant when a later control point establishes a new balance.
type transform struct {
	reset bool
	value int64
}

func (t transform) apply(value int64) (int64, error) {
	if t.reset {
		return t.value, nil
	}
	return domain.AddMoney(value, t.value)
}
func compose(day, suffix transform) (transform, error) {
	if suffix.reset {
		return suffix, nil
	}
	v, err := domain.AddMoney(day.value, suffix.value)
	return transform{day.reset, v}, err
}

type calculationDay struct {
	projection                                      domain.DayProjection
	planIncome, planRequired, planOptional, reserve int64
	ledger                                          transform
}

func moneyPtr(value int64) *domain.Money { m := domain.Rub(value); return &m }
func Calculate(input Input) Output {
	output, _ := CalculateContext(context.Background(), input)
	return output
}
func CalculateContext(ctx context.Context, input Input) (Output, error) {
	var output Output
	states := map[string]domain.EventOccurrence{}
	for _, o := range input.Occurrences {
		states[key(o)] = o
	}
	horizon, inclusive, err := resolveHorizon(ctx, input, states)
	if err != nil {
		return output, err
	}
	last := horizon
	if !inclusive {
		last = last.AddDays(-1)
	}
	today := domain.DateFromTime(input.AsOf)
	from := input.RangeFrom
	if today.Before(from) {
		from = today
	}
	to := input.RangeTo
	if today.After(to) {
		to = today
	}
	if last.After(to) {
		to = last
	}
	if err = ctx.Err(); err != nil {
		return output, err
	}
	plan, err := expand(ctx, input.Events, from, to)
	if err != nil {
		return output, err
	}
	for day, items := range plan {
		filtered := []domain.EventOccurrence{}
		for _, o := range items {
			if saved, ok := states[key(o)]; ok {
				if saved.Status == domain.OccurrenceCancelled {
					o.Status = domain.OccurrenceCancelled
				} else {
					continue
				}
			}
			filtered = append(filtered, o)
		}
		plan[day] = filtered
	}
	for _, o := range append(append([]domain.EventOccurrence{}, input.Occurrences...), input.LegacyFacts...) {
		if o.Status == domain.OccurrenceCancelled && !o.SeriesDeleted {
			found := false
			for _, planned := range plan[o.Date.String()] {
				if key(planned) == key(o) {
					found = true
					break
				}
			}
			if !found && !o.Date.Before(from) && !o.Date.After(to) {
				plan[o.Date.String()] = append(plan[o.Date.String()], o)
			}
			continue
		}
		if o.Status != domain.OccurrenceConfirmed || o.EffectiveAt == nil || o.EffectiveAt.After(input.AsOf) {
			continue
		}
		d := domain.DateFromTime(*o.EffectiveAt)
		if d.Before(from) || d.After(to) {
			continue
		}
		plan[d.String()] = append(plan[d.String()], o)
	}
	ledger := points(input)
	var current int64
	var currentAnchor *domain.BalanceAnchor
	for _, p := range ledger {
		if p.anchor != nil {
			current = p.amount
			currentAnchor = p.anchor
		} else if currentAnchor != nil {
			current, err = domain.AddMoney(current, p.amount)
			if err != nil {
				return Output{}, err
			}
		}
	}
	output.Anchor = currentAnchor
	var actual int64
	known := false
	pointIndex := 0
	apply := func(p ledgerPoint) error {
		if p.anchor != nil {
			actual = p.amount
			known = true
		} else if known {
			var err error
			actual, err = domain.AddMoney(actual, p.amount)
			return err
		}
		return nil
	}
	for pointIndex < len(ledger) && ledger[pointIndex].at.Before(from.Time()) {
		if err = apply(ledger[pointIndex]); err != nil {
			return Output{}, err
		}
		pointIndex++
	}
	var projected int64 = current
	days := []calculationDay{}
	summary := domain.ForecastSummary{CurrentBalance: domain.Rub(current), HorizonDate: horizon, HorizonPolicy: input.Settings.HorizonPolicy}
	var summaryIncome int64
	for d := from; !d.After(to); d = d.AddDays(1) {
		if err = ctx.Err(); err != nil {
			return Output{}, err
		}
		day := calculationDay{projection: domain.DayProjection{Date: d, DayCost: dayCost(d, input), Events: plan[d.String()], RequiredTotal: domain.Rub(0), OptionalTotal: domain.Rub(0), IncomeTotal: domain.Rub(0)}}
		for pointIndex < len(ledger) && ledger[pointIndex].at.Before(d.AddDays(1).Time()) {
			p := ledger[pointIndex]
			if err = apply(p); err != nil {
				return Output{}, err
			}
			if p.anchor != nil {
				day.ledger = transform{true, p.amount}
			} else if known {
				day.ledger.value, err = domain.AddMoney(day.ledger.value, p.amount)
				if err != nil {
					return Output{}, err
				}
			}
			pointIndex++
		}
		var marked int64
		for _, o := range day.projection.Events {
			if o.Status == domain.OccurrenceCancelled {
				continue
			}
			total := &day.projection.IncomeTotal.Amount
			if o.Kind == domain.EventKindExpense {
				total = &day.projection.RequiredTotal.Amount
				if o.Necessity == domain.NecessityOptional {
					total = &day.projection.OptionalTotal.Amount
				}
			}
			*total, err = domain.AddMoney(*total, o.Amount.Amount)
			if err != nil {
				return Output{}, err
			}
			if o.Status == domain.OccurrenceConfirmed && o.CountsTowardDayBudget && o.Kind == domain.EventKindExpense {
				marked, err = domain.AddMoney(marked, o.Amount.Amount)
				if err != nil {
					return Output{}, err
				}
			}
			if o.Status != domain.OccurrencePlanned || d.Before(today) || (currentAnchor != nil && d.Before(domain.DateFromTime(currentAnchor.AnchoredAt))) {
				continue
			}
			p := &day.planIncome
			if o.Kind == domain.EventKindExpense {
				p = &day.planRequired
				if o.Necessity == domain.NecessityOptional {
					p = &day.planOptional
				}
			}
			*p, err = domain.AddMoney(*p, o.Amount.Amount)
			if err != nil {
				return Output{}, err
			}
		}
		day.reserve = day.projection.DayCost.Amount - marked
		if day.reserve < 0 {
			day.reserve = 0
		}
		if d.Before(today) {
			if known {
				day.projection.ExpectedBalance = moneyPtr(actual)
			}
		} else if currentAnchor != nil {
			for _, delta := range []int64{day.planIncome, -day.planRequired, -day.planOptional, -day.reserve} {
				projected, err = domain.AddMoney(projected, delta)
				if err != nil {
					return Output{}, err
				}
			}
			day.projection.ExpectedBalance = moneyPtr(projected)
		}
		if currentAnchor != nil && !d.Before(today) && !d.After(last) {
			summary.DaysUntilHorizon++
			for _, a := range []struct {
				to     *int64
				amount int64
			}{{&summaryIncome, day.planIncome}, {&summary.RequiredRemaining.Amount, day.planRequired}, {&summary.OptionalRemaining.Amount, day.planOptional}, {&summary.DayCostRemaining.Amount, day.reserve}} {
				*a.to, err = domain.AddMoney(*a.to, a.amount)
				if err != nil {
					return Output{}, err
				}
			}
		}
		days = append(days, day)
	}
	if currentAnchor != nil {
		value := current
		for _, delta := range []int64{summaryIncome, -summary.RequiredRemaining.Amount, -summary.DayCostRemaining.Amount} {
			value, err = domain.AddMoney(value, delta)
			if err != nil {
				return Output{}, err
			}
		}
		summary.MoneyForLife = domain.Rub(value)
		summary.RequiredRemaining.Currency = "RUB"
		summary.OptionalRemaining.Currency = "RUB"
		summary.DayCostRemaining.Currency = "RUB"
		output.Summary = &summary
	}
	suffix := transform{}
	for i := len(days) - 1; i >= 0; i-- {
		if err = ctx.Err(); err != nil {
			return Output{}, err
		}
		day := &days[i]
		d := day.projection.Date
		if day.projection.ExpectedBalance == nil {
			break
		}
		if day.projection.ExpectedBalance != nil {
			value := day.projection.ExpectedBalance.Amount
			if d.Before(last) {
				value, err = suffix.apply(value)
				if err != nil {
					return Output{}, err
				}
			}
			day.projection.MoneyForLife = moneyPtr(value)
		}
		if !d.After(last) {
			daily := day.ledger
			for _, delta := range []int64{day.planIncome, -day.planRequired, -day.reserve} {
				daily.value, err = domain.AddMoney(daily.value, delta)
				if err != nil {
					return Output{}, err
				}
			}
			suffix, err = compose(daily, suffix)
			if err != nil {
				return Output{}, err
			}
		}
	}
	output.Days = []domain.DayProjection{}
	for _, day := range days {
		if !day.projection.Date.Before(input.RangeFrom) && !day.projection.Date.After(input.RangeTo) {
			output.Days = append(output.Days, day.projection)
		}
	}
	return output, nil
}
