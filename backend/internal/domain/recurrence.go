package domain

import "time"

// DaysBetween uses calendar midnights, avoiding time.Duration's 290-year limit.
func DaysBetween(from, to CalendarDate) int64 {
	return (to.Time().Unix() - from.Time().Unix()) / 86400
}

func NextOccurrence(rule RecurrenceRule, from CalendarDate) (CalendarDate, bool) {
	if from.Before(rule.StartDate) {
		from = rule.StartDate
	}
	var result CalendarDate
	switch rule.Type {
	case RecurrenceNone:
		result = rule.StartDate
	case RecurrenceEveryNDays:
		if rule.IntervalDays == nil || *rule.IntervalDays < 1 {
			return result, false
		}
		elapsed := DaysBetween(rule.StartDate, from)
		interval := int64(*rule.IntervalDays)
		steps := elapsed / interval
		if elapsed%interval != 0 {
			steps++
		}
		limit := DaysBetween(rule.StartDate, CalendarDate{9999, 12, 31})
		if steps > limit/interval {
			return result, false
		}
		result = rule.StartDate.AddDays(int(steps * interval))
	case RecurrenceMonthlyOnDay:
		if rule.DayOfMonth == nil || *rule.DayOfMonth < 1 || *rule.DayOfMonth > 31 {
			return result, false
		}
		cursor := time.Date(from.Year, time.Month(from.Month), 1, 0, 0, 0, 0, Moscow)
		for i := 0; i < 2; i++ {
			day := *rule.DayOfMonth
			last := time.Date(cursor.Year(), cursor.Month()+1, 0, 0, 0, 0, 0, Moscow).Day()
			if day > last {
				day = last
			}
			result = CalendarDate{cursor.Year(), int(cursor.Month()), day}
			if !result.Before(from) {
				break
			}
			cursor = cursor.AddDate(0, 1, 0)
		}
	default:
		return result, false
	}
	if result.Year > 9999 || result.Before(from) || (rule.Until != nil && result.After(*rule.Until)) {
		return CalendarDate{}, false
	}
	return result, true
}

func OccursOn(rule RecurrenceRule, date CalendarDate) bool {
	next, ok := NextOccurrence(rule, date)
	return ok && next == date
}

func PlannedOccurrence(event FinancialEvent, date CalendarDate) EventOccurrence {
	return EventOccurrence{EventID: event.ID, Date: date, Kind: event.Kind, Necessity: event.Necessity,
		Amount: event.Amount, CategoryID: event.CategoryID, Comment: event.Comment, Status: OccurrencePlanned}
}
