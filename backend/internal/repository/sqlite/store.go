package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if path == "" {
		path = "data/dengovod.db"
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	if err = applyMigrations(context.Background(), db, migrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) ProvisionUser(ctx context.Context, login string) (domain.User, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return domain.User{}, domain.Validation("login", "is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback()
	var user domain.User
	err = tx.QueryRowContext(ctx, `SELECT id, login, created_at FROM users WHERE login = ? COLLATE NOCASE`, login).
		Scan(&user.ID, &user.Login, &user.CreatedAt)
	if err == nil {
		return user, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, err
	}
	now := time.Now().UTC()
	user = domain.User{ID: uuid.New(), Login: login, CreatedAt: now}
	if _, err = tx.ExecContext(ctx, `INSERT INTO users(id,login,created_at) VALUES(?,?,?)`, user.ID, user.Login, now); err != nil {
		return domain.User{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings(user_id,base_day_cost,horizon_policy,currency,updated_at) VALUES(?,0,'END_OF_MONTH','RUB',?)`, user.ID, now); err != nil {
		return domain.User{}, err
	}
	defaults := []struct {
		name string
		kind domain.EventKind
	}{
		{"Жильё", domain.EventKindExpense}, {"Еда", domain.EventKindExpense},
		{"Транспорт", domain.EventKindExpense}, {"Здоровье", domain.EventKindExpense},
		{"Развлечения", domain.EventKindExpense}, {"Прочее", domain.EventKindExpense},
		{"Зарплата", domain.EventKindIncome}, {"Подработка", domain.EventKindIncome},
		{"Прочее", domain.EventKindIncome},
	}
	for _, item := range defaults {
		var systemCode any
		if item.name == "Зарплата" && item.kind == domain.EventKindIncome {
			systemCode = domain.CategorySystemSalary
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO categories(id,user_id,name,kind,is_default,created_at,system_code) VALUES(?,?,?,?,1,?,?)`,
			uuid.New(), user.ID, item.name, item.kind, now, systemCode); err != nil {
			return domain.User{}, err
		}
	}
	return user, tx.Commit()
}

func (s *Store) UserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	var user domain.User
	err := s.db.QueryRowContext(ctx, `SELECT id,login,created_at FROM users WHERE id=?`, id).Scan(&user.ID, &user.Login, &user.CreatedAt)
	return user, normalize(err)
}

func (s *Store) GetSettings(ctx context.Context, userID uuid.UUID) (domain.UserSettings, error) {
	var result domain.UserSettings
	var custom sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT user_id,base_day_cost,horizon_policy,horizon_custom_date,currency,updated_at FROM settings WHERE user_id=?`, userID).
		Scan(&result.UserID, &result.BaseDayCost.Amount, &result.HorizonPolicy, &custom, &result.Currency, &result.UpdatedAt)
	if err != nil {
		return result, normalize(err)
	}
	result.BaseDayCost.Currency = result.Currency
	if custom.Valid {
		date, parseErr := domain.ParseDate(custom.String)
		if parseErr != nil {
			return result, parseErr
		}
		result.HorizonCustomDate = &date
	}
	return result, nil
}

func (s *Store) UpdateSettings(ctx context.Context, value domain.UserSettings) error {
	var custom any
	if value.HorizonCustomDate != nil {
		custom = value.HorizonCustomDate.String()
	}
	result, err := s.db.ExecContext(ctx, `UPDATE settings SET base_day_cost=?,horizon_policy=?,horizon_custom_date=?,currency=?,updated_at=? WHERE user_id=?`,
		value.BaseDayCost.Amount, value.HorizonPolicy, custom, value.Currency, value.UpdatedAt.UTC(), value.UserID)
	return affected(result, err)
}

func (s *Store) CreateAnchor(ctx context.Context, value domain.BalanceAnchor) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO anchors(id,user_id,amount,anchored_at,created_at) VALUES(?,?,?,?,?)`,
		value.ID, value.UserID, value.Amount.Amount, value.AnchoredAt.UTC(), value.CreatedAt.UTC())
	return err
}

