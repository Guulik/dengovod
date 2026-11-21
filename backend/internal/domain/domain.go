package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var Moscow = time.FixedZone("Europe/Moscow", 3*60*60)

var (
	ErrNotFound       = errors.New("not found")
	ErrValidation     = errors.New("validation")
	ErrConflict       = errors.New("conflict")
	ErrAnchorRequired = errors.New("anchor required")
)

type DomainError struct {
	Code    string
	Message string
	Field   string
	Err     error
}

func (e *DomainError) Error() string { return e.Message }
func (e *DomainError) Unwrap() error { return e.Err }

func Validation(field, message string) error {
	return &DomainError{Code: "VALIDATION_ERROR", Message: message, Field: field, Err: ErrValidation}
}

type Money struct {
	Amount   int64
	Currency string
}

func Rub(amount int64) Money { return Money{Amount: amount, Currency: "RUB"} }

type CalendarDate struct {
	Year  int
	Month int
	Day   int
}

func ParseDate(value string) (CalendarDate, error) {
	t, err := time.Parse("2006-01-02", value)
	if err != nil || t.Year() < 1 {
		return CalendarDate{}, Validation("date", "must be YYYY-MM-DD")
	}
	return CalendarDate{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}, nil
}

func DateFromTime(t time.Time) CalendarDate {
	t = t.In(Moscow)
	return CalendarDate{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
}

func (d CalendarDate) Time() time.Time {
	return time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, Moscow)
}

func (d CalendarDate) String() string                 { return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day) }
func (d CalendarDate) Before(other CalendarDate) bool { return d.Time().Before(other.Time()) }
func (d CalendarDate) After(other CalendarDate) bool  { return d.Time().After(other.Time()) }
func (d CalendarDate) AddDays(days int) CalendarDate {
	return DateFromTime(d.Time().AddDate(0, 0, days))
}

type RecurrenceType string

const (
	RecurrenceNone         RecurrenceType = "NONE"
	RecurrenceMonthlyOnDay RecurrenceType = "MONTHLY_ON_DAY"
	RecurrenceEveryNDays   RecurrenceType = "EVERY_N_DAYS"
)

type RecurrenceRule struct {
	Type         RecurrenceType
	StartDate    CalendarDate
	DayOfMonth   *int
	IntervalDays *int
	Until        *CalendarDate
}

type HorizonPolicy string

const (
	HorizonEndOfMonth HorizonPolicy = "END_OF_MONTH"
	HorizonNextIncome HorizonPolicy = "NEXT_INCOME"
	HorizonCustomDate HorizonPolicy = "CUSTOM_DATE"
)

type EventKind string

const (
	EventKindIncome  EventKind = "INCOME"
	EventKindExpense EventKind = "EXPENSE"
)

type EventNecessity string

const (
	NecessityRequired EventNecessity = "REQUIRED"
	NecessityOptional EventNecessity = "OPTIONAL"
)

type User struct {
	ID        uuid.UUID
	Login     string
	CreatedAt time.Time
}
type UserSettings struct {
	UserID            uuid.UUID
	BaseDayCost       Money
	HorizonPolicy     HorizonPolicy
	HorizonCustomDate *CalendarDate
	Currency          string
	UpdatedAt         time.Time
}
type BalanceAnchor struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Amount     Money
	AnchoredAt time.Time
	CreatedAt  time.Time
	Sequence   int64
}

const CategorySystemSalary = "SALARY"

type Category struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Name       string
	Kind       EventKind
	IsDefault  bool
	SystemCode *string
	CreatedAt  time.Time
}
type FinancialEvent struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Kind       EventKind
	Necessity  EventNecessity
	Amount     Money
	CategoryID *uuid.UUID
	Comment    string
	Recurrence RecurrenceRule
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
type DayCostOverride struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	DateFrom    CalendarDate
	DateTo      CalendarDate
	DailyAmount Money
	Comment     string
	CreatedAt   time.Time
	Priority    int64
}
type EventOccurrence struct {
	EventID               uuid.UUID
	Date                  CalendarDate
	Kind                  EventKind
	Necessity             EventNecessity
	Amount                Money
	CategoryID            *uuid.UUID
	Comment               string
	IsOverridden          bool
	Status                OccurrenceStatus
	EffectiveAt           *time.Time
	Sequence              int64
	CountsTowardDayBudget bool
	SeriesDeleted         bool
}
type OccurrenceStatus string

