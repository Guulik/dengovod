package repository

import (
	"context"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/google/uuid"
)

type UserRepository interface {
	ProvisionUser(context.Context, string) (domain.User, error)
	UserByID(context.Context, uuid.UUID) (domain.User, error)
}
type SettingsRepository interface {
	GetSettings(context.Context, uuid.UUID) (domain.UserSettings, error)
	UpdateSettings(context.Context, domain.UserSettings) error
}
type BalanceRepository interface {
	CreateAnchor(context.Context, domain.BalanceAnchor) error
	LatestAnchor(context.Context, uuid.UUID) (domain.BalanceAnchor, error)
}
type EventRepository interface {
	CreateEvent(context.Context, domain.FinancialEvent) error
	UpdateEvent(context.Context, domain.FinancialEvent) error
	DeleteEvent(context.Context, uuid.UUID, uuid.UUID) error
	GetEvent(context.Context, uuid.UUID, uuid.UUID) (domain.FinancialEvent, error)
	ListEvents(context.Context, uuid.UUID) ([]domain.FinancialEvent, error)
}
type DayCostOverrideRepository interface {
	CreateOverride(context.Context, domain.DayCostOverride) error
	UpdateOverride(context.Context, domain.DayCostOverride) error
	DeleteOverride(context.Context, uuid.UUID, uuid.UUID) error
	GetOverride(context.Context, uuid.UUID, uuid.UUID) (domain.DayCostOverride, error)
	ListOverrides(context.Context, uuid.UUID) ([]domain.DayCostOverride, error)
}
type CategoryRepository interface {
	ListCategories(context.Context, uuid.UUID) ([]domain.Category, error)
}

type Store interface {
	UserRepository
	SettingsRepository
	BalanceRepository
	EventRepository
	DayCostOverrideRepository
	CategoryRepository
	ChangeOccurrence(context.Context, uuid.UUID, uuid.UUID, domain.CalendarDate, domain.OccurrenceChange) (domain.OccurrenceResult, error)
	FinancialSnapshot(context.Context, uuid.UUID) (domain.FinancialSnapshot, error)
}