func (s *Store) LatestAnchor(ctx context.Context, userID uuid.UUID) (domain.BalanceAnchor, error) {
	var value domain.BalanceAnchor
	err := s.db.QueryRowContext(ctx, `SELECT id,user_id,amount,anchored_at,created_at FROM anchors WHERE user_id=? ORDER BY anchored_at DESC,created_at DESC LIMIT 1`, userID).
		Scan(&value.ID, &value.UserID, &value.Amount.Amount, &value.AnchoredAt, &value.CreatedAt)
	value.Amount.Currency = "RUB"
	if errors.Is(err, sql.ErrNoRows) {
		return value, domain.ErrAnchorRequired
	}
	return value, err
}

func eventArgs(value domain.FinancialEvent) []any {
	var category, day, interval, until any
	if value.CategoryID != nil {
		category = *value.CategoryID
	}
	if value.Recurrence.DayOfMonth != nil {
		day = *value.Recurrence.DayOfMonth
	}
	if value.Recurrence.IntervalDays != nil {
		interval = *value.Recurrence.IntervalDays
	}
	if value.Recurrence.Until != nil {
		until = value.Recurrence.Until.String()
	}
	return []any{value.Kind, value.Necessity, value.Amount.Amount, category, value.Comment, value.Recurrence.Type,
		value.Recurrence.StartDate.String(), day, interval, until, value.UpdatedAt.UTC()}
}

func (s *Store) CreateEvent(ctx context.Context, value domain.FinancialEvent) error {
	if err := s.validateCategory(ctx, value.UserID, value.CategoryID); err != nil {
		return err
	}
	args := []any{value.ID, value.UserID}
	args = append(args, eventArgs(value)...)
	args = append(args, value.CreatedAt.UTC())
	_, err := s.db.ExecContext(ctx, `INSERT INTO financial_events(id,user_id,kind,necessity,amount,category_id,comment,recurrence_type,start_date,day_of_month,interval_days,until_date,updated_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
	return err
}

func (s *Store) UpdateEvent(ctx context.Context, value domain.FinancialEvent) error {
	if err := s.validateCategory(ctx, value.UserID, value.CategoryID); err != nil {
		return err
	}
	args := eventArgs(value)
	args = append(args, value.ID, value.UserID)
	result, err := s.db.ExecContext(ctx, `UPDATE financial_events SET kind=?,necessity=?,amount=?,category_id=?,comment=?,recurrence_type=?,start_date=?,day_of_month=?,interval_days=?,until_date=?,updated_at=? WHERE id=? AND user_id=?`, args...)
	return affected(result, err)
}

func (s *Store) DeleteEvent(ctx context.Context, userID, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM financial_events WHERE id=? AND user_id=?`, id, userID)
	return affected(result, err)
}

type scanner interface{ Scan(...any) error }

func scanEvent(row scanner) (domain.FinancialEvent, error) {
	var value domain.FinancialEvent
	var category sql.NullString
	var day, interval sql.NullInt64
	var until sql.NullString
	var start string
	err := row.Scan(&value.ID, &value.UserID, &value.Kind, &value.Necessity, &value.Amount.Amount, &category,
		&value.Comment, &value.Recurrence.Type, &start, &day, &interval, &until, &value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		return value, normalize(err)
	}
	value.Amount.Currency = "RUB"
	value.Recurrence.StartDate, err = domain.ParseDate(start)
	if err != nil {
		return value, err
	}
	if category.Valid {
		id, parseErr := uuid.Parse(category.String)
		if parseErr != nil {
			return value, parseErr
		}
		value.CategoryID = &id
	}
	if day.Valid {
		number := int(day.Int64)
		value.Recurrence.DayOfMonth = &number
	}
	if interval.Valid {
		number := int(interval.Int64)
		value.Recurrence.IntervalDays = &number
	}
	if until.Valid {
		date, parseErr := domain.ParseDate(until.String)
		if parseErr != nil {
			return value, parseErr
		}
		value.Recurrence.Until = &date
	}
	return value, nil
}

