package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

type Stats struct {
	Repos      int    `json:"repos"`
	Files      int    `json:"files"`
	Chunks     int    `json:"chunks"`
	Categories int    `json:"categories"`
	LastScan   string `json:"last_scan"`
	DBSizeMB   float64 `json:"db_size_mb"`
}

type CategorySummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	FileCount   int    `json:"file_count"`
	ChunkCount  int    `json:"chunk_count"`
}

type SearchResult struct {
	Chunk     chunker.Chunk
	Score     float32
	Highlight string
}

type ScanState struct {
	Repo          string
	LastCommitSHA string
	LastScanTime  time.Time
	FileCount     int
	ChunkCount    int
}

// Brain represents a single AI agent conversation session.
type Brain struct {
	BrainID        string `json:"brain_id"`
	ProjectPath    string `json:"project_path"`
	ProjectKey     string `json:"project_key"`
	SessionFile    string `json:"session_file"`
	AgentType      string `json:"agent_type"`
	Model          string `json:"model"`
	GitBranch      string `json:"git_branch"`
	Status         string `json:"status"`
	MessageCount   int    `json:"message_count"`
	FirstMessageAt string `json:"first_message_at"`
	LastMessageAt  string `json:"last_message_at"`
	Summary        string `json:"summary"`
	TokenUsage     int    `json:"token_usage"`
	LastOffset     int64  `json:"last_offset"`
	Slug           string `json:"slug"`
	Version        string `json:"version"`
}

// BrainStats holds aggregate statistics about all brains.
type BrainStats struct {
	TotalBrains   int `json:"total_brains"`
	ActiveBrains  int `json:"active_brains"`
	TotalMessages int `json:"total_messages"`
	TotalTokens   int `json:"total_tokens"`
	Projects      int `json:"projects"`
}

// HourlyActivity holds per-hour message counts for a brain.
type HourlyActivity struct {
	BrainID    string `json:"brain_id"`
	Hour       string `json:"hour"`        // ISO hour bucket, e.g. "2026-04-28T14"
	ProjectKey string `json:"project_key"`
	UserMsgs   int    `json:"user_msgs"`
	AsstMsgs   int    `json:"asst_msgs"`
	ToolUses   int    `json:"tool_uses"`
	Tokens     int    `json:"tokens"`
}

// AgentMessage represents a single subagent invocation captured from the Agent tool.
// ID is the tool_use_id, which uniquely identifies the pairing of a tool_use with its
// matching tool_result (the latter may arrive in a later incremental parse).
type AgentMessage struct {
	ID          string `json:"id"`           // tool_use_id (e.g. "toolu_01...")
	BrainID     string `json:"brain_id"`     // parent session id
	AgentName   string `json:"agent_name"`   // subagent_type
	Description string `json:"description"`  // input.description
	Prompt      string `json:"prompt"`       // input.prompt
	Response    string `json:"response"`     // tool_result content (text only)
	Timestamp   string `json:"timestamp"`    // ISO timestamp of the tool_use
	ProjectKey  string `json:"project_key"`
}

type Store interface {
	UpsertDocument(ctx context.Context, doc scanner.Document, chunks []chunker.Chunk) error
	DeleteDocument(ctx context.Context, docID string) error
	UpdateScanState(ctx context.Context, state ScanState) error

	Search(ctx context.Context, queryVec []float32, limit int) ([]SearchResult, error)
	SearchFTS(ctx context.Context, query string, limit int) ([]SearchResult, error)
	GetDocument(ctx context.Context, path string) (*scanner.Document, []chunker.Chunk, error)
	GetScanState(ctx context.Context, repo string) (*ScanState, error)
	GetStats(ctx context.Context) (Stats, error)
	ListCategories(ctx context.Context) ([]CategorySummary, error)
	GetContentHash(ctx context.Context, docID string) (string, error)
	ListFilesByCategory(ctx context.Context, category string, limit int) ([]FileSummary, error)
	ListAllFiles(ctx context.Context) ([]FileSummary, error)

	// Brain methods
	UpsertBrain(ctx context.Context, b Brain) error
	GetBrain(ctx context.Context, brainID string) (*Brain, error)
	ListBrains(ctx context.Context, projectKey string, status string, limit int) ([]Brain, error)
	GetBrainStats(ctx context.Context) (BrainStats, error)

	// Activity methods
	DeleteBrainActivity(ctx context.Context, brainID string) error
	UpsertActivity(ctx context.Context, a HourlyActivity) error
	ListActivity(ctx context.Context, projectKey string, date string) ([]HourlyActivity, error)

	// Agent message methods
	UpsertAgentMessage(ctx context.Context, m AgentMessage) error
	SearchAgentMessages(ctx context.Context, agentName string, query string, limit int) ([]AgentMessage, error)
	ListAgentActivity(ctx context.Context, agentName string, startDate string, endDate string, limit int) ([]AgentMessage, error)

	Close() error
}

