package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"example.com/pengumpulan_sampah/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Household struct{ repo domain.HouseholdRepository }

func NewHousehold(repo domain.HouseholdRepository) *Household { return &Household{repo: repo} }
func (s *Household) Create(ctx context.Context, name, address string) (domain.Household, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(address) == "" {
		return domain.Household{}, domain.ErrInvalid
	}
	return s.repo.CreateHousehold(ctx, strings.TrimSpace(name), strings.TrimSpace(address))
}
func (s *Household) List(ctx context.Context, p domain.Pagination) (domain.Page[domain.Household], error) {
	return s.repo.ListHouseholds(ctx, normalizePagination(p))
}
func (s *Household) Get(ctx context.Context, id uuid.UUID) (domain.Household, error) {
	return s.repo.GetHousehold(ctx, id)
}
func (s *Household) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteHousehold(ctx, id)
}

type Pickup struct{ repo domain.PickupRepository }

func NewPickup(repo domain.PickupRepository) *Pickup { return &Pickup{repo: repo} }
func (s *Pickup) Create(ctx context.Context, householdID uuid.UUID, kind domain.PickupType, safety *bool) (domain.Pickup, error) {
	switch kind {
	case domain.Organic, domain.Plastic, domain.Paper:
	case domain.Electronic:
		if safety == nil {
			return domain.Pickup{}, fmt.Errorf("%w: safety_check is required for electronic pickup", domain.ErrInvalid)
		}
	default:
		return domain.Pickup{}, fmt.Errorf("%w: unsupported pickup type", domain.ErrInvalid)
	}
	return s.repo.CreatePickup(ctx, householdID, kind, safety)
}
func (s *Pickup) List(ctx context.Context, filter domain.PickupFilter) (domain.Page[domain.Pickup], error) {
	if filter.Status != "" && filter.Status != string(domain.PickupPending) && filter.Status != string(domain.PickupScheduled) && filter.Status != string(domain.PickupCompleted) && filter.Status != string(domain.PickupCanceled) {
		return domain.Page[domain.Pickup]{}, domain.ErrInvalid
	}
	if filter.HouseholdID != "" {
		if _, err := uuid.Parse(filter.HouseholdID); err != nil {
			return domain.Page[domain.Pickup]{}, domain.ErrInvalid
		}
	}
	filter.Pagination = normalizePagination(filter.Pagination)
	return s.repo.ListPickups(ctx, filter)
}
func (s *Pickup) Schedule(ctx context.Context, id uuid.UUID, when time.Time) (domain.Pickup, error) {
	if !when.After(time.Now()) {
		return domain.Pickup{}, fmt.Errorf("%w: pickup_date must be in the future", domain.ErrInvalid)
	}
	return s.repo.SchedulePickup(ctx, id, when)
}
func (s *Pickup) Complete(ctx context.Context, id uuid.UUID) (domain.Pickup, error) {
	return s.repo.CompletePickup(ctx, id)
}
func (s *Pickup) Cancel(ctx context.Context, id uuid.UUID) (domain.Pickup, error) {
	return s.repo.CancelPickup(ctx, id)
}

type Payment struct {
	repo domain.PaymentRepository
	now  func() time.Time
}

func NewPayment(repo domain.PaymentRepository) *Payment { return &Payment{repo: repo, now: time.Now} }
func (s *Payment) Create(ctx context.Context, householdID, wasteID uuid.UUID, amount decimal.Decimal) (domain.Payment, error) {
	if !amount.GreaterThan(decimal.Zero) {
		return domain.Payment{}, fmt.Errorf("%w: amount must be greater than zero", domain.ErrInvalid)
	}
	if amount.Exponent() < -2 || amount.GreaterThan(decimal.New(999999999999, -2)) {
		return domain.Payment{}, fmt.Errorf("%w: amount exceeds NUMERIC(12,2)", domain.ErrInvalid)
	}
	return s.repo.CreateManualPayment(ctx, householdID, wasteID, amount)
}
func (s *Payment) List(ctx context.Context, filter domain.PaymentFilter) (domain.Page[domain.Payment], error) {
	if filter.Status != "" && filter.Status != string(domain.PaymentPending) && filter.Status != string(domain.PaymentPaid) && filter.Status != string(domain.PaymentFailed) {
		return domain.Page[domain.Payment]{}, domain.ErrInvalid
	}
	if filter.HouseholdID != "" {
		if _, err := uuid.Parse(filter.HouseholdID); err != nil {
			return domain.Page[domain.Payment]{}, domain.ErrInvalid
		}
	}
	if filter.DateFrom != nil && filter.DateTo != nil && filter.DateFrom.After(*filter.DateTo) {
		return domain.Page[domain.Payment]{}, fmt.Errorf("%w: date_from must not be after date_to", domain.ErrInvalid)
	}
	filter.Pagination = normalizePagination(filter.Pagination)
	return s.repo.ListPayments(ctx, filter)
}
func (s *Payment) Confirm(ctx context.Context, id uuid.UUID, fileURL string) (domain.Payment, error) {
	if strings.TrimSpace(fileURL) == "" {
		return domain.Payment{}, fmt.Errorf("%w: proof file is required", domain.ErrInvalid)
	}
	return s.repo.ConfirmPayment(ctx, id, fileURL, s.now())
}

type Report struct{ repo domain.ReportRepository }

func NewReport(repo domain.ReportRepository) *Report { return &Report{repo: repo} }
func (s *Report) WasteSummary(ctx context.Context) ([]domain.WasteSummary, error) {
	return s.repo.WasteSummary(ctx)
}
func (s *Report) PaymentSummary(ctx context.Context) (domain.PaymentReport, error) {
	return s.repo.PaymentSummary(ctx)
}

func normalizePagination(p domain.Pagination) domain.Pagination {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Limit < 1 {
		p.Limit = 10
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	return p
}
