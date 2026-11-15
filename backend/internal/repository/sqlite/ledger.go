package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) beginWrite(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	// Acquire SQLite's writer lock before reading. All confirmation and anchor
	// writers serialize here, including separate Store instances/processes.
	if _, err = tx.ExecContext(ctx, `UPDATE ledger_clock SET value=value WHERE id=1`); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func nextSequence(ctx context.Context, tx *sql.Tx) (int64, error) {
	var seq int64
	err := tx.QueryRowContext(ctx, `UPDATE ledger_clock SET value=value+1 WHERE id=1 RETURNING value`).Scan(&seq)
	return seq, err
}

const occurrenceSelect = `SELECT o.event_id,o.occurrence_date,o.status,o.effective_at,o.sequence,o.kind,o.necessity,o.amount,o.category_id,o.comment,o.counts_toward_day_budget,(e.deleted_at IS NOT NULL)
 FROM occurrence_states o JOIN financial_events e ON e.id=o.event_id`

func scanOccurrence(row scanner) (domain.EventOccurrence, error) {
	var value domain.EventOccurrence
	var date string
	var at sql.NullTime
	var seq sql.NullInt64
	var category sql.NullString
	err := row.Scan(&value.EventID, &date, &value.Status, &at, &seq, &value.Kind, &value.Necessity, &value.Amount.Amount, &category, &value.Comment, &value.CountsTowardDayBudget, &value.SeriesDeleted)
	if err != nil {
		return value, normalize(err)
	}
	value.Date, err = domain.ParseDate(date)
	if err != nil {
		return value, err
	}
	value.Amount.Currency = "RUB"
	if at.Valid {
		v := at.Time.UTC()
		value.EffectiveAt = &v
	}
	value.Sequence = seq.Int64
	if category.Valid {
		id, err := uuid.Parse(category.String)
		if err != nil {
			return value, err
		}
		value.CategoryID = &id
	}
	return value, nil
}

func (s *Store) ChangeOccurrence(ctx context.Context, userID, eventID uuid.UUID, date domain.CalendarDate, change domain.OccurrenceChange) (domain.OccurrenceResult, error) {
	var result domain.OccurrenceResult
	tx, err := s.beginWrite(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	event, err := scanEvent(tx.QueryRowContext(ctx, eventSelect+` WHERE id=? AND user_id=?`, eventID, userID))
	if err != nil {
		return result, err
	}
	var deleted sql.NullTime
	if err = tx.QueryRowContext(ctx, `SELECT deleted_at FROM financial_events WHERE id=? AND user_id=?`, eventID, userID).Scan(&deleted); err != nil {
		return result, err
	}
	if deleted.Valid {
		return result, domain.ErrConflict
	}
	saved, err := scanOccurrence(tx.QueryRowContext(ctx, occurrenceSelect+` WHERE o.event_id=? AND o.user_id=? AND o.occurrence_date=?`, eventID, userID, date.String()))
	exists := err == nil
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return result, err
	}
	valid := domain.OccursOn(event.Recurrence, date)
	var removed int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM occurrence_unconfirmations WHERE event_id=? AND occurrence_date=?`, eventID, date.String()).Scan(&removed); err != nil {
		return result, err
	}
	if !valid && !(exists && saved.Status == domain.OccurrenceConfirmed) && !(change.Status == domain.OccurrencePlanned && (exists || removed > 0)) {
		return result, domain.ErrNotFound
	}
	result.EventOccurrence = domain.PlannedOccurrence(event, date)
	switch change.Status {
	case domain.OccurrencePlanned:
		if exists {
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO occurrence_unconfirmations VALUES(?,?)`, eventID, date.String()); err != nil {
				return result, err
			}
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM occurrence_states WHERE event_id=? AND user_id=? AND occurrence_date=?`, eventID, userID, date.String()); err != nil {
			return result, err
		}
		result.PlanRestored = valid
	case domain.OccurrenceCancelled:
		if exists && saved.Status == domain.OccurrenceConfirmed {
			return result, domain.ErrConflict
		}
		if !valid {
			return result, domain.ErrNotFound
		}
		result.Status = domain.OccurrenceCancelled
	case domain.OccurrenceConfirmed:
		if exists && saved.Status == domain.OccurrenceConfirmed {
			if (change.EffectiveAt != nil && !change.EffectiveAt.Equal(*saved.EffectiveAt)) ||
				(change.CountsTowardDayBudget != nil && *change.CountsTowardDayBudget != saved.CountsTowardDayBudget) {
				return result, domain.ErrConflict
			}
			result.EventOccurrence = saved
			return result, tx.Commit()
		}
		if !valid {
			return result, domain.ErrNotFound
		}
		if date.After(domain.DateFromTime(change.Now)) {
			return result, domain.Validation("occurrence_date", "cannot confirm a future occurrence")
		}
		at := change.Now.UTC()
		if date.Before(domain.DateFromTime(change.Now)) {
			at = date.Time().Add(12 * time.Hour).UTC()
		}
		if change.EffectiveAt != nil {
			at = change.EffectiveAt.UTC()
		}
		if at.After(change.Now) || at.Year() < 1 || at.Year() > 9999 {
			return result, domain.Validation("effective_at", "must be a non-future timestamp from year 0001")
		}
		budget := change.CountsTowardDayBudget != nil && *change.CountsTowardDayBudget
		if budget && event.Kind != domain.EventKindExpense {
			return result, domain.Validation("counts_toward_day_budget", "is only allowed for expenses")
		}
		seq, err := nextSequence(ctx, tx)
		if err != nil {
			return result, err
		}
		result.Status = domain.OccurrenceConfirmed
		result.EffectiveAt = &at
		result.Sequence = seq
		result.CountsTowardDayBudget = budget
	default:
		return result, domain.Validation("status", "is invalid")
	}
	if change.Status != domain.OccurrencePlanned {
		var category, at, seq any
		if result.CategoryID != nil {
			category = *result.CategoryID
		}
		if result.EffectiveAt != nil {
			at = *result.EffectiveAt
			seq = result.Sequence
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO occurrence_states(event_id,user_id,occurrence_date,status,effective_at,sequence,kind,necessity,amount,category_id,comment,counts_toward_day_budget,created_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(event_id,occurrence_date) DO UPDATE SET status=excluded.status,effective_at=excluded.effective_at,sequence=excluded.sequence,kind=excluded.kind,necessity=excluded.necessity,amount=excluded.amount,category_id=excluded.category_id,comment=excluded.comment,counts_toward_day_budget=excluded.counts_toward_day_budget,created_at=excluded.created_at`,
			eventID, userID, date.String(), result.Status, at, seq, result.Kind, result.Necessity, result.Amount.Amount, category, result.Comment, result.CountsTowardDayBudget, change.Now.UTC())
		if err != nil {
			return result, err
		}
	}
	if err = validateLedger(ctx, tx, userID); err != nil {
		return result, err
	}
	return result, tx.Commit()
}

