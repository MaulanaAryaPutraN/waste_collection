package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"example.com/pengumpulan_sampah/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

func (r *Postgres) CreateHousehold(ctx context.Context, name, address string) (domain.Household, error) {
	return scanHousehold(r.pool.QueryRow(ctx, `
		INSERT INTO households (owner_name, address) VALUES ($1, $2)
		RETURNING id, owner_name, address, created_at, updated_at`, name, address))
}

func (r *Postgres) ListHouseholds(ctx context.Context, p domain.Pagination) (domain.Page[domain.Household], error) {
	var result domain.Page[domain.Household]
	result.Page, result.Limit = p.Page, p.Limit
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM households`).Scan(&result.Total); err != nil {
		return result, fmt.Errorf("count households: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT id, owner_name, address, created_at, updated_at
		FROM households ORDER BY created_at DESC LIMIT $1 OFFSET $2`, p.Limit, (p.Page-1)*p.Limit)
	if err != nil {
		return result, fmt.Errorf("list households: %w", err)
	}
	defer rows.Close()
	result.Items = make([]domain.Household, 0)
	for rows.Next() {
		item, scanErr := scanHousehold(rows)
		if scanErr != nil {
			return result, fmt.Errorf("scan household: %w", scanErr)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate households: %w", err)
	}
	result.TotalPages = int64(math.Ceil(float64(result.Total) / float64(p.Limit)))
	return result, nil
}

func (r *Postgres) GetHousehold(ctx context.Context, id uuid.UUID) (domain.Household, error) {
	item, err := scanHousehold(r.pool.QueryRow(ctx, `SELECT id, owner_name, address, created_at, updated_at
		FROM households WHERE id = $1`, id))
	return item, mapNotFound(err)
}

func (r *Postgres) DeleteHousehold(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM households WHERE id = $1`, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: household has associated pickups or payments", domain.ErrConflict)
		}
		return fmt.Errorf("delete household: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Postgres) CreatePickup(ctx context.Context, householdID uuid.UUID, kind domain.PickupType, safety *bool) (domain.Pickup, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("begin pickup transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var household uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM households WHERE id = $1 FOR UPDATE`, householdID).Scan(&household)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Pickup{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("lock household: %w", err)
	}
	var hasPending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM payments WHERE household_id = $1 AND status = 'pending'
	)`, householdID).Scan(&hasPending); err != nil {
		return domain.Pickup{}, fmt.Errorf("check pending payments: %w", err)
	}
	if hasPending {
		return domain.Pickup{}, fmt.Errorf("%w: household has a pending payment", domain.ErrConflict)
	}
	item, err := scanPickup(tx.QueryRow(ctx, `INSERT INTO waste_pickups (household_id, type, safety_check)
		VALUES ($1, $2, $3) RETURNING id, household_id, type, status, pickup_date, safety_check, created_at, updated_at`,
		householdID, kind, safety))
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("create pickup: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Pickup{}, fmt.Errorf("commit pickup: %w", err)
	}
	return item, nil
}

func (r *Postgres) ListPickups(ctx context.Context, f domain.PickupFilter) (domain.Page[domain.Pickup], error) {
	var result domain.Page[domain.Pickup]
	result.Page, result.Limit = f.Page, f.Limit
	var householdID any
	if f.HouseholdID != "" {
		parsedID, err := uuid.Parse(f.HouseholdID)
		if err != nil {
			return result, fmt.Errorf("%w: invalid household_id filter", domain.ErrInvalid)
		}
		householdID = parsedID
	}
	const filter = ` WHERE ($1::text = '' OR status = $1)
		AND ($2::uuid IS NULL OR household_id = $2)`
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM waste_pickups`+filter, f.Status, householdID).Scan(&result.Total); err != nil {
		return result, fmt.Errorf("count pickups: %w", err)
	}
	query := `SELECT id, household_id, type, status, pickup_date, safety_check, created_at, updated_at
		FROM waste_pickups` + filter + ` ORDER BY created_at DESC LIMIT $3 OFFSET $4`
	rows, err := r.pool.Query(ctx, query, f.Status, householdID, f.Limit, (f.Page-1)*f.Limit)
	if err != nil {
		return result, fmt.Errorf("list pickups: %w", err)
	}
	defer rows.Close()
	result.Items = make([]domain.Pickup, 0)
	for rows.Next() {
		item, scanErr := scanPickup(rows)
		if scanErr != nil {
			return result, fmt.Errorf("scan pickup: %w", scanErr)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate pickups: %w", err)
	}
	result.TotalPages = int64(math.Ceil(float64(result.Total) / float64(f.Limit)))
	return result, nil
}

