package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/alshatilov/dengovod/backend/internal/domain/forecast"
	"github.com/alshatilov/dengovod/backend/internal/domain/repository"
	"github.com/google/uuid"
)

type App struct {
	store repository.Store
	Now   func() time.Time
}

func New(store repository.Store) *App {
	return &App{store: store, Now: func() time.Time { return time.Now().UTC() }}
}

func (a *App) Login(ctx context.Context, login string) (domain.User, string, error) {
	user, err := a.store.ProvisionUser(ctx, login)
	if err != nil {
		return user, "", err
	}
	return user, user.ID.String(), nil
}

func (a *App) Authenticate(ctx context.Context, token string) (domain.User, error) {
	id, err := uuid.Parse(strings.TrimSpace(token))
	if err != nil {
		return domain.User{}, domain.ErrNotFound
	}
	return a.store.UserByID(ctx, id)
}

func (a *App) GetSettings(ctx context.Context, userID uuid.UUID) (domain.UserSettings, error) {
	return a.store.GetSettings(ctx, userID)
}

func (a *App) UpdateSettings(ctx context.Context, settings domain.UserSettings) (domain.UserSettings, error) {
	if settings.Currency == "" {
		settings.Currency = "RUB"
	}
	settings.BaseDayCost.Currency = settings.Currency
	if err := domain.ValidateSettings(settings); err != nil {
		return settings, err
	}
	settings.UpdatedAt = a.Now().UTC()
	if err := a.store.UpdateSettings(ctx, settings); err != nil {
		return settings, err
	}
	return a.store.GetSettings(ctx, settings.UserID)
}

func (a *App) SetAnchor(ctx context.Context, userID uuid.UUID, amount int64, anchoredAt time.Time) (domain.BalanceState, error) {
	if amount < 0 {
		return domain.BalanceState{}, domain.Validation("amount", "must be >= 0")
	}
	if anchoredAt.IsZero() {
		anchoredAt = a.Now()
	}
	anchor := domain.BalanceAnchor{
		ID: uuid.New(), UserID: userID, Amount: domain.Rub(amount),
		AnchoredAt: anchoredAt.UTC(), CreatedAt: a.Now().UTC(),
	}
	if err := a.store.CreateAnchor(ctx, anchor); err != nil {
		return domain.BalanceState{}, err
	}
	return a.GetBalance(ctx, userID)
}

func (a *App) GetBalance(ctx context.Context, userID uuid.UUID) (domain.BalanceState, error) {
	output, anchor, err := a.calculate(ctx, userID, domain.DateFromTime(a.Now()), domain.DateFromTime(a.Now()))
	if err != nil {
		return domain.BalanceState{}, err
	}
	return domain.BalanceState{Anchor: anchor, ComputedBalance: output.Summary.CurrentBalance, AsOf: a.Now().UTC()}, nil
}

func (a *App) CreateEvent(ctx context.Context, event domain.FinancialEvent) (domain.FinancialEvent, error) {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.Necessity == "" {
		event.Necessity = domain.NecessityRequired
	}
	event.Amount.Currency = "RUB"
	if err := domain.ValidateEvent(event); err != nil {
		return event, err
	}
	now := a.Now().UTC()
	event.CreatedAt, event.UpdatedAt = now, now
	if err := a.store.CreateEvent(ctx, event); err != nil {
		return event, err
	}
	return event, nil
}

func (a *App) UpdateEvent(ctx context.Context, event domain.FinancialEvent) (domain.FinancialEvent, error) {
	existing, err := a.store.GetEvent(ctx, event.UserID, event.ID)
	if err != nil {
		return event, err
	}
	event.CreatedAt = existing.CreatedAt
	event.UpdatedAt = a.Now().UTC()
	event.Amount.Currency = "RUB"
	if err = domain.ValidateEvent(event); err != nil {
		return event, err
	}
	if err = a.store.UpdateEvent(ctx, event); err != nil {
		return event, err
	}
	return a.store.GetEvent(ctx, event.UserID, event.ID)
}