// Check all known historical segments inside the writer transaction. Undo can
// overflow too (for example, removing an expense that offset a later income).
func validateLedger(ctx context.Context, tx *sql.Tx, userID uuid.UUID) error {
	rows, err := tx.QueryContext(ctx, `SELECT at,sequence,amount,is_anchor FROM (
 SELECT anchored_at AS at,sequence,amount,1 AS is_anchor FROM anchors WHERE user_id=?
 UNION ALL SELECT effective_at,sequence,CASE kind WHEN 'INCOME' THEN amount ELSE -amount END,0 FROM occurrence_states WHERE user_id=? AND status='CONFIRMED'
 UNION ALL SELECT effective_at,sequence,CASE kind WHEN 'INCOME' THEN amount ELSE -amount END,0 FROM legacy_facts WHERE user_id=?
 ) ORDER BY julianday(at),sequence`, userID, userID, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type point struct {
		at          time.Time
		seq, amount int64
		anchor      bool
	}
	var points []point
	for rows.Next() {
		var p point
		if err = rows.Scan(&p.at, &p.seq, &p.amount, &p.anchor); err != nil {
			return err
		}
		points = append(points, p)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].at.Equal(points[j].at) {
			return points[i].seq < points[j].seq
		}
		return points[i].at.Before(points[j].at)
	})
	var balance int64
	known := false
	for _, p := range points {
		if p.amount > domain.MaxMoney || p.amount < -domain.MaxMoney {
			return domain.CalculationOverflow()
		}
		if p.anchor {
			balance = p.amount
			known = true
		} else if known {
			balance, err = domain.AddMoney(balance, p.amount)
			if err != nil {
				return err
			}
		}
	}
	return rows.Err()
}

