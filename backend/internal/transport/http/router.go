package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/domain"
	"github.com/alshatilov/dengovod/backend/internal/domain/forecast"
	"github.com/alshatilov/dengovod/backend/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type handler struct{ app *service.App }
type contextKey string

const userKey contextKey = "user"

type moneyDTO struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}
type recurrenceDTO struct {
	Type         domain.RecurrenceType `json:"type"`
	StartDate    string                `json:"start_date"`
	DayOfMonth   *int                  `json:"day_of_month"`
	IntervalDays *int                  `json:"interval_days"`
	UntilDate    *string               `json:"until_date"`
}
type eventRequest struct {
	Kind       *domain.EventKind      `json:"kind"`
	Necessity  *domain.EventNecessity `json:"necessity"`
	Amount     *moneyDTO              `json:"amount"`
	CategoryID *string                `json:"category_id"`
	Comment    *string                `json:"comment"`
	Recurrence *recurrenceDTO         `json:"recurrence"`
}
type overrideRequest struct {
	DateFrom    string   `json:"date_from"`
	DateTo      string   `json:"date_to"`
	DailyAmount moneyDTO `json:"daily_amount"`
	Comment     string   `json:"comment"`
}

func NewRouter(app *service.App) http.Handler {
	h := &handler{app: app}
	r := chi.NewRouter()
	r.Use(recoverer, cors)
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", h.login)
		r.Group(func(r chi.Router) {
			r.Use(h.auth)
			r.Get("/settings", h.getSettings)
			r.Put("/settings", h.putSettings)
			r.Get("/balance", h.getBalance)
			r.Post("/balance/anchor", h.postAnchor)
			r.Get("/events", h.listEvents)
			r.Post("/events", h.postEvent)
			r.Get("/events/{id}", h.getEvent)
			r.Put("/events/{id}", h.putEvent)
			r.Delete("/events/{id}", h.deleteEvent)
			r.Get("/day-cost-overrides", h.listOverrides)
			r.Post("/day-cost-overrides", h.postOverride)
			r.Put("/day-cost-overrides/{id}", h.putOverride)
			r.Delete("/day-cost-overrides/{id}", h.deleteOverride)
			r.Get("/categories", h.listCategories)
			r.Get("/calendar", h.calendar)
			r.Get("/forecast", h.forecast)
		})
	})
	return r
}

func (h *handler) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.Header.Get("Authorization")
		if !strings.HasPrefix(value, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token", "")
			return
		}
		user, err := h.app.Authenticate(r.Context(), strings.TrimPrefix(value, "Bearer "))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid bearer token", "")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, user.ID)))
	})
}

func userID(r *http.Request) uuid.UUID { return r.Context().Value(userKey).(uuid.UUID) }

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Login string `json:"login"`
	}
	if !decode(w, r, &request) {
		return
	}
	user, token, err := h.app.Login(r.Context(), request.Login)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": map[string]any{
		"id": user.ID, "login": user.Login, "created_at": user.CreatedAt.UTC(),
	}, "token": token})
}

func settingsResponse(value domain.UserSettings) map[string]any {
	var custom any
	if value.HorizonCustomDate != nil {
		custom = value.HorizonCustomDate.String()
	}
	return map[string]any{
		"base_day_cost": money(value.BaseDayCost.Amount), "horizon_policy": value.HorizonPolicy,
		"horizon_custom_date": custom, "currency": value.Currency, "updated_at": value.UpdatedAt.UTC(),
	}
}

func (h *handler) getSettings(w http.ResponseWriter, r *http.Request) {
	value, err := h.app.GetSettings(r.Context(), userID(r))
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse(value))
}

