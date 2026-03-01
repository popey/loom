package database

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jordanhubbard/loom/pkg/models"
)

// UpsertOrgChart inserts or updates an org chart
func (d *Database) UpsertOrgChart(chart *models.OrgChart) error {
	if chart == nil {
		return fmt.Errorf("org chart cannot be nil")
	}

	if chart.CreatedAt.IsZero() {
		chart.CreatedAt = time.Now()
	}
	chart.UpdatedAt = time.Now()

	// Marshal positions to JSON
	positionsJSON, err := json.Marshal(chart.Positions)
	if err != nil {
		return fmt.Errorf("failed to marshal positions: %w", err)
	}

	query := `
		INSERT INTO org_charts (id, project_id, name, positions, is_template, parent_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			positions = excluded.positions,
			is_template = excluded.is_template,
			parent_id = excluded.parent_id,
			updated_at = excluded.updated_at
	`

	_, err = d.db.Exec(rebind(query),
		chart.ID,
		chart.ProjectID,
		chart.Name,
		string(positionsJSON),
		chart.IsTemplate,
		chart.ParentID,
		chart.CreatedAt,
		chart.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert org chart: %w", err)
	}

	return nil
}

// GetOrgChart retrieves an org chart by ID
func (d *Database) GetOrgChart(id string) (*models.OrgChart, error) {
	query := `
		SELECT id, project_id, name, positions, is_template, parent_id, created_at, updated_at
		FROM org_charts
		WHERE id = ?
	`

	chart := &models.OrgChart{}
	var positionsJSON string

	err := d.db.QueryRow(rebind(query), id).Scan(
		&chart.ID,
		&chart.ProjectID,
		&chart.Name,
		&positionsJSON,
		&chart.IsTemplate,
		&chart.ParentID,
		&chart.CreatedAt,
		&chart.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get org chart: %w", err)
	}

	// Unmarshal positions
	if err := json.Unmarshal([]byte(positionsJSON), &chart.Positions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal positions: %w", err)
	}

	return chart, nil
}

// GetOrgChartByProject retrieves an org chart by project ID
func (d *Database) GetOrgChartByProject(projectID string) (*models.OrgChart, error) {
	query := `
		SELECT id, project_id, name, positions, is_template, parent_id, created_at, updated_at
		FROM org_charts
		WHERE project_id = ? AND is_template = false
		LIMIT 1
	`

	chart := &models.OrgChart{}
	var positionsJSON string

	err := d.db.QueryRow(rebind(query), projectID).Scan(
		&chart.ID,
		&chart.ProjectID,
		&chart.Name,
		&positionsJSON,
		&chart.IsTemplate,
		&chart.ParentID,
		&chart.CreatedAt,
		&chart.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get org chart for project: %w", err)
	}

	// Unmarshal positions
	if err := json.Unmarshal([]byte(positionsJSON), &chart.Positions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal positions: %w", err)
	}

	return chart, nil
}

// ListOrgCharts retrieves all org charts
func (d *Database) ListOrgCharts() ([]*models.OrgChart, error) {
	query := `
		SELECT id, project_id, name, positions, is_template, parent_id, created_at, updated_at
		FROM org_charts
		ORDER BY created_at DESC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list org charts: %w", err)
	}
	defer rows.Close()

	var charts []*models.OrgChart
	for rows.Next() {
		chart := &models.OrgChart{}
		var positionsJSON string

		err := rows.Scan(
			&chart.ID,
			&chart.ProjectID,
			&chart.Name,
			&positionsJSON,
			&chart.IsTemplate,
			&chart.ParentID,
			&chart.CreatedAt,
			&chart.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan org chart: %w", err)
		}

		// Unmarshal positions
		if err := json.Unmarshal([]byte(positionsJSON), &chart.Positions); err != nil {
			return nil, fmt.Errorf("failed to unmarshal positions: %w", err)
		}

		charts = append(charts, chart)
	}

	return charts, nil
}

// DeleteOrgChart deletes an org chart
func (d *Database) DeleteOrgChart(id string) error {
	query := `DELETE FROM org_charts WHERE id = ?`
	result, err := d.db.Exec(rebind(query), id)
	if err != nil {
		return fmt.Errorf("failed to delete org chart: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("org chart not found: %s", id)
	}

	return nil
}
