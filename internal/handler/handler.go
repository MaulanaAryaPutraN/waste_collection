package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/pengumpulan_sampah/internal/domain"
	"example.com/pengumpulan_sampah/internal/response"
	"example.com/pengumpulan_sampah/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type API struct {
	households domain.HouseholdService
	pickups    domain.PickupService
	payments   domain.PaymentService
	reports    domain.ReportService
	files      storage.FileStore
	uploadDir  string
	maxUpload  int64
	validate   *validator.Validate
	logger     *slog.Logger
}

func NewRouter(h domain.HouseholdService, p domain.PickupService, pay domain.PaymentService,
	reports domain.ReportService, files storage.FileStore, uploadDir string, maxUploadMB int64, logger *slog.Logger) http.Handler {
	api := &API{households: h, pickups: p, payments: pay, reports: reports, files: files,
		uploadDir: uploadDir, maxUpload: maxUploadMB * 1024 * 1024, validate: validator.New(), logger: logger}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))
	r.Use(panicRecovery(logger))
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		response.Success(w, http.StatusOK, "healthy", map[string]string{"status": "ok"}, nil)
	})
	r.Route("/api", func(r chi.Router) {
		r.Post("/households", api.createHousehold)
		r.Get("/households", api.listHouseholds)
		r.Get("/households/{id}", api.getHousehold)
		r.Delete("/households/{id}", api.deleteHousehold)
		r.Post("/pickups", api.createPickup)
		r.Get("/pickups", api.listPickups)
		r.Put("/pickups/{id}/schedule", api.schedulePickup)
		r.Put("/pickups/{id}/complete", api.completePickup)
		r.Put("/pickups/{id}/cancel", api.cancelPickup)
		r.Post("/payments", api.createPayment)
		r.Get("/payments", api.listPayments)
		r.Put("/payments/{id}/confirm", api.confirmPayment)
		r.Get("/reports/waste-summary", api.wasteSummary)
		r.Get("/reports/payment-summary", api.paymentSummary)
	})
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadDir))))
	return r
}

type createHouseholdRequest struct {
	OwnerName string `json:"owner_name" validate:"required,max=200"`
	Address   string `json:"address" validate:"required,max=1000"`
}

func (a *API) createHousehold(w http.ResponseWriter, r *http.Request) {
	var req createHouseholdRequest
	if !a.decode(w, r, &req) {
		return
	}
	if !a.validateRequest(w, req) {
		return
	}
	item, err := a.households.Create(r.Context(), req.OwnerName, req.Address)
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, "household created", item, nil)
}

func (a *API) listHouseholds(w http.ResponseWriter, r *http.Request) {
	p, ok := a.pagination(w, r)
	if !ok {
		return
	}
	result, err := a.households.List(r.Context(), p)
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusOK, "households retrieved", result.Items, pageMeta(result))
}

func (a *API) getHousehold(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r)
	if !ok {
		return
	}
	item, err := a.households.Get(r.Context(), id)
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusOK, "household retrieved", item, nil)
}

func (a *API) deleteHousehold(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r)
	if !ok {
		return
	}
	if err := a.households.Delete(r.Context(), id); err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusOK, "household deleted", map[string]string{"id": id.String()}, nil)
}

type createPickupRequest struct {
	HouseholdID uuid.UUID         `json:"household_id" validate:"required"`
	Type        domain.PickupType `json:"type" validate:"required,oneof=organic plastic paper electronic"`
	SafetyCheck *bool             `json:"safety_check"`
}

func (a *API) createPickup(w http.ResponseWriter, r *http.Request) {
	var req createPickupRequest
	if !a.decode(w, r, &req) {
		return
	}
	if !a.validateRequest(w, req) {
		return
	}
	item, err := a.pickups.Create(r.Context(), req.HouseholdID, req.Type, req.SafetyCheck)
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, "pickup created", item, nil)
}

func (a *API) listPickups(w http.ResponseWriter, r *http.Request) {
	p, ok := a.pagination(w, r)
	if !ok {
		return
	}
	result, err := a.pickups.List(r.Context(), domain.PickupFilter{Pagination: p,
		Status: r.URL.Query().Get("status"), HouseholdID: r.URL.Query().Get("household_id")})
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusOK, "pickups retrieved", result.Items, pageMeta(result))
}

type schedulePickupRequest struct {
	PickupDate time.Time `json:"pickup_date"`
}