func (h *handler) putSettings(w http.ResponseWriter, r *http.Request) {
	var request struct {
		BaseDayCost       moneyDTO             `json:"base_day_cost"`
		HorizonPolicy     domain.HorizonPolicy `json:"horizon_policy"`
		HorizonCustomDate *string              `json:"horizon_custom_date"`
	}
	if !decode(w, r, &request) {
		return
	}
	value := domain.UserSettings{UserID: userID(r), BaseDayCost: domain.Rub(request.BaseDayCost.Amount),
		HorizonPolicy: request.HorizonPolicy, Currency: "RUB"}
	if request.HorizonCustomDate != nil {
		date, err := domain.ParseDate(*request.HorizonCustomDate)
		if err != nil {
			respondError(w, err)
			return
		}
		value.HorizonCustomDate = &date
	}
	value, err := h.app.UpdateSettings(r.Context(), value)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse(value))
}

func balanceResponse(value domain.BalanceState) map[string]any {
	return map[string]any{
		"anchor":           map[string]any{"id": value.Anchor.ID, "amount": money(value.Anchor.Amount.Amount), "anchored_at": value.Anchor.AnchoredAt.UTC()},
		"computed_balance": money(value.ComputedBalance.Amount), "as_of": value.AsOf.UTC(),
	}
}

func (h *handler) getBalance(w http.ResponseWriter, r *http.Request) {
	value, err := h.app.GetBalance(r.Context(), userID(r))
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, balanceResponse(value))
}

func (h *handler) postAnchor(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Amount     moneyDTO   `json:"amount"`
		AnchoredAt *time.Time `json:"anchored_at"`
	}
	if !decode(w, r, &request) {
		return
	}
	var at time.Time
	if request.AnchoredAt != nil {
		at = request.AnchoredAt.UTC()
	}
	value, err := h.app.SetAnchor(r.Context(), userID(r), request.Amount.Amount, at)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, balanceResponse(value))
}

func parseRecurrence(value recurrenceDTO) (domain.RecurrenceRule, error) {
	start, err := domain.ParseDate(value.StartDate)
	if err != nil {
		return domain.RecurrenceRule{}, err
	}
	result := domain.RecurrenceRule{Type: value.Type, StartDate: start, DayOfMonth: value.DayOfMonth, IntervalDays: value.IntervalDays}
	if value.UntilDate != nil {
		until, parseErr := domain.ParseDate(*value.UntilDate)
		if parseErr != nil {
			return result, parseErr
		}
		result.Until = &until
	}
	return result, nil
}

func applyEventRequest(value *domain.FinancialEvent, request eventRequest) error {
	if request.Kind != nil {
		value.Kind = *request.Kind
	}
	if request.Necessity != nil {
		value.Necessity = *request.Necessity
	}
	if request.Amount != nil {
		value.Amount = domain.Rub(request.Amount.Amount)
	}
	if request.CategoryID != nil {
		if *request.CategoryID == "" {
			value.CategoryID = nil
		} else {
			id, err := uuid.Parse(*request.CategoryID)
			if err != nil {
				return domain.Validation("category_id", "must be a UUID")
			}
			value.CategoryID = &id
		}
	}
	if request.Comment != nil {
		value.Comment = *request.Comment
	}
	if request.Recurrence != nil {
		recurrence, err := parseRecurrence(*request.Recurrence)
		if err != nil {
			return err
		}
		value.Recurrence = recurrence
	}
	return nil
}

func eventResponse(value domain.FinancialEvent) map[string]any {
	var category any
	if value.CategoryID != nil {
		category = value.CategoryID.String()
	}
	var until any
	if value.Recurrence.Until != nil {
		until = value.Recurrence.Until.String()
	}
	return map[string]any{
		"id": value.ID, "kind": value.Kind, "necessity": value.Necessity, "amount": money(value.Amount.Amount),
		"category_id": category, "comment": value.Comment,
		"recurrence": map[string]any{"type": value.Recurrence.Type, "start_date": value.Recurrence.StartDate.String(),
			"day_of_month": value.Recurrence.DayOfMonth, "interval_days": value.Recurrence.IntervalDays, "until_date": until},
		"created_at": value.CreatedAt.UTC(), "updated_at": value.UpdatedAt.UTC(),
	}
}