func readRows(ctx context.Context, tx *sql.Tx, query string, userID uuid.UUID, scan func(*sql.Rows) error) error {
	rows, err := tx.QueryContext(ctx, query, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err = scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) FinancialSnapshot(ctx context.Context, userID uuid.UUID) (domain.FinancialSnapshot, error) {
	var result domain.FinancialSnapshot
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var custom sql.NullString
	v := &result.Settings
	err = tx.QueryRowContext(ctx, `SELECT user_id,base_day_cost,horizon_policy,horizon_custom_date,currency,updated_at,needs_balance_review FROM settings WHERE user_id=?`, userID).
		Scan(&v.UserID, &v.BaseDayCost.Amount, &v.HorizonPolicy, &custom, &v.Currency, &v.UpdatedAt, &result.NeedsBalanceReview)
	if err != nil {
		return result, normalize(err)
	}
	v.BaseDayCost.Currency = v.Currency
	if custom.Valid {
		date, err := domain.ParseDate(custom.String)
		if err != nil {
			return result, err
		}
		v.HorizonCustomDate = &date
	}
	queries := []struct {
		sql  string
		scan func(*sql.Rows) error
	}{
		{`SELECT id,user_id,amount,anchored_at,created_at,sequence FROM anchors WHERE user_id=?`, func(rows *sql.Rows) error {
			var a domain.BalanceAnchor
			err := rows.Scan(&a.ID, &a.UserID, &a.Amount.Amount, &a.AnchoredAt, &a.CreatedAt, &a.Sequence)
			a.Amount.Currency = "RUB"
			result.Anchors = append(result.Anchors, a)
			return err
		}},
		{eventSelect + ` WHERE user_id=? AND deleted_at IS NULL ORDER BY created_at,id`, func(rows *sql.Rows) error {
			e, err := scanEvent(rows)
			result.Events = append(result.Events, e)
			return err
		}},
		{occurrenceSelect + ` WHERE o.user_id=? AND e.user_id=o.user_id ORDER BY o.event_id,o.occurrence_date`, func(rows *sql.Rows) error {
			o, err := scanOccurrence(rows)
			result.Occurrences = append(result.Occurrences, o)
			return err
		}},
		{`SELECT id,effective_at,sequence,kind,amount,category_id,comment FROM legacy_facts WHERE user_id=? ORDER BY sequence`, func(rows *sql.Rows) error {
			var o domain.EventOccurrence
			var at time.Time
			var category sql.NullString
			if err := rows.Scan(&o.EventID, &at, &o.Sequence, &o.Kind, &o.Amount.Amount, &category, &o.Comment); err != nil {
				return err
			}
			o.Date = domain.DateFromTime(at)
			o.EffectiveAt = &at
			o.Status = domain.OccurrenceConfirmed
			o.Necessity = domain.NecessityRequired
			o.SeriesDeleted = true
			o.Amount.Currency = "RUB"
			if category.Valid {
				id, err := uuid.Parse(category.String)
				if err != nil {
					return err
				}
				o.CategoryID = &id
			}
			result.LegacyFacts = append(result.LegacyFacts, o)
			return nil
		}},
		{overrideSelect + ` WHERE user_id=? ORDER BY created_at,id`, func(rows *sql.Rows) error {
			o, err := scanOverride(rows)
			result.Overrides = append(result.Overrides, o)
			return err
		}},
		{`SELECT id,effective_from,amount FROM base_day_cost_history WHERE user_id=? ORDER BY effective_from,id`, func(rows *sql.Rows) error {
			var cost domain.DayCostVersion
			var date string
			if err := rows.Scan(&cost.ID, &date, &cost.Amount.Amount); err != nil {
				return err
			}
			d, err := domain.ParseDate(date)
			cost.EffectiveFrom = d
			cost.Amount.Currency = "RUB"
			result.DayCosts = append(result.DayCosts, cost)
			return err
		}},
	}
	for _, q := range queries {
		if err = readRows(ctx, tx, q.sql, userID, q.scan); err != nil {
			return result, err
		}
	}
	return result, tx.Commit()
}