func (a *App) DeleteEvent(ctx context.Context, userID, id uuid.UUID) error {
	return a.store.DeleteEvent(ctx, userID, id)
}
func (a *App) GetEvent(ctx context.Context, userID, id uuid.UUID) (domain.FinancialEvent, error) {
	return a.store.GetEvent(ctx, userID, id)
}
func (a *App) ListEvents(ctx context.Context, userID uuid.UUID) ([]domain.FinancialEvent, error) {
	return a.store.ListEvents(ctx, userID)
}

func (a *App) CreateOverride(ctx context.Context, value domain.DayCostOverride) (domain.DayCostOverride, error) {
	if value.ID == uuid.Nil {
		value.ID = uuid.New()
	}
	value.DailyAmount.Currency = "RUB"
	if err := domain.ValidateOverride(value); err != nil {
		return value, err
	}
	value.CreatedAt = a.Now().UTC()
	if err := a.store.CreateOverride(ctx, value); err != nil {
		return value, err
	}
	return value, nil
}
func (a *App) UpdateOverride(ctx context.Context, value domain.DayCostOverride) (domain.DayCostOverride, error) {
	existing, err := a.store.GetOverride(ctx, value.UserID, value.ID)
	if err != nil {
		return value, err
	}
	value.CreatedAt = existing.CreatedAt
	value.DailyAmount.Currency = "RUB"
	if err = domain.ValidateOverride(value); err != nil {
		return value, err
	}
	if err = a.store.UpdateOverride(ctx, value); err != nil {
		return value, err
	}
	return a.store.GetOverride(ctx, value.UserID, value.ID)
}
func (a *App) DeleteOverride(ctx context.Context, userID, id uuid.UUID) error {
	return a.store.DeleteOverride(ctx, userID, id)
}
func (a *App) ListOverrides(ctx context.Context, userID uuid.UUID) ([]domain.DayCostOverride, error) {
	return a.store.ListOverrides(ctx, userID)
}
func (a *App) ListCategories(ctx context.Context, userID uuid.UUID) ([]domain.Category, error) {
	return a.store.ListCategories(ctx, userID)
}

func (a *App) Calendar(ctx context.Context, userID uuid.UUID, from, to domain.CalendarDate) (forecast.Output, error) {
	if to.Before(from) {
		return forecast.Output{}, domain.Validation("to", "must not precede from")
	}
	output, _, err := a.calculate(ctx, userID, from, to)
	return output, err
}

func (a *App) Forecast(ctx context.Context, userID uuid.UUID) (domain.ForecastSummary, error) {
	today := domain.DateFromTime(a.Now())
	output, _, err := a.calculate(ctx, userID, today, today.AddDays(3660))
	return output.Summary, err
}

func (a *App) calculate(ctx context.Context, userID uuid.UUID, from, to domain.CalendarDate) (forecast.Output, domain.BalanceAnchor, error) {
	anchor, err := a.store.LatestAnchor(ctx, userID)
	if errors.Is(err, domain.ErrAnchorRequired) {
		return forecast.Output{}, anchor, &domain.DomainError{Code: "ANCHOR_REQUIRED", Message: "balance anchor is required", Err: domain.ErrAnchorRequired}
	}
	if err != nil {
		return forecast.Output{}, anchor, err
	}
	settings, err := a.store.GetSettings(ctx, userID)
	if err != nil {
		return forecast.Output{}, anchor, err
	}
	events, err := a.store.ListEvents(ctx, userID)
	if err != nil {
		return forecast.Output{}, anchor, err
	}
	overrides, err := a.store.ListOverrides(ctx, userID)
	if err != nil {
		return forecast.Output{}, anchor, err
	}
	return forecast.Calculate(forecast.Input{
		Anchor: anchor, Settings: settings, Events: events, Overrides: overrides,
		RangeFrom: from, RangeTo: to, AsOf: a.Now().UTC(),
	}), anchor, nil
}