type SQLiteStore struct {
	db           *sql.DB
	dimensions   int
	dbPath       string
	vecAvailable bool
	ftsAvailable bool
}

func New(dbPath string, dimensions int) (*SQLiteStore, error) {
	sqlite_vec.Auto()

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	s := &SQLiteStore{
		db:           db,
		dimensions:   dimensions,
		dbPath:       dbPath,
		vecAvailable: true,
		ftsAvailable: true,
	}

	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) initSchema() error {
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "PRAGMA") {
			continue
		}
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing schema statement: %w\nSQL: %s", err, stmt)
		}
	}

	// Create brains table
	for _, stmt := range strings.Split(brainSchemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing brains schema: %w\nSQL: %s", err, stmt)
		}
	}

	// Create licenses table (paid-tier gating, agentops-012)
	for _, stmt := range strings.Split(licenseSchemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing licenses schema: %w\nSQL: %s", err, stmt)
		}
	}

	// Create brain_activity table
	for _, stmt := range strings.Split(activitySchemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing activity schema: %w\nSQL: %s", err, stmt)
		}
	}

	// Create agent_messages table (subagent invocation tracking)
	for _, stmt := range strings.Split(agentMessagesSchemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing agent_messages schema: %w\nSQL: %s", err, stmt)
		}
	}

	// Create agent_messages FTS table (optional — requires fts5 module)
	if _, err := s.db.Exec(agentMessagesFTSSQL); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			log.Printf("Warning: FTS5 not available for agent_messages: %v", err)
		}
	}

	// Create FTS table (optional — requires fts5 module)
	if _, err := s.db.Exec(ftsSQL); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			log.Printf("Warning: FTS5 not available, full-text search disabled: %v", err)
			s.ftsAvailable = false
		}
	}

	// Create vec table (optional — requires sqlite-vec extension)
	vecStmt := vecSQL(s.dimensions)
	if _, err := s.db.Exec(vecStmt); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			// sqlite-vec extension not available — vector search will be disabled
			s.vecAvailable = false
		}
	}

	return nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) UpsertDocument(ctx context.Context, doc scanner.Document, chunks []chunker.Chunk) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Upsert repo if present
	if doc.Repo != "" {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO repos (id, root_path, remote_url, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			 ON CONFLICT(id) DO UPDATE SET root_path=excluded.root_path, remote_url=excluded.remote_url, updated_at=CURRENT_TIMESTAMP`,
			doc.Repo, doc.RepoRoot, doc.Metadata["remote_url"])
		if err != nil {
			return fmt.Errorf("upserting repo: %w", err)
		}
	}

	// Delete existing chunks for this document
	rows, err := tx.QueryContext(ctx, `SELECT id FROM chunks WHERE document_id = ?`, doc.ID)
	if err != nil {
		return err
	}
	var oldChunkIDs []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		oldChunkIDs = append(oldChunkIDs, id)
	}
	rows.Close()

	for _, cid := range oldChunkIDs {
		if s.vecAvailable {
			tx.ExecContext(ctx, `DELETE FROM chunk_embeddings WHERE chunk_id = ?`, cid)
		}
		if s.ftsAvailable {
			tx.ExecContext(ctx, `DELETE FROM chunks_fts WHERE chunk_id = ?`, cid)
		}
	}
	tx.ExecContext(ctx, `DELETE FROM chunks WHERE document_id = ?`, doc.ID)

	// Upsert document
	metaJSON, _ := json.Marshal(doc.Metadata)
	repoID := sql.NullString{String: doc.Repo, Valid: doc.Repo != ""}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO documents (id, path, rel_path, repo_id, category, language, file_type, content, content_hash, metadata, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
		   path=excluded.path, rel_path=excluded.rel_path, repo_id=excluded.repo_id,
		   category=excluded.category, language=excluded.language, file_type=excluded.file_type,
		   content=excluded.content, content_hash=excluded.content_hash, metadata=excluded.metadata,
		   updated_at=CURRENT_TIMESTAMP`,
		doc.ID, doc.Path, doc.RelPath, repoID, string(doc.Category), doc.Language, string(doc.FileType),
		doc.Content, doc.ContentHash, string(metaJSON))
	if err != nil {
		return fmt.Errorf("upserting document: %w", err)
	}

	// Insert chunks
	for _, c := range chunks {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO chunks (id, document_id, content, start_line, end_line) VALUES (?, ?, ?, ?, ?)`,
			c.ID, c.DocumentID, c.Content, c.StartLine, c.EndLine)
		if err != nil {
			return fmt.Errorf("inserting chunk: %w", err)
		}

		// Insert FTS (optional)
		if s.ftsAvailable {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO chunks_fts (content, document_id, chunk_id) VALUES (?, ?, ?)`,
				c.Content, c.DocumentID, c.ID)
			if err != nil {
				return fmt.Errorf("inserting FTS: %w", err)
			}
		}

		// Insert embedding if present and vec extension available
		if len(c.Embedding) > 0 && s.vecAvailable {
			vecBytes, err := sqlite_vec.SerializeFloat32(c.Embedding)
			if err != nil {
				return fmt.Errorf("serializing embedding: %w", err)
			}
			_, err = tx.ExecContext(ctx,
				`INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)`,
				c.ID, vecBytes)
			if err != nil {
				return fmt.Errorf("inserting embedding: %w", err)
			}
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) DeleteDocument(ctx context.Context, docID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id FROM chunks WHERE document_id = ?`, docID)
	if err != nil {
		return err
	}
	var chunkIDs []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		chunkIDs = append(chunkIDs, id)
	}
	rows.Close()

	for _, cid := range chunkIDs {
		if s.vecAvailable {
			tx.ExecContext(ctx, `DELETE FROM chunk_embeddings WHERE chunk_id = ?`, cid)
		}
		if s.ftsAvailable {
			tx.ExecContext(ctx, `DELETE FROM chunks_fts WHERE chunk_id = ?`, cid)
		}
	}

	tx.ExecContext(ctx, `DELETE FROM chunks WHERE document_id = ?`, docID)
	tx.ExecContext(ctx, `DELETE FROM documents WHERE id = ?`, docID)

	return tx.Commit()
}

func (s *SQLiteStore) UpdateScanState(ctx context.Context, state ScanState) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO scan_state (repo_id, last_commit_sha, last_scan_time, file_count, chunk_count)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(repo_id) DO UPDATE SET
		   last_commit_sha=excluded.last_commit_sha, last_scan_time=excluded.last_scan_time,
		   file_count=excluded.file_count, chunk_count=excluded.chunk_count`,
		state.Repo, state.LastCommitSHA, state.LastScanTime, state.FileCount, state.ChunkCount)
	return err
}