func (h *handler) postEvent(w http.ResponseWriter, r *http.Request) {
	var request eventRequest
	if !decode(w, r, &request) {
		return
	}
	value := domain.FinancialEvent{UserID: userID(r)}
	if err := applyEventRequest(&value, request); err != nil {
		respondError(w, err)
		return
	}
	value, err := h.app.CreateEvent(r.Context(), value)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, eventResponse(value))
}

func parsePathID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, domain.Validation("id", "must be a UUID")
	}
	return id, nil
}

func (h *handler) getEvent(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r)
	if err != nil {
		respondError(w, err)
		return
	}
	value, err := h.app.GetEvent(r.Context(), userID(r), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, eventResponse(value))
}
func (h *handler) listEvents(w http.ResponseWriter, r *http.Request) {
	values, err := h.app.ListEvents(r.Context(), userID(r))
	if err != nil {
		respondError(w, err)
		return
	}
	items := []any{}
	for _, value := range values {
		if kind := r.URL.Query().Get("kind"); kind == "" || kind == string(value.Kind) {
			items = append(items, eventResponse(value))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": items})
}
func (h *handler) putEvent(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r)
	if err != nil {
		respondError(w, err)
		return
	}
	value, err := h.app.GetEvent(r.Context(), userID(r), id)
	if err != nil {
		respondError(w, err)
		return
	}
	var request eventRequest
	if !decode(w, r, &request) {
		return
	}
	if err = applyEventRequest(&value, request); err != nil {
		respondError(w, err)
		return
	}
	value, err = h.app.UpdateEvent(r.Context(), value)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, eventResponse(value))
}
func (h *handler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r)
	if err != nil {
		respondError(w, err)
		return
	}
	if err = h.app.DeleteEvent(r.Context(), userID(r), id); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func overrideResponse(value domain.DayCostOverride) map[string]any {
	return map[string]any{"id": value.ID, "date_from": value.DateFrom.String(), "date_to": value.DateTo.String(),
		"daily_amount": money(value.DailyAmount.Amount), "comment": value.Comment, "created_at": value.CreatedAt.UTC()}
}
func mapOverride(user uuid.UUID, request overrideRequest) (domain.DayCostOverride, error) {
	from, err := domain.ParseDate(request.DateFrom)
	if err != nil {
		return domain.DayCostOverride{}, err
	}
	to, err := domain.ParseDate(request.DateTo)
	if err != nil {
		return domain.DayCostOverride{}, err
	}
	return domain.DayCostOverride{UserID: user, DateFrom: from, DateTo: to, DailyAmount: domain.Rub(request.DailyAmount.Amount), Comment: request.Comment}, nil
}
func (h *handler) postOverride(w http.ResponseWriter, r *http.Request) {
	var request overrideRequest
	if !decode(w, r, &request) {
		return
	}
	value, err := mapOverride(userID(r), request)
	if err != nil {
		respondError(w, err)
		return
	}
	value, err = h.app.CreateOverride(r.Context(), value)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, overrideResponse(value))
}
func (h *handler) listOverrides(w http.ResponseWriter, r *http.Request) {
	values, err := h.app.ListOverrides(r.Context(), userID(r))
	if err != nil {
		respondError(w, err)
		return
	}
	items := []any{}
	for _, value := range values {
		items = append(items, overrideResponse(value))
	}
	writeJSON(w, http.StatusOK, map[string]any{"overrides": items})
}
func (h *handler) putOverride(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r)
	if err != nil {
		respondError(w, err)
		return
	}
	var request overrideRequest
	if !decode(w, r, &request) {
		return
	}
	value, err := mapOverride(userID(r), request)
	if err != nil {
		respondError(w, err)
		return
	}
	value.ID = id
	value, err = h.app.UpdateOverride(r.Context(), value)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overrideResponse(value))
}
func (h *handler) deleteOverride(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r)
	if err != nil {
		respondError(w, err)
		return
	}
	if err = h.app.DeleteOverride(r.Context(), userID(r), id); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) listCategories(w http.ResponseWriter, r *http.Request) {
	values, err := h.app.ListCategories(r.Context(), userID(r))
	if err != nil {
		respondError(w, err)
		return
	}
	items := []any{}
	for _, value := range values {
		if kind := r.URL.Query().Get("kind"); kind == "" || kind == string(value.Kind) {
			items = append(items, map[string]any{"id": value.ID, "name": value.Name, "kind": value.Kind, "is_default": value.IsDefault})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": items})
}

func occurrenceResponse(value domain.EventOccurrence) map[string]any {
	var category any
	if value.CategoryID != nil {
		category = value.CategoryID.String()
	}
	return map[string]any{"event_id": value.EventID, "kind": value.Kind, "necessity": value.Necessity,
		"amount": money(value.Amount.Amount), "category_id": category, "comment": value.Comment}
}
func summaryResponse(value domain.ForecastSummary) map[string]any {
	return map[string]any{"current_balance": money(value.CurrentBalance.Amount), "money_for_life": money(value.MoneyForLife.Amount),
		"horizon_date": value.HorizonDate.String(), "horizon_policy": value.HorizonPolicy,
		"required_remaining": money(value.RequiredRemaining.Amount), "day_cost_remaining": money(value.DayCostRemaining.Amount),
		"optional_remaining": money(value.OptionalRemaining.Amount), "days_until_horizon": value.DaysUntilHorizon}
}
func calendarResponse(value forecast.Output) map[string]any {
	days := []any{}
	for _, day := range value.Days {
		events := []any{}
		for _, event := range day.Events {
			events = append(events, occurrenceResponse(event))
		}
		days = append(days, map[string]any{"date": day.Date.String(), "expected_balance": money(day.ExpectedBalance.Amount),
			"day_cost": money(day.DayCost.Amount), "money_for_life": money(day.MoneyForLife.Amount),
			"income_total": money(day.IncomeTotal.Amount), "required_total": money(day.RequiredTotal.Amount),
			"optional_total": money(day.OptionalTotal.Amount), "events": events})
	}
	return map[string]any{"days": days, "summary": summaryResponse(value.Summary)}
}
func (h *handler) calendar(w http.ResponseWriter, r *http.Request) {
	from, err := domain.ParseDate(r.URL.Query().Get("from"))
	if err != nil {
		respondError(w, err)
		return
	}
	to, err := domain.ParseDate(r.URL.Query().Get("to"))
	if err != nil {
		respondError(w, err)
		return
	}
	value, err := h.app.Calendar(r.Context(), userID(r), from, to)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, calendarResponse(value))
}
func (h *handler) forecast(w http.ResponseWriter, r *http.Request) {
	value, err := h.app.Forecast(r.Context(), userID(r))
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summaryResponse(value))
}

func money(amount int64) moneyDTO { return moneyDTO{Amount: amount, Currency: "RUB"} }
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "malformed JSON request", "")
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func respondError(w http.ResponseWriter, err error) {
	var typed *domain.DomainError
	if errors.As(err, &typed) {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, domain.ErrAnchorRequired) {
			status = http.StatusConflict
		}
		writeError(w, status, typed.Code, typed.Message, typed.Field)
		return
	}
	if errors.Is(err, domain.ErrAnchorRequired) {
		writeError(w, http.StatusConflict, "ANCHOR_REQUIRED", "balance anchor is required", "")
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found", "")
		return
	}
	if errors.Is(err, domain.ErrConflict) {
		writeError(w, http.StatusConflict, "CONFLICT", err.Error(), "")
		return
	}
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", "")
}
func writeError(w http.ResponseWriter, status int, code, message, field string) {
	details := []any{}
	if field != "" {
		details = append(details, map[string]string{"field": field, "message": message})
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "details": details}})
}
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