func (a *API) schedulePickup(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r)
	if !ok {
		return
	}
	var req schedulePickupRequest
	if !a.decode(w, r, &req) {
		return
	}
	if req.PickupDate.IsZero() {
		response.Failure(w, http.StatusUnprocessableEntity, "validation failed",
			response.Error{Field: "pickup_date", Message: "is required"})
		return
	}
	item, err := a.pickups.Schedule(r.Context(), id, req.PickupDate)
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusOK, "pickup scheduled", item, nil)
}

func (a *API) completePickup(w http.ResponseWriter, r *http.Request) {
	a.changePickup(w, r, a.pickups.Complete, "pickup completed")
}

func (a *API) cancelPickup(w http.ResponseWriter, r *http.Request) {
	a.changePickup(w, r, a.pickups.Cancel, "pickup canceled")
}

func (a *API) changePickup(w http.ResponseWriter, r *http.Request, action func(context.Context, uuid.UUID) (domain.Pickup, error), message string) {
	id, ok := a.pathID(w, r)
	if !ok {
		return
	}
	item, err := action(r.Context(), id)
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusOK, message, item, nil)
}

type createPaymentRequest struct {
	HouseholdID uuid.UUID       `json:"household_id" validate:"required"`
	WasteID     uuid.UUID       `json:"waste_id" validate:"required"`
	Amount      decimal.Decimal `json:"amount"`
}

func (a *API) createPayment(w http.ResponseWriter, r *http.Request) {
	var req createPaymentRequest
	if !a.decode(w, r, &req) {
		return
	}
	if !a.validateRequest(w, req) {
		return
	}
	item, err := a.payments.Create(r.Context(), req.HouseholdID, req.WasteID, req.Amount)
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusCreated, "payment created", item, nil)
}

func (a *API) listPayments(w http.ResponseWriter, r *http.Request) {
	p, ok := a.pagination(w, r)
	if !ok {
		return
	}
	from, err := parseDate(r.URL.Query().Get("date_from"), false)
	if err != nil {
		response.Failure(w, http.StatusBadRequest, "invalid date_from", response.Error{Field: "date_from", Message: err.Error()})
		return
	}
	to, err := parseDate(r.URL.Query().Get("date_to"), true)
	if err != nil {
		response.Failure(w, http.StatusBadRequest, "invalid date_to", response.Error{Field: "date_to", Message: err.Error()})
		return
	}
	result, err := a.payments.List(r.Context(), domain.PaymentFilter{Pagination: p,
		Status: r.URL.Query().Get("status"), HouseholdID: r.URL.Query().Get("household_id"), DateFrom: from, DateTo: to})
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusOK, "payments retrieved", result.Items, pageMeta(result))
}

func (a *API) confirmPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.maxUpload+(64<<10))
	if err := r.ParseMultipartForm(a.maxUpload); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			response.Failure(w, http.StatusRequestEntityTooLarge, "upload too large")
		} else {
			response.Failure(w, http.StatusBadRequest, "invalid multipart form")
		}
		return
	}
	file, _, err := r.FormFile("proof")
	if err != nil {
		response.Failure(w, http.StatusUnprocessableEntity, "proof file is required",
			response.Error{Field: "proof", Message: "multipart field proof is required"})
		return
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, a.maxUpload+1))
	if err != nil {
		a.writeError(w, fmt.Errorf("read proof upload: %w", err))
		return
	}
	if int64(len(payload)) > a.maxUpload {
		response.Failure(w, http.StatusRequestEntityTooLarge, "upload too large")
		return
	}
	if len(payload) == 0 {
		response.Failure(w, http.StatusUnprocessableEntity, "proof file is empty",
			response.Error{Field: "proof", Message: "file must not be empty"})
		return
	}
	contentType := storage.DetectType(payload[:min(len(payload), 512)])
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "application/pdf" {
		response.Failure(w, http.StatusUnprocessableEntity, "unsupported proof content type",
			response.Error{Field: "proof", Message: "only JPG, PNG, or PDF files are allowed"})
		return
	}
	fileURL, err := a.files.Save(bytes.NewReader(payload), contentType)
	if err != nil {
		a.writeError(w, fmt.Errorf("store proof file: %w", err))
		return
	}
	item, err := a.payments.Confirm(r.Context(), id, fileURL)
	if err != nil {
		if removeErr := a.files.Remove(fileURL); removeErr != nil {
			a.logger.Error("failed to remove uncommitted proof", "path", fileURL, "error", removeErr)
		}
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusOK, "payment confirmed", item, nil)
}

