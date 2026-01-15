package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ObservationPoint represents a user-defined observation location
type ObservationPoint struct {
	ID              int       `json:"id"`
	UserID          int       `json:"userId"`
	Name            string    `json:"name"`
	Latitude        float64   `json:"latitude"`
	Longitude       float64   `json:"longitude"`
	ElevationMeters float64   `json:"elevationMeters"`
	IsActive        bool      `json:"isActive"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// ObservationPointRepository provides methods for managing observation points
type ObservationPointRepository struct {
	db *DB
}

// NewObservationPointRepository creates a new observation point repository
func NewObservationPointRepository(db *DB) *ObservationPointRepository {
	return &ObservationPointRepository{db: db}
}

// GetUserPoints returns all observation points for a user
func (r *ObservationPointRepository) GetUserPoints(ctx context.Context, userID int) ([]ObservationPoint, error) {
	query := `
		SELECT id, user_id, name, latitude, longitude, elevation_meters, is_active, created_at, updated_at
		FROM observation_points
		WHERE user_id = $1
		ORDER BY is_active DESC, name ASC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query observation points: %w", err)
	}
	defer rows.Close()

	var points []ObservationPoint
	for rows.Next() {
		var p ObservationPoint
		err := rows.Scan(
			&p.ID,
			&p.UserID,
			&p.Name,
			&p.Latitude,
			&p.Longitude,
			&p.ElevationMeters,
			&p.IsActive,
			&p.CreatedAt,
			&p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan observation point: %w", err)
		}
		points = append(points, p)
	}

	return points, nil
}

// GetActivePoint returns the active observation point for a user
func (r *ObservationPointRepository) GetActivePoint(ctx context.Context, userID int) (*ObservationPoint, error) {
	query := `
		SELECT id, user_id, name, latitude, longitude, elevation_meters, is_active, created_at, updated_at
		FROM observation_points
		WHERE user_id = $1 AND is_active = TRUE
		LIMIT 1
	`

	var p ObservationPoint
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&p.ID,
		&p.UserID,
		&p.Name,
		&p.Latitude,
		&p.Longitude,
		&p.ElevationMeters,
		&p.IsActive,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No active point found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active observation point: %w", err)
	}

	return &p, nil
}

// GetByID returns a specific observation point by ID
func (r *ObservationPointRepository) GetByID(ctx context.Context, pointID, userID int) (*ObservationPoint, error) {
	query := `
		SELECT id, user_id, name, latitude, longitude, elevation_meters, is_active, created_at, updated_at
		FROM observation_points
		WHERE id = $1 AND user_id = $2
	`

	var p ObservationPoint
	err := r.db.QueryRowContext(ctx, query, pointID, userID).Scan(
		&p.ID,
		&p.UserID,
		&p.Name,
		&p.Latitude,
		&p.Longitude,
		&p.ElevationMeters,
		&p.IsActive,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("observation point not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get observation point: %w", err)
	}

	return &p, nil
}

// Create creates a new observation point
func (r *ObservationPointRepository) Create(ctx context.Context, point *ObservationPoint) error {
	query := `
		INSERT INTO observation_points (user_id, name, latitude, longitude, elevation_meters, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		point.UserID,
		point.Name,
		point.Latitude,
		point.Longitude,
		point.ElevationMeters,
		point.IsActive,
	).Scan(&point.ID, &point.CreatedAt, &point.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create observation point: %w", err)
	}

	return nil
}

// Update updates an existing observation point
func (r *ObservationPointRepository) Update(ctx context.Context, point *ObservationPoint) error {
	query := `
		UPDATE observation_points
		SET name = $1, latitude = $2, longitude = $3, elevation_meters = $4, is_active = $5, updated_at = NOW()
		WHERE id = $6 AND user_id = $7
		RETURNING updated_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		point.Name,
		point.Latitude,
		point.Longitude,
		point.ElevationMeters,
		point.IsActive,
		point.ID,
		point.UserID,
	).Scan(&point.UpdatedAt)

	if err == sql.ErrNoRows {
		return fmt.Errorf("observation point not found")
	}
	if err != nil {
		return fmt.Errorf("failed to update observation point: %w", err)
	}

	return nil
}

// Delete deletes an observation point
func (r *ObservationPointRepository) Delete(ctx context.Context, pointID, userID int) error {
	query := `DELETE FROM observation_points WHERE id = $1 AND user_id = $2`

	result, err := r.db.ExecContext(ctx, query, pointID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete observation point: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("observation point not found")
	}

	return nil
}

// SetActive sets a specific observation point as active for the user
func (r *ObservationPointRepository) SetActive(ctx context.Context, pointID, userID int) error {
	query := `
		UPDATE observation_points
		SET is_active = TRUE, updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`

	result, err := r.db.ExecContext(ctx, query, pointID, userID)
	if err != nil {
		return fmt.Errorf("failed to set active observation point: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("observation point not found")
	}

	return nil
}
