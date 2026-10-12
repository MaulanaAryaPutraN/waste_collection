package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/pengumpulan_sampah/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type pickupRepoMock struct {
	createErr   error
	scheduleErr error
	completeErr error
	cancelErr   error
	createCalls int
}

func (m *pickupRepoMock) CreatePickup(context.Context, uuid.UUID, domain.PickupType, *bool) (domain.Pickup, error) {
	m.createCalls++
	return domain.Pickup{}, m.createErr
}
func (*pickupRepoMock) ListPickups(context.Context, domain.PickupFilter) (domain.Page[domain.Pickup], error) {
	return domain.Page[domain.Pickup]{}, nil
}
func (m *pickupRepoMock) SchedulePickup(context.Context, uuid.UUID, time.Time) (domain.Pickup, error) {
	return domain.Pickup{}, m.scheduleErr
}
func (m *pickupRepoMock) CompletePickup(context.Context, uuid.UUID) (domain.Pickup, error) {
	return domain.Pickup{}, m.completeErr
}
func (m *pickupRepoMock) CancelPickup(context.Context, uuid.UUID) (domain.Pickup, error) {
	return domain.Pickup{}, m.cancelErr
}

type paymentRepoMock struct {
	createErr  error
	confirmErr error
}

func (m *paymentRepoMock) CreateManualPayment(context.Context, uuid.UUID, uuid.UUID, decimal.Decimal) (domain.Payment, error) {
	return domain.Payment{}, m.createErr
}
func (*paymentRepoMock) GetPayment(context.Context, uuid.UUID) (domain.Payment, error) {
	return domain.Payment{}, domain.ErrNotFound
}
func (*paymentRepoMock) ListPayments(context.Context, domain.PaymentFilter) (domain.Page[domain.Payment], error) {
	return domain.Page[domain.Payment]{}, nil
}
func (m *paymentRepoMock) ConfirmPayment(context.Context, uuid.UUID, string, time.Time) (domain.Payment, error) {
	return domain.Payment{}, m.confirmErr
}

func TestPickupBusinessRules(t *testing.T) {
	future := time.Now().Add(time.Hour)
	noSafety := (*bool)(nil)
	tests := []struct {
		name      string
		run       func(*pickupRepoMock) error
		want      error
		wantCalls int
	}{
		{
			name: "pending household payment prevents creating pickup",
			run: func(repo *pickupRepoMock) error {
				repo.createErr = domain.ErrConflict
				_, err := NewPickup(repo).Create(context.Background(), uuid.New(), domain.Organic, nil)
				return err
			},
			want: domain.ErrConflict, wantCalls: 1,
		},
		{
			name: "electronic pickup requires safety check",
			run: func(repo *pickupRepoMock) error {
				_, err := NewPickup(repo).Create(context.Background(), uuid.New(), domain.Electronic, noSafety)
				return err
			},
			want: domain.ErrInvalid,
		},
		{
			name: "pickup cannot be scheduled in the past",
			run: func(repo *pickupRepoMock) error {
				_, err := NewPickup(repo).Schedule(context.Background(), uuid.New(), time.Now().Add(-time.Second))
				return err
			},
			want: domain.ErrInvalid,
		},
		{
			name: "only pending pickup can be scheduled",
			run: func(repo *pickupRepoMock) error {
				repo.scheduleErr = domain.ErrConflict
				_, err := NewPickup(repo).Schedule(context.Background(), uuid.New(), future)
				return err
			},
			want: domain.ErrConflict,
		},
		{
			name: "electronic pickup must pass safety check before scheduling",
			run: func(repo *pickupRepoMock) error {
				repo.scheduleErr = domain.ErrInvalid
				_, err := NewPickup(repo).Schedule(context.Background(), uuid.New(), future)
				return err
			},
			want: domain.ErrInvalid,
		},
		{
			name: "only scheduled pickup can complete and completion reports conflict",
			run: func(repo *pickupRepoMock) error {
				repo.completeErr = domain.ErrConflict
				_, err := NewPickup(repo).Complete(context.Background(), uuid.New())
				return err
			},
			want: domain.ErrConflict,
		},
		{
			name: "only pending or scheduled pickup can be canceled",
			run: func(repo *pickupRepoMock) error {
				repo.cancelErr = domain.ErrConflict
				_, err := NewPickup(repo).Cancel(context.Background(), uuid.New())
				return err
			},
			want: domain.ErrConflict,
		},
		{
			name: "unsupported pickup type rejected before repository",
			run: func(repo *pickupRepoMock) error {
				_, err := NewPickup(repo).Create(context.Background(), uuid.New(), "glass", nil)
				return err
			},
			want: domain.ErrInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &pickupRepoMock{}
			err := test.run(repo)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repo.createCalls != test.wantCalls {
				t.Fatalf("create calls = %d, want %d", repo.createCalls, test.wantCalls)
			}
		})
	}
}

func TestPaymentBusinessRules(t *testing.T) {
	tests := []struct {
		name string
		run  func(*paymentRepoMock) error
		want error
	}{
		{
			name: "manual payment requires positive amount",
			run: func(repo *paymentRepoMock) error {
				_, err := NewPayment(repo).Create(context.Background(), uuid.New(), uuid.New(), decimal.Zero)
				return err
			},
			want: domain.ErrInvalid,
		},
		{
			name: "manual payment rejects precision outside NUMERIC scale",
			run: func(repo *paymentRepoMock) error {
				_, err := NewPayment(repo).Create(context.Background(), uuid.New(), uuid.New(), decimal.RequireFromString("10.001"))
				return err
			},
			want: domain.ErrInvalid,
		},
		{
			name: "manual payment validates pickup ownership",
			run: func(repo *paymentRepoMock) error {
				repo.createErr = domain.ErrInvalid
				_, err := NewPayment(repo).Create(context.Background(), uuid.New(), uuid.New(), decimal.NewFromInt(500))
				return err
			},
			want: domain.ErrInvalid,
		},
		{
			name: "only pending payment can be confirmed",
			run: func(repo *paymentRepoMock) error {
				repo.confirmErr = domain.ErrConflict
				_, err := NewPayment(repo).Confirm(context.Background(), uuid.New(), "/uploads/proof.pdf")
				return err
			},
			want: domain.ErrConflict,
		},
		{
			name: "payment confirmation requires proof",
			run: func(repo *paymentRepoMock) error {
				_, err := NewPayment(repo).Confirm(context.Background(), uuid.New(), "")
				return err
			},
			want: domain.ErrInvalid,
		},
		{
			name: "valid manual payment amount is passed through",
			run: func(repo *paymentRepoMock) error {
				_, err := NewPayment(repo).Create(context.Background(), uuid.New(), uuid.New(), decimal.RequireFromString("9999999999.99"))
				return err
			},
			want: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run(&paymentRepoMock{})
			if !errors.Is(err, test.want) {
				if err == nil || test.want == nil {
					t.Fatalf("error = %v, want %v", err, test.want)
				}
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
