package store

import (
	"context"
	"fmt"
	"strings"
)

// UpsertAgentMessage inserts or updates an agent_message by ID (the tool_use_id).
//
// The two halves of a subagent invocation — the tool_use (with prompt) and the
// matching tool_result (with response) — may arrive in different incremental
// parse passes. The ON CONFLICT clause uses NULLIF/excluded semantics so that
// each half can fill in the fields it knows about without clobbering the other.
//
// Mirrors the upsert into the FTS5 virtual table so search stays in sync.
func (s *SQLiteStore) UpsertAgentMessage(ctx context.Context, m AgentMessage) error {
	if m.ID == "" {
		return fmt.Errorf("agent message id is required")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_messages (id, brain_id, agent_name, description, prompt, response, timestamp, project_key, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			brain_id    = CASE WHEN excluded.brain_id    <> '' THEN excluded.brain_id    ELSE agent_messages.brain_id    END,
			agent_name  = CASE WHEN excluded.agent_name  <> '' THEN excluded.agent_name  ELSE agent_messages.agent_name  END,
			description = CASE WHEN excluded.description <> '' THEN excluded.description ELSE agent_messages.description END,
			prompt      = CASE WHEN excluded.prompt      <> '' THEN excluded.prompt      ELSE agent_messages.prompt      END,
			response    = CASE WHEN excluded.response    <> '' THEN excluded.response    ELSE agent_messages.response    END,
			timestamp   = CASE WHEN excluded.timestamp IS NOT NULL AND excluded.timestamp <> '' THEN excluded.timestamp ELSE agent_messages.timestamp END,
			project_key = CASE WHEN excluded.project_key <> '' THEN excluded.project_key ELSE agent_messages.project_key END,
			updated_at  = CURRENT_TIMESTAMP`,
		m.ID, m.BrainID, m.AgentName, m.Description, m.Prompt, m.Response, m.Timestamp, m.ProjectKey,
	)
	if err != nil {
		return fmt.Errorf("upsert agent_message: %w", err)
	}

	// Best-effort sync into FTS table — replace the row so search reflects the
	// merged state of prompt+response. If FTS5 isn't available this fails
	// quietly and search just falls back to the main table.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM agent_messages_fts WHERE id = ?`, m.ID); err == nil {
		// Read back the merged row so FTS reflects the post-merge state, not just
		// what this call supplied.
		var prompt, response, agentName, brainID string
		_ = s.db.QueryRowContext(ctx,
			`SELECT prompt, response, agent_name, brain_id FROM agent_messages WHERE id = ?`,
			m.ID,
		).Scan(&prompt, &response, &agentName, &brainID)
		_, _ = s.db.ExecContext(ctx,
			`INSERT INTO agent_messages_fts (prompt, response, agent_name, brain_id, id) VALUES (?, ?, ?, ?, ?)`,
			prompt, response, agentName, brainID, m.ID,
		)
	}

	return nil
}

// SearchAgentMessages performs FTS5 search across an agent's prompts and responses.
// If query is empty, returns the most recent invocations for that agent. If agentName
// is empty, searches across all agents.
func (s *SQLiteStore) SearchAgentMessages(ctx context.Context, agentName string, query string, limit int) ([]AgentMessage, error) {
	if limit <= 0 {
		limit = 20
	}

	// Empty query → return recent invocations (no FTS needed)
	if strings.TrimSpace(query) == "" {
		return s.ListAgentActivity(ctx, agentName, "", "", limit)
	}

	// FTS5 query — both prompt and response are indexed
	sql := `
		SELECT m.id, m.brain_id, m.agent_name, m.description, m.prompt, m.response, m.timestamp, m.project_key
		FROM agent_messages m
		JOIN agent_messages_fts fts ON fts.id = m.id
		WHERE agent_messages_fts MATCH ?`
	args := []interface{}{query}

	if agentName != "" {
		sql += ` AND m.agent_name = ?`
		args = append(args, agentName)
	}
	sql += ` ORDER BY rank LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		// FTS5 may not be available — fall back to plain LIKE
		return s.searchAgentMessagesFallback(ctx, agentName, query, limit)
	}
	defer rows.Close()

	return scanAgentMessages(rows)
}

func (s *SQLiteStore) searchAgentMessagesFallback(ctx context.Context, agentName string, query string, limit int) ([]AgentMessage, error) {
	pattern := "%" + query + "%"
	sql := `
		SELECT id, brain_id, agent_name, description, prompt, response, timestamp, project_key
		FROM agent_messages
		WHERE (prompt LIKE ? OR response LIKE ?)`
	args := []interface{}{pattern, pattern}
	if agentName != "" {
		sql += ` AND agent_name = ?`
		args = append(args, agentName)
	}
	sql += ` ORDER BY timestamp DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("agent_messages LIKE search: %w", err)
	}
	defer rows.Close()
	return scanAgentMessages(rows)
}

// ListAgentActivity returns agent invocations filtered by agent name and date range.
// Dates are matched as YYYY-MM-DD prefixes against the ISO timestamp. Empty strings
// for either bound are open-ended.
func (s *SQLiteStore) ListAgentActivity(ctx context.Context, agentName string, startDate string, endDate string, limit int) ([]AgentMessage, error) {
	if limit <= 0 {
		limit = 50
	}

	sql := `
		SELECT id, brain_id, agent_name, description, prompt, response, timestamp, project_key
		FROM agent_messages
		WHERE 1=1`
	args := []interface{}{}

	if agentName != "" {
		sql += ` AND agent_name = ?`
		args = append(args, agentName)
	}
	if startDate != "" {
		sql += ` AND timestamp >= ?`
		args = append(args, startDate)
	}
	if endDate != "" {
		// Inclusive upper bound — append "Z" to push past any time-of-day
		sql += ` AND timestamp <= ?`
		args = append(args, endDate+"T23:59:59.999Z")
	}

	sql += ` ORDER BY timestamp DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list agent activity: %w", err)
	}
	defer rows.Close()
	return scanAgentMessages(rows)
}

// scanAgentMessages is shared by Search and List — column order must match.
func scanAgentMessages(rows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}) ([]AgentMessage, error) {
	var out []AgentMessage
	for rows.Next() {
		var m AgentMessage
		if err := rows.Scan(&m.ID, &m.BrainID, &m.AgentName, &m.Description, &m.Prompt, &m.Response, &m.Timestamp, &m.ProjectKey); err != nil {
			return nil, fmt.Errorf("scan agent_message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