func (r *Postgres) SchedulePickup(ctx context.Context, id uuid.UUID, when time.Time) (domain.Pickup, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("begin schedule transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var status domain.PickupStatus
	var kind domain.PickupType
	var safety *bool
	err = tx.QueryRow(ctx, `SELECT status, type, safety_check FROM waste_pickups WHERE id = $1 FOR UPDATE`, id).Scan(&status, &kind, &safety)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Pickup{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("load pickup: %w", err)
	}
	if status != domain.PickupPending {
		return domain.Pickup{}, fmt.Errorf("%w: only pending pickups can be scheduled", domain.ErrConflict)
	}
	if kind == domain.Electronic && (safety == nil || !*safety) {
		return domain.Pickup{}, fmt.Errorf("%w: electronic pickup requires safety_check=true", domain.ErrInvalid)
	}
	item, err := scanPickup(tx.QueryRow(ctx, `UPDATE waste_pickups SET status = 'scheduled', pickup_date = $2, updated_at = now()
		WHERE id = $1 RETURNING id, household_id, type, status, pickup_date, safety_check, created_at, updated_at`, id, when))
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("schedule pickup: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Pickup{}, fmt.Errorf("commit schedule: %w", err)
	}
	return item, nil
}

func (r *Postgres) CompletePickup(ctx context.Context, id uuid.UUID) (domain.Pickup, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("begin completion transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var householdID uuid.UUID
	var kind domain.PickupType
	var status domain.PickupStatus
	err = tx.QueryRow(ctx, `SELECT household_id FROM waste_pickups WHERE id = $1`, id).Scan(&householdID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Pickup{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("load pickup household: %w", err)
	}
	var lockedHousehold uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM households WHERE id = $1 FOR UPDATE`, householdID).Scan(&lockedHousehold)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Pickup{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("lock household for completion: %w", err)
	}
	err = tx.QueryRow(ctx, `SELECT household_id, type, status FROM waste_pickups WHERE id = $1 FOR UPDATE`, id).
		Scan(&householdID, &kind, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Pickup{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("lock pickup: %w", err)
	}
	if status != domain.PickupScheduled {
		return domain.Pickup{}, fmt.Errorf("%w: only scheduled pickups can be completed", domain.ErrConflict)
	}
	amount := decimal.NewFromInt(50000)
	if kind == domain.Electronic {
		amount = decimal.NewFromInt(100000)
	}
	item, err := scanPickup(tx.QueryRow(ctx, `UPDATE waste_pickups SET status = 'completed', updated_at = now()
		WHERE id = $1 RETURNING id, household_id, type, status, pickup_date, safety_check, created_at, updated_at`, id))
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("complete pickup: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO payments (household_id, waste_id, amount, status) VALUES ($1, $2, $3, 'pending')`,
		householdID, id, amount)
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("create completion payment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Pickup{}, fmt.Errorf("commit completion: %w", err)
	}
	return item, nil
}

func (r *Postgres) CancelPickup(ctx context.Context, id uuid.UUID) (domain.Pickup, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("begin cancel transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var status domain.PickupStatus
	err = tx.QueryRow(ctx, `SELECT status FROM waste_pickups WHERE id = $1 FOR UPDATE`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Pickup{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("load pickup for cancel: %w", err)
	}
	if status != domain.PickupPending && status != domain.PickupScheduled {
		return domain.Pickup{}, fmt.Errorf("%w: only pending or scheduled pickups can be canceled", domain.ErrConflict)
	}
	item, err := scanPickup(tx.QueryRow(ctx, `UPDATE waste_pickups SET status = 'canceled', updated_at = now()
		WHERE id = $1 RETURNING id, household_id, type, status, pickup_date, safety_check, created_at, updated_at`, id))
	if err != nil {
		return domain.Pickup{}, fmt.Errorf("cancel pickup: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Pickup{}, fmt.Errorf("commit cancel: %w", err)
	}
	return item, nil
}

func (r *Postgres) CreateManualPayment(ctx context.Context, householdID, wasteID uuid.UUID, amount decimal.Decimal) (domain.Payment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Payment{}, fmt.Errorf("begin payment transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var household uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM households WHERE id = $1 FOR UPDATE`, householdID).Scan(&household)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Payment{}, fmt.Errorf("lock household for payment: %w", err)
	}
	var pickupHousehold uuid.UUID
	err = tx.QueryRow(ctx, `SELECT household_id FROM waste_pickups WHERE id = $1 FOR UPDATE`, wasteID).Scan(&pickupHousehold)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Payment{}, fmt.Errorf("load pickup for payment: %w", err)
	}
	if pickupHousehold != householdID {
		return domain.Payment{}, fmt.Errorf("%w: pickup does not belong to household", domain.ErrInvalid)
	}
	item, err := scanPayment(tx.QueryRow(ctx, `INSERT INTO payments (household_id, waste_id, amount)
		VALUES ($1, $2, $3) RETURNING id, household_id, waste_id, amount, payment_date, status, proof_file_url, created_at, updated_at`,
		householdID, wasteID, amount))
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Payment{}, fmt.Errorf("%w: pickup already has a payment", domain.ErrConflict)
		}
		return domain.Payment{}, fmt.Errorf("create manual payment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Payment{}, fmt.Errorf("commit manual payment: %w", err)
	}
	return item, nil
}

func (r *Postgres) GetPayment(ctx context.Context, id uuid.UUID) (domain.Payment, error) {
	item, err := scanPayment(r.pool.QueryRow(ctx, `SELECT id, household_id, waste_id, amount, payment_date, status, proof_file_url, created_at, updated_at
		FROM payments WHERE id = $1`, id))
	return item, mapNotFound(err)
}

func (r *Postgres) ListPayments(ctx context.Context, f domain.PaymentFilter) (domain.Page[domain.Payment], error) {
	var result domain.Page[domain.Payment]
	result.Page, result.Limit = f.Page, f.Limit
	var householdID any
	if f.HouseholdID != "" {
		parsedID, err := uuid.Parse(f.HouseholdID)
		if err != nil {
			return result, fmt.Errorf("%w: invalid household_id filter", domain.ErrInvalid)
		}
		householdID = parsedID
	}
	const filter = ` WHERE ($1::text = '' OR status = $1)
		AND ($2::uuid IS NULL OR household_id = $2)
		AND ($3::timestamptz IS NULL OR payment_date >= $3)
		AND ($4::timestamptz IS NULL OR payment_date <= $4)`
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM payments`+filter,
		f.Status, householdID, f.DateFrom, f.DateTo).Scan(&result.Total); err != nil {
		return result, fmt.Errorf("count payments: %w", err)
	}
	query := `SELECT id, household_id, waste_id, amount, payment_date, status, proof_file_url, created_at, updated_at
		FROM payments` + filter + ` ORDER BY created_at DESC LIMIT $5 OFFSET $6`
	rows, err := r.pool.Query(ctx, query, f.Status, householdID, f.DateFrom, f.DateTo, f.Limit, (f.Page-1)*f.Limit)
	if err != nil {
		return result, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()
	result.Items = make([]domain.Payment, 0)
	for rows.Next() {
		item, scanErr := scanPayment(rows)
		if scanErr != nil {
			return result, fmt.Errorf("scan payment: %w", scanErr)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate payments: %w", err)
	}
	result.TotalPages = int64(math.Ceil(float64(result.Total) / float64(f.Limit)))
	return result, nil
}

func (r *Postgres) ConfirmPayment(ctx context.Context, id uuid.UUID, proofURL string, paidAt time.Time) (domain.Payment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Payment{}, fmt.Errorf("begin confirmation transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var status domain.PaymentStatus
	err = tx.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1 FOR UPDATE`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Payment{}, fmt.Errorf("lock payment: %w", err)
	}
	if status != domain.PaymentPending {
		return domain.Payment{}, fmt.Errorf("%w: only pending payments can be confirmed", domain.ErrConflict)
	}
	item, err := scanPayment(tx.QueryRow(ctx, `UPDATE payments SET status = 'paid', payment_date = $2,
		proof_file_url = $3, updated_at = now() WHERE id = $1
		RETURNING id, household_id, waste_id, amount, payment_date, status, proof_file_url, created_at, updated_at`,
		id, paidAt, proofURL))
	if err != nil {
		return domain.Payment{}, fmt.Errorf("confirm payment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Payment{}, fmt.Errorf("commit payment confirmation: %w", err)
	}
	return item, nil
}

func (r *Postgres) WasteSummary(ctx context.Context) ([]domain.WasteSummary, error) {
	rows, err := r.pool.Query(ctx, `SELECT type, status, count(*) FROM waste_pickups GROUP BY type, status ORDER BY type, status`)
	if err != nil {
		return nil, fmt.Errorf("query waste summary: %w", err)
	}
	defer rows.Close()
	result := make([]domain.WasteSummary, 0)
	for rows.Next() {
		var item domain.WasteSummary
		if err := rows.Scan(&item.Type, &item.Status, &item.Count); err != nil {
			return nil, fmt.Errorf("scan waste summary: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Postgres) PaymentSummary(ctx context.Context) (domain.PaymentReport, error) {
	rows, err := r.pool.Query(ctx, `SELECT status, count(*), COALESCE(sum(amount), 0) FROM payments GROUP BY status ORDER BY status`)
	if err != nil {
		return domain.PaymentReport{}, fmt.Errorf("query payment summary: %w", err)
	}
	defer rows.Close()
	report := domain.PaymentReport{Payments: make([]domain.PaymentSummary, 0)}
	for rows.Next() {
		var item domain.PaymentSummary
		if err := rows.Scan(&item.Status, &item.Count, &item.Total); err != nil {
			return report, fmt.Errorf("scan payment summary: %w", err)
		}
		report.Payments = append(report.Payments, item)
	}
	if err := rows.Err(); err != nil {
		return report, fmt.Errorf("iterate payment summary: %w", err)
	}
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(sum(amount), 0) FROM payments WHERE status = 'paid'`).Scan(&report.TotalRevenue); err != nil {
		return report, fmt.Errorf("query payment revenue: %w", err)
	}
	return report, nil
}

type scanner interface{ Scan(...any) error }

func scanHousehold(row scanner) (domain.Household, error) {
	var item domain.Household
	err := row.Scan(&item.ID, &item.OwnerName, &item.Address, &item.CreatedAt, &item.UpdatedAt)
	return item, mapNotFound(err)
}

func scanPickup(row scanner) (domain.Pickup, error) {
	var item domain.Pickup
	err := row.Scan(&item.ID, &item.HouseholdID, &item.Type, &item.Status, &item.PickupDate,
		&item.SafetyCheck, &item.CreatedAt, &item.UpdatedAt)
	return item, mapNotFound(err)
}

func scanPayment(row scanner) (domain.Payment, error) {
	var item domain.Payment
	err := row.Scan(&item.ID, &item.HouseholdID, &item.WasteID, &item.Amount, &item.PaymentDate,
		&item.Status, &item.ProofFileURL, &item.CreatedAt, &item.UpdatedAt)
	return item, mapNotFound(err)
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
