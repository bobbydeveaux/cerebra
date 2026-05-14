package store

import (
	"context"
	"fmt"
)

func (s *SQLiteStore) DeleteBrainActivity(ctx context.Context, brainID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM brain_activity WHERE brain_id = ?`, brainID)
	if err != nil {
		return fmt.Errorf("deleting brain activity: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpsertActivity(ctx context.Context, a HourlyActivity) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO brain_activity (brain_id, hour, project_key, user_msgs, asst_msgs, tool_uses, tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(brain_id, hour) DO UPDATE SET
			user_msgs  = brain_activity.user_msgs  + excluded.user_msgs,
			asst_msgs  = brain_activity.asst_msgs  + excluded.asst_msgs,
			tool_uses  = brain_activity.tool_uses  + excluded.tool_uses,
			tokens     = brain_activity.tokens     + excluded.tokens`,
		a.BrainID, a.Hour, a.ProjectKey, a.UserMsgs, a.AsstMsgs, a.ToolUses, a.Tokens,
	)
	if err != nil {
		return fmt.Errorf("upserting activity: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListActivity(ctx context.Context, projectKey string, date string) ([]HourlyActivity, error) {
	query := `SELECT brain_id, hour, project_key, user_msgs, asst_msgs, tool_uses, tokens
	          FROM brain_activity WHERE 1=1`
	var args []interface{}

	if projectKey != "" {
		query += ` AND project_key LIKE ?`
		args = append(args, "%"+projectKey+"%")
	}
	if date != "" {
		query += ` AND hour LIKE ?`
		args = append(args, date+"%")
	}

	query += ` ORDER BY hour ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing activity: %w", err)
	}
	defer rows.Close()

	var results []HourlyActivity
	for rows.Next() {
		var a HourlyActivity
		if err := rows.Scan(&a.BrainID, &a.Hour, &a.ProjectKey, &a.UserMsgs, &a.AsstMsgs, &a.ToolUses, &a.Tokens); err != nil {
			return nil, fmt.Errorf("scanning activity: %w", err)
		}
		results = append(results, a)
	}
	return results, rows.Err()
}