func (s *SQLiteStore) GetContentHash(ctx context.Context, docID string) (string, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT content_hash FROM documents WHERE id = ?`, docID).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return hash, err
}

func (s *SQLiteStore) GetDocument(ctx context.Context, path string) (*scanner.Document, []chunker.Chunk, error) {
	var doc scanner.Document
	var repoID sql.NullString
	var metaJSON string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, path, rel_path, repo_id, category, language, file_type, content, content_hash, metadata
		 FROM documents WHERE rel_path = ? OR path = ?`, path, path).
		Scan(&doc.ID, &doc.Path, &doc.RelPath, &repoID, &doc.Category, &doc.Language, &doc.FileType,
			&doc.Content, &doc.ContentHash, &metaJSON)
	if err != nil {
		return nil, nil, err
	}
	if repoID.Valid {
		doc.Repo = repoID.String
	}
	doc.Metadata = make(map[string]string)
	json.Unmarshal([]byte(metaJSON), &doc.Metadata)

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, document_id, content, start_line, end_line FROM chunks WHERE document_id = ? ORDER BY start_line`, doc.ID)
	if err != nil {
		return &doc, nil, err
	}
	defer rows.Close()

	var chunks []chunker.Chunk
	for rows.Next() {
		var c chunker.Chunk
		if err := rows.Scan(&c.ID, &c.DocumentID, &c.Content, &c.StartLine, &c.EndLine); err != nil {
			continue
		}
		c.Metadata = chunker.ChunkMeta{
			Path:     doc.RelPath,
			Repo:     doc.Repo,
			Category: doc.Category,
			Language: doc.Language,
			FileType: doc.FileType,
		}
		chunks = append(chunks, c)
	}

	return &doc, chunks, nil
}

func (s *SQLiteStore) GetScanState(ctx context.Context, repo string) (*ScanState, error) {
	var state ScanState
	err := s.db.QueryRowContext(ctx,
		`SELECT repo_id, last_commit_sha, last_scan_time, file_count, chunk_count FROM scan_state WHERE repo_id = ?`, repo).
		Scan(&state.Repo, &state.LastCommitSHA, &state.LastScanTime, &state.FileCount, &state.ChunkCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *SQLiteStore) GetStats(ctx context.Context) (Stats, error) {
	var stats Stats

	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM repos`).Scan(&stats.Repos)
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents`).Scan(&stats.Files)
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunks`).Scan(&stats.Chunks)
	s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT category) FROM documents`).Scan(&stats.Categories)

	var lastScan sql.NullString
	s.db.QueryRowContext(ctx, `SELECT MAX(last_scan_time) FROM scan_state`).Scan(&lastScan)
	if lastScan.Valid {
		stats.LastScan = lastScan.String
	}

	if info, err := os.Stat(s.dbPath); err == nil {
		stats.DBSizeMB = float64(info.Size()) / 1024 / 1024
	}

	return stats, nil
}

func (s *SQLiteStore) ListCategories(ctx context.Context) ([]CategorySummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT category, COUNT(*) as file_count,
		 (SELECT COUNT(*) FROM chunks c2
		  JOIN documents d2 ON c2.document_id = d2.id
		  WHERE d2.category = d.category) as chunk_count
		 FROM documents d GROUP BY category ORDER BY file_count DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []CategorySummary
	for rows.Next() {
		var c CategorySummary
		if err := rows.Scan(&c.Name, &c.FileCount, &c.ChunkCount); err != nil {
			continue
		}
		cats = append(cats, c)
	}
	return cats, nil
}

type FileSummary struct {
	RelPath  string
	Language string
	FileType string
}

func (s *SQLiteStore) ListFilesByCategory(ctx context.Context, category string, limit int) ([]FileSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT rel_path, language, file_type FROM documents WHERE category = ? ORDER BY rel_path LIMIT ?`,
		category, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileSummary
	for rows.Next() {
		var f FileSummary
		if err := rows.Scan(&f.RelPath, &f.Language, &f.FileType); err != nil {
			continue
		}
		files = append(files, f)
	}
	return files, nil
}

func (s *SQLiteStore) ListAllFiles(ctx context.Context) ([]FileSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT rel_path, language, file_type FROM documents ORDER BY rel_path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileSummary
	for rows.Next() {
		var f FileSummary
		if err := rows.Scan(&f.RelPath, &f.Language, &f.FileType); err != nil {
			continue
		}
		files = append(files, f)
	}
	return files, nil
}