const eventSelect = `SELECT id,user_id,kind,necessity,amount,category_id,comment,recurrence_type,start_date,day_of_month,interval_days,until_date,created_at,updated_at FROM financial_events`

func (s *Store) GetEvent(ctx context.Context, userID, id uuid.UUID) (domain.FinancialEvent, error) {
	return scanEvent(s.db.QueryRowContext(ctx, eventSelect+` WHERE id=? AND user_id=?`, id, userID))
}

func (s *Store) ListEvents(ctx context.Context, userID uuid.UUID) ([]domain.FinancialEvent, error) {
	rows, err := s.db.QueryContext(ctx, eventSelect+` WHERE user_id=? ORDER BY created_at,id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []domain.FinancialEvent{}
	for rows.Next() {
		value, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) CreateOverride(ctx context.Context, value domain.DayCostOverride) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO day_cost_overrides(id,user_id,date_from,date_to,daily_amount,comment,created_at) VALUES(?,?,?,?,?,?,?)`,
		value.ID, value.UserID, value.DateFrom.String(), value.DateTo.String(), value.DailyAmount.Amount, value.Comment, value.CreatedAt.UTC())
	return err
}

func (s *Store) UpdateOverride(ctx context.Context, value domain.DayCostOverride) error {
	result, err := s.db.ExecContext(ctx, `UPDATE day_cost_overrides SET date_from=?,date_to=?,daily_amount=?,comment=? WHERE id=? AND user_id=?`,
		value.DateFrom.String(), value.DateTo.String(), value.DailyAmount.Amount, value.Comment, value.ID, value.UserID)
	return affected(result, err)
}

func (s *Store) DeleteOverride(ctx context.Context, userID, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM day_cost_overrides WHERE id=? AND user_id=?`, id, userID)
	return affected(result, err)
}

func scanOverride(row scanner) (domain.DayCostOverride, error) {
	var value domain.DayCostOverride
	var from, to string
	err := row.Scan(&value.ID, &value.UserID, &from, &to, &value.DailyAmount.Amount, &value.Comment, &value.CreatedAt)
	if err != nil {
		return value, normalize(err)
	}
	value.DailyAmount.Currency = "RUB"
	if value.DateFrom, err = domain.ParseDate(from); err != nil {
		return value, err
	}
	value.DateTo, err = domain.ParseDate(to)
	return value, err
}

const overrideSelect = `SELECT id,user_id,date_from,date_to,daily_amount,comment,created_at FROM day_cost_overrides`

func (s *Store) GetOverride(ctx context.Context, userID, id uuid.UUID) (domain.DayCostOverride, error) {
	return scanOverride(s.db.QueryRowContext(ctx, overrideSelect+` WHERE id=? AND user_id=?`, id, userID))
}

func (s *Store) ListOverrides(ctx context.Context, userID uuid.UUID) ([]domain.DayCostOverride, error) {
	rows, err := s.db.QueryContext(ctx, overrideSelect+` WHERE user_id=? ORDER BY created_at,id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []domain.DayCostOverride{}
	for rows.Next() {
		value, scanErr := scanOverride(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) ListCategories(ctx context.Context, userID uuid.UUID) ([]domain.Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,name,kind,is_default,created_at,system_code FROM categories WHERE user_id=? ORDER BY kind,name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []domain.Category{}
	for rows.Next() {
		var value domain.Category
		if err = rows.Scan(&value.ID, &value.UserID, &value.Name, &value.Kind, &value.IsDefault, &value.CreatedAt, &value.SystemCode); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) validateCategory(ctx context.Context, userID uuid.UUID, categoryID *uuid.UUID) error {
	if categoryID == nil {
		return nil
	}
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM categories WHERE id=? AND user_id=?`, *categoryID, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Validation("category_id", "does not belong to user")
	}
	return err
}

func normalize(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func affected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return domain.ErrNotFound
	}
	return err
}