func (a *API) wasteSummary(w http.ResponseWriter, r *http.Request) {
	result, err := a.reports.WasteSummary(r.Context())
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusOK, "waste summary retrieved", result, nil)
}

func (a *API) paymentSummary(w http.ResponseWriter, r *http.Request) {
	result, err := a.reports.PaymentSummary(r.Context())
	if err != nil {
		a.writeError(w, err)
		return
	}
	response.Success(w, http.StatusOK, "payment summary retrieved", result, nil)
}

func (a *API) decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		response.Failure(w, http.StatusBadRequest, "invalid JSON body",
			response.Error{Field: "body", Message: "request body must be valid JSON"})
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		response.Failure(w, http.StatusBadRequest, "invalid JSON body",
			response.Error{Field: "body", Message: "request body must contain one JSON value"})
		return false
	}
	return true
}

func (a *API) validateRequest(w http.ResponseWriter, value any) bool {
	if err := a.validate.Struct(value); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			issues := make([]response.Error, 0, len(validationErrors))
			for _, fieldErr := range validationErrors {
				field := strings.ToLower(fieldErr.Field())
				message := "is invalid"
				switch fieldErr.Tag() {
				case "required":
					message = "is required"
				case "max":
					message = "is too long"
				case "oneof":
					message = "must be one of the allowed values"
				}
				issues = append(issues, response.Error{Field: field, Message: message})
			}
			response.Failure(w, http.StatusUnprocessableEntity, "validation failed", issues...)
			return false
		}
		a.writeError(w, fmt.Errorf("validate request: %w", err))
		return false
	}
	return true
}

func (a *API) pagination(w http.ResponseWriter, r *http.Request) (domain.Pagination, bool) {
	page, err := queryInt(r, "page", 1)
	if err != nil || page < 1 {
		response.Failure(w, http.StatusBadRequest, "invalid pagination",
			response.Error{Field: "page", Message: "must be a positive integer"})
		return domain.Pagination{}, false
	}
	limit, err := queryInt(r, "limit", 10)
	if err != nil || limit < 1 || limit > 100 {
		response.Failure(w, http.StatusBadRequest, "invalid pagination",
			response.Error{Field: "limit", Message: "must be an integer between 1 and 100"})
		return domain.Pagination{}, false
	}
	return domain.Pagination{Page: page, Limit: limit}, true
}

func queryInt(r *http.Request, key string, fallback int) (int, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func (a *API) pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Failure(w, http.StatusBadRequest, "invalid id",
			response.Error{Field: "id", Message: "must be a valid UUID"})
		return uuid.Nil, false
	}
	return id, true
}

func parseDate(value string, endOfDay bool) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	var parsed time.Time
	var err error
	if parsed, err = time.Parse(time.RFC3339, value); err != nil {
		parsed, err = time.Parse("2006-01-02", value)
		if err == nil && endOfDay {
			parsed = parsed.Add(24*time.Hour - time.Nanosecond)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("must be RFC3339 or YYYY-MM-DD")
	}
	return &parsed, nil
}

func pageMeta[T any](page domain.Page[T]) map[string]any {
	return map[string]any{"total": page.Total, "page": page.Page, "limit": page.Limit, "total_pages": page.TotalPages}
}

func (a *API) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		response.Failure(w, http.StatusNotFound, "resource not found")
	case errors.Is(err, domain.ErrConflict):
		response.Failure(w, http.StatusConflict, "request conflicts with current resource state")
	case errors.Is(err, domain.ErrInvalid):
		response.Failure(w, http.StatusUnprocessableEntity, "request validation failed")
	case errors.Is(err, domain.ErrForbidden):
		response.Failure(w, http.StatusUnprocessableEntity, "operation is not allowed")
	default:
		a.logger.Error("request failed", "error", err)
		response.Failure(w, http.StatusInternalServerError, "internal server error")
	}
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("http request", "request_id", middleware.GetReqID(r.Context()),
				"method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
		})
	}
}

func panicRecovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("request panic recovered", "request_id", middleware.GetReqID(r.Context()), "panic", recovered)
					response.Failure(w, http.StatusInternalServerError, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