const (
	OccurrencePlanned   OccurrenceStatus = "PLANNED"
	OccurrenceConfirmed OccurrenceStatus = "CONFIRMED"
	OccurrenceCancelled OccurrenceStatus = "CANCELLED"
)

type OccurrenceChange struct {
	Status                OccurrenceStatus
	EffectiveAt           *time.Time
	CountsTowardDayBudget *bool
	Now                   time.Time
}
type OccurrenceResult struct {
	EventOccurrence
	PlanRestored bool
}
type DayCostVersion struct {
	ID            int64
	EffectiveFrom CalendarDate
	Amount        Money
}
type FinancialSnapshot struct {
	Settings           UserSettings
	Anchors            []BalanceAnchor
	Events             []FinancialEvent
	Occurrences        []EventOccurrence
	LegacyFacts        []EventOccurrence
	Overrides          []DayCostOverride
	DayCosts           []DayCostVersion
	NeedsBalanceReview bool
}
type DayProjection struct {
	Date            CalendarDate
	ExpectedBalance *Money
	DayCost         Money
	MoneyForLife    *Money
	Events          []EventOccurrence
	RequiredTotal   Money
	OptionalTotal   Money
	IncomeTotal     Money
}
type ForecastSummary struct {
	CurrentBalance    Money
	MoneyForLife      Money
	HorizonDate       CalendarDate
	HorizonPolicy     HorizonPolicy
	RequiredRemaining Money
	DayCostRemaining  Money
	OptionalRemaining Money
	DaysUntilHorizon  int
}
type BalanceState struct {
	Anchor             BalanceAnchor
	ComputedBalance    Money
	AsOf               time.Time
	NeedsBalanceReview bool
}

func ValidateSettings(s UserSettings) error {
	if s.BaseDayCost.Amount < 0 || s.BaseDayCost.Amount > MaxMoney {
		return Validation("base_day_cost", "must be between 0 and 9007199254740991")
	}
	switch s.HorizonPolicy {
	case HorizonEndOfMonth, HorizonNextIncome:
	case HorizonCustomDate:
		if s.HorizonCustomDate == nil {
			return Validation("horizon_custom_date", "is required")
		}
	default:
		return Validation("horizon_policy", "is invalid")
	}
	return nil
}

func ValidateEvent(e FinancialEvent) error {
	if e.Amount.Amount <= 0 || e.Amount.Amount > MaxMoney {
		return Validation("amount", "must be between 1 and 9007199254740991")
	}
	if e.Kind != EventKindIncome && e.Kind != EventKindExpense {
		return Validation("kind", "is invalid")
	}
	if e.Necessity != NecessityRequired && e.Necessity != NecessityOptional {
		return Validation("necessity", "is invalid")
	}
	switch e.Recurrence.Type {
	case RecurrenceNone:
	case RecurrenceMonthlyOnDay:
		if e.Recurrence.DayOfMonth == nil || *e.Recurrence.DayOfMonth < 1 || *e.Recurrence.DayOfMonth > 31 {
			return Validation("recurrence.day_of_month", "must be between 1 and 31")
		}
	case RecurrenceEveryNDays:
		if e.Recurrence.IntervalDays == nil || *e.Recurrence.IntervalDays < 1 {
			return Validation("recurrence.interval_days", "must be >= 1")
		}
	default:
		return Validation("recurrence.type", "is invalid")
	}
	if e.Recurrence.Until != nil && e.Recurrence.Until.Before(e.Recurrence.StartDate) {
		return Validation("recurrence.until_date", "must not precede start_date")
	}
	return nil
}

func ValidateOverride(o DayCostOverride) error {
	if o.DateTo.Before(o.DateFrom) {
		return Validation("date_to", "must not precede date_from")
	}
	if o.DailyAmount.Amount < 0 || o.DailyAmount.Amount > MaxMoney {
		return Validation("daily_amount", "must be between 0 and 9007199254740991")
	}
	return nil
}
