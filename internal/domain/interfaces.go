package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type HouseholdRepository interface {
	CreateHousehold(context.Context, string, string) (Household, error)
	ListHouseholds(context.Context, Pagination) (Page[Household], error)
	GetHousehold(context.Context, uuid.UUID) (Household, error)
	DeleteHousehold(context.Context, uuid.UUID) error
}

type PickupRepository interface {
	CreatePickup(context.Context, uuid.UUID, PickupType, *bool) (Pickup, error)
	ListPickups(context.Context, PickupFilter) (Page[Pickup], error)
	SchedulePickup(context.Context, uuid.UUID, time.Time) (Pickup, error)
	CompletePickup(context.Context, uuid.UUID) (Pickup, error)
	CancelPickup(context.Context, uuid.UUID) (Pickup, error)
}

type PaymentRepository interface {
	CreateManualPayment(context.Context, uuid.UUID, uuid.UUID, decimal.Decimal) (Payment, error)
	GetPayment(context.Context, uuid.UUID) (Payment, error)
	ListPayments(context.Context, PaymentFilter) (Page[Payment], error)
	ConfirmPayment(context.Context, uuid.UUID, string, time.Time) (Payment, error)
}

type ReportRepository interface {
	WasteSummary(context.Context) ([]WasteSummary, error)
	PaymentSummary(context.Context) (PaymentReport, error)
}

type HouseholdService interface {
	Create(context.Context, string, string) (Household, error)
	List(context.Context, Pagination) (Page[Household], error)
	Get(context.Context, uuid.UUID) (Household, error)
	Delete(context.Context, uuid.UUID) error
}

type PickupService interface {
	Create(context.Context, uuid.UUID, PickupType, *bool) (Pickup, error)
	List(context.Context, PickupFilter) (Page[Pickup], error)
	Schedule(context.Context, uuid.UUID, time.Time) (Pickup, error)
	Complete(context.Context, uuid.UUID) (Pickup, error)
	Cancel(context.Context, uuid.UUID) (Pickup, error)
}

type PaymentService interface {
	Create(context.Context, uuid.UUID, uuid.UUID, decimal.Decimal) (Payment, error)
	List(context.Context, PaymentFilter) (Page[Payment], error)
	Confirm(context.Context, uuid.UUID, string) (Payment, error)
}

type ReportService interface {
	WasteSummary(context.Context) ([]WasteSummary, error)
	PaymentSummary(context.Context) (PaymentReport, error)
}
