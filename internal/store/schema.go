package store

const schemaSQL = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS repos (
    id          TEXT PRIMARY KEY,
    root_path   TEXT NOT NULL,
    remote_url  TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS documents (
    id           TEXT PRIMARY KEY,
    path         TEXT NOT NULL UNIQUE,
    rel_path     TEXT NOT NULL,
    repo_id      TEXT REFERENCES repos(id),
    category     TEXT NOT NULL,
    language     TEXT NOT NULL DEFAULT '',
    file_type    TEXT NOT NULL,
    content      TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    metadata     TEXT NOT NULL DEFAULT '{}',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS chunks (
    id          TEXT PRIMARY KEY,
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    content     TEXT NOT NULL,
    start_line  INT NOT NULL DEFAULT 0,
    end_line    INT NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS scan_state (
    repo_id        TEXT PRIMARY KEY REFERENCES repos(id),
    last_commit_sha TEXT NOT NULL DEFAULT '',
    last_scan_time  DATETIME NOT NULL,
    file_count      INT NOT NULL DEFAULT 0,
    chunk_count     INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS categories (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    file_count  INT NOT NULL DEFAULT 0,
    chunk_count INT NOT NULL DEFAULT 0,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const brainSchemaSQL = `
CREATE TABLE IF NOT EXISTS brains (
    brain_id         TEXT PRIMARY KEY,
    project_path     TEXT NOT NULL,
    project_key      TEXT NOT NULL,
    session_file     TEXT NOT NULL,
    agent_type       TEXT NOT NULL DEFAULT 'cli',
    model            TEXT NOT NULL DEFAULT '',
    git_branch       TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'active',
    message_count    INTEGER NOT NULL DEFAULT 0,
    first_message_at TEXT,
    last_message_at  TEXT,
    summary          TEXT NOT NULL DEFAULT '',
    token_usage      INTEGER NOT NULL DEFAULT 0,
    last_offset      INTEGER NOT NULL DEFAULT 0,
    slug             TEXT NOT NULL DEFAULT '',
    version          TEXT NOT NULL DEFAULT '',
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_brains_project_key ON brains(project_key);
CREATE INDEX IF NOT EXISTS idx_brains_status ON brains(status);
CREATE INDEX IF NOT EXISTS idx_brains_last_message_at ON brains(last_message_at);
`

const activitySchemaSQL = `
CREATE TABLE IF NOT EXISTS brain_activity (
    brain_id    TEXT NOT NULL,
    hour        TEXT NOT NULL,
    project_key TEXT NOT NULL,
    user_msgs   INTEGER NOT NULL DEFAULT 0,
    asst_msgs   INTEGER NOT NULL DEFAULT 0,
    tool_uses   INTEGER NOT NULL DEFAULT 0,
    tokens      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (brain_id, hour),
    FOREIGN KEY (brain_id) REFERENCES brains(brain_id)
);

CREATE INDEX IF NOT EXISTS idx_activity_hour ON brain_activity(hour);
CREATE INDEX IF NOT EXISTS idx_activity_project ON brain_activity(project_key);
`

// FTS table is created separately as it uses different syntax
const ftsSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    content,
    document_id UNINDEXED,
    chunk_id UNINDEXED,
    tokenize = "porter ascii"
);
`

// Vec table creation is dynamic based on dimensions
func vecSQL(dimensions int) string {
	return `CREATE VIRTUAL TABLE IF NOT EXISTS chunk_embeddings USING vec0(
    chunk_id TEXT PRIMARY KEY,
    embedding float[` + itoa(dimensions) + `]
);`
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
