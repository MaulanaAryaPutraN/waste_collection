package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PickupType string

const (
	Organic    PickupType = "organic"
	Plastic    PickupType = "plastic"
	Paper      PickupType = "paper"
	Electronic PickupType = "electronic"
)

type PickupStatus string

const (
	PickupPending   PickupStatus = "pending"
	PickupScheduled PickupStatus = "scheduled"
	PickupCompleted PickupStatus = "completed"
	PickupCanceled  PickupStatus = "canceled"
)

type PaymentStatus string

const (
	PaymentPending PaymentStatus = "pending"
	PaymentPaid    PaymentStatus = "paid"
	PaymentFailed  PaymentStatus = "failed"
)

type Household struct {
	ID        uuid.UUID `json:"id"`
	OwnerName string    `json:"owner_name"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Pickup struct {
	ID          uuid.UUID    `json:"id"`
	HouseholdID uuid.UUID    `json:"household_id"`
	Type        PickupType   `json:"type"`
	Status      PickupStatus `json:"status"`
	PickupDate  *time.Time   `json:"pickup_date"`
	SafetyCheck *bool        `json:"safety_check"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Payment struct {
	ID           uuid.UUID       `json:"id"`
	HouseholdID  uuid.UUID       `json:"household_id"`
	WasteID      uuid.UUID       `json:"waste_id"`
	Amount       decimal.Decimal `json:"amount"`
	PaymentDate  *time.Time      `json:"payment_date"`
	Status       PaymentStatus   `json:"status"`
	ProofFileURL *string         `json:"proof_file_url"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type Page[T any] struct {
	Items      []T   `json:"items"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalPages int64 `json:"total_pages"`
}

type Pagination struct {
	Page  int
	Limit int
}

type PickupFilter struct {
	Pagination
	Status      string
	HouseholdID string
}

type PaymentFilter struct {
	Pagination
	Status      string
	HouseholdID string
	DateFrom    *time.Time
	DateTo      *time.Time
}

type WasteSummary struct {
	Type   PickupType   `json:"type"`
	Status PickupStatus `json:"status"`
	Count  int64        `json:"count"`
}

type PaymentSummary struct {
	Status PaymentStatus   `json:"status"`
	Count  int64           `json:"count"`
	Total  decimal.Decimal `json:"total"`
}

type PaymentReport struct {
	Payments     []PaymentSummary `json:"payments"`
	TotalRevenue decimal.Decimal  `json:"total_revenue"`
}
