package store

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *SQLiteStore) UpsertBrain(ctx context.Context, b Brain) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO brains (
			brain_id, project_path, project_key, session_file,
			agent_type, model, git_branch, status,
			message_count, first_message_at, last_message_at,
			summary, token_usage, last_offset, slug, version,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(brain_id) DO UPDATE SET
			project_path     = excluded.project_path,
			session_file     = excluded.session_file,
			agent_type       = excluded.agent_type,
			model            = excluded.model,
			git_branch       = excluded.git_branch,
			status           = excluded.status,
			message_count    = excluded.message_count,
			last_message_at  = excluded.last_message_at,
			summary          = excluded.summary,
			token_usage      = excluded.token_usage,
			last_offset      = excluded.last_offset,
			slug             = excluded.slug,
			version          = excluded.version,
			updated_at       = CURRENT_TIMESTAMP`,
		b.BrainID, b.ProjectPath, b.ProjectKey, b.SessionFile,
		b.AgentType, b.Model, b.GitBranch, b.Status,
		b.MessageCount, b.FirstMessageAt, b.LastMessageAt,
		b.Summary, b.TokenUsage, b.LastOffset, b.Slug, b.Version,
	)
	if err != nil {
		return fmt.Errorf("upserting brain: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetBrain(ctx context.Context, brainID string) (*Brain, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT brain_id, project_path, project_key, session_file,
		       agent_type, model, git_branch, status,
		       message_count, COALESCE(first_message_at,''), COALESCE(last_message_at,''),
		       summary, token_usage, last_offset, slug, version
		FROM brains WHERE brain_id = ?`, brainID)

	var b Brain
	err := row.Scan(
		&b.BrainID, &b.ProjectPath, &b.ProjectKey, &b.SessionFile,
		&b.AgentType, &b.Model, &b.GitBranch, &b.Status,
		&b.MessageCount, &b.FirstMessageAt, &b.LastMessageAt,
		&b.Summary, &b.TokenUsage, &b.LastOffset, &b.Slug, &b.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting brain: %w", err)
	}
	return &b, nil
}

func (s *SQLiteStore) ListBrains(ctx context.Context, projectKey string, status string, limit int) ([]Brain, error) {
	query := `SELECT brain_id, project_path, project_key, session_file,
	                 agent_type, model, git_branch, status,
	                 message_count, COALESCE(first_message_at,''), COALESCE(last_message_at,''),
	                 summary, token_usage, last_offset, slug, version
	          FROM brains WHERE 1=1`
	var args []interface{}

	if projectKey != "" {
		query += ` AND project_key LIKE ?`
		args = append(args, "%"+projectKey+"%")
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	query += ` ORDER BY last_message_at DESC`

	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing brains: %w", err)
	}
	defer rows.Close()

	var brains []Brain
	for rows.Next() {
		var b Brain
		if err := rows.Scan(
			&b.BrainID, &b.ProjectPath, &b.ProjectKey, &b.SessionFile,
			&b.AgentType, &b.Model, &b.GitBranch, &b.Status,
			&b.MessageCount, &b.FirstMessageAt, &b.LastMessageAt,
			&b.Summary, &b.TokenUsage, &b.LastOffset, &b.Slug, &b.Version,
		); err != nil {
			return nil, fmt.Errorf("scanning brain: %w", err)
		}
		brains = append(brains, b)
	}
	return brains, rows.Err()
}

func (s *SQLiteStore) GetBrainStats(ctx context.Context) (BrainStats, error) {
	var stats BrainStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(CASE WHEN status = 'active' THEN 1 END),
			COALESCE(SUM(message_count), 0),
			COALESCE(SUM(token_usage), 0),
			COUNT(DISTINCT project_key)
		FROM brains`).Scan(
		&stats.TotalBrains, &stats.ActiveBrains,
		&stats.TotalMessages, &stats.TotalTokens, &stats.Projects,
	)
	if err != nil {
		return stats, fmt.Errorf("getting brain stats: %w", err)
	}
	return stats, nil
}
