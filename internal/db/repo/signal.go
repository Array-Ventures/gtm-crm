package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Array-Ventures/gtm-crm/internal/model"
	"github.com/google/uuid"
)

// SignalRepo handles signal database operations.
type SignalRepo struct {
	db *sql.DB
}

// NewSignalRepo creates a new SignalRepo.
func NewSignalRepo(db *sql.DB) *SignalRepo {
	return &SignalRepo{db: db}
}

func scanSignal(row interface{ Scan(...any) error }) (*model.Signal, error) {
	var s model.Signal
	err := row.Scan(
		&s.ID, &s.UUID, &s.SignalType, &s.Description, &s.SourceURL,
		&s.PersonID, &s.OrgID, &s.DetectedAt,
		&s.Archived, &s.CreatedAt, &s.UpdatedAt,
	)
	return &s, err
}

// Create inserts a new signal.
func (r *SignalRepo) Create(ctx context.Context, input model.CreateSignalInput) (*model.Signal, error) {
	if strings.TrimSpace(input.SignalType) == "" {
		return nil, fmt.Errorf("signal_type is required: %w", model.ErrValidation)
	}

	id := uuid.New().String()
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO signals (uuid, signal_type, description, source_url, person_id, org_id, detected_at)
		 VALUES (?, ?, ?, ?, ?, ?, COALESCE(?, datetime('now')))`,
		id, input.SignalType, input.Description, input.SourceURL, input.PersonID, input.OrgID, input.DetectedAt)
	if err != nil {
		return nil, fmt.Errorf("create signal: %w", err)
	}

	signalID, _ := result.LastInsertId()
	return r.FindByID(ctx, signalID)
}

// FindByID returns a signal by ID.
func (r *SignalRepo) FindByID(ctx context.Context, id int64) (*model.Signal, error) {
	s, err := scanSignal(r.db.QueryRowContext(ctx,
		`SELECT id, uuid, signal_type, description, source_url, person_id, org_id, detected_at,
		        archived, created_at, updated_at
		 FROM signals WHERE id = ? AND archived = 0`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("signal %d: %w", id, model.ErrNotFound)
		}
		return nil, fmt.Errorf("find signal %d: %w", id, err)
	}
	return s, nil
}

// FindAll returns signals with optional filters.
func (r *SignalRepo) FindAll(ctx context.Context, filters model.SignalFilters) ([]*model.Signal, error) {
	query := `SELECT id, uuid, signal_type, description, source_url, person_id, org_id, detected_at,
	                 archived, created_at, updated_at
	          FROM signals WHERE archived = 0`
	var args []any

	if filters.SignalType != nil {
		query += " AND signal_type = ?"
		args = append(args, *filters.SignalType)
	}
	if filters.PersonID != nil {
		query += " AND person_id = ?"
		args = append(args, *filters.PersonID)
	}
	if filters.OrgID != nil {
		query += " AND org_id = ?"
		args = append(args, *filters.OrgID)
	}

	query += " ORDER BY detected_at DESC"

	if filters.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filters.Limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list signals: %w", err)
	}
	defer rows.Close()

	var signals []*model.Signal
	for rows.Next() {
		s, err := scanSignal(rows)
		if err != nil {
			return nil, fmt.Errorf("scan signal: %w", err)
		}
		signals = append(signals, s)
	}
	return signals, rows.Err()
}

// Update modifies a signal.
func (r *SignalRepo) Update(ctx context.Context, id int64, input model.UpdateSignalInput) (*model.Signal, error) {
	var setClauses []string
	var args []any

	if input.SignalType != nil {
		if strings.TrimSpace(*input.SignalType) == "" {
			return nil, fmt.Errorf("signal_type cannot be empty: %w", model.ErrValidation)
		}
		setClauses = append(setClauses, "signal_type = ?")
		args = append(args, *input.SignalType)
	}
	if input.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *input.Description)
	}
	if input.PersonID != nil {
		setClauses = append(setClauses, "person_id = ?")
		args = append(args, *input.PersonID)
	}
	if input.OrgID != nil {
		setClauses = append(setClauses, "org_id = ?")
		args = append(args, *input.OrgID)
	}
	if input.DetectedAt != nil {
		setClauses = append(setClauses, "detected_at = ?")
		args = append(args, *input.DetectedAt)
	}

	if len(setClauses) == 0 {
		return r.FindByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = datetime('now')")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE signals SET %s WHERE id = ? AND archived = 0", strings.Join(setClauses, ", "))
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update signal %d: %w", id, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, fmt.Errorf("signal %d: %w", id, model.ErrNotFound)
	}

	return r.FindByID(ctx, id)
}

// Archive soft-deletes a signal.
func (r *SignalRepo) Archive(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE signals SET archived = 1, updated_at = datetime('now') WHERE id = ? AND archived = 0", id)
	if err != nil {
		return fmt.Errorf("archive signal %d: %w", id, err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("signal %d: %w", id, model.ErrNotFound)
	}
	return nil
}
