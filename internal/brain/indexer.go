package brain

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// Indexer indexes brain conversation content into the vector store
// so it becomes searchable alongside code and documentation.
type Indexer struct {
	store      store.Store
	embedder   embedder.Embedder
	pool       *embedder.Pool
	dispatcher *chunker.Dispatcher
}

// NewIndexer creates a brain indexer that uses the existing embedding pipeline.
func NewIndexer(s store.Store, emb embedder.Embedder, workers, batchSize, chunkSize int) *Indexer {
	return &Indexer{
		store:      s,
		embedder:   emb,
		pool:       embedder.NewPool(emb, workers, batchSize),
		dispatcher: chunker.NewDispatcher(chunkSize),
	}
}

// IndexBrain extracts conversation text from a JSONL file and indexes it
// into the vector store as a searchable document.
func (idx *Indexer) IndexBrain(ctx context.Context, b *store.Brain) error {
	content, err := extractConversationText(b.SessionFile)
	if err != nil {
		return fmt.Errorf("extracting conversation: %w", err)
	}

	if len(content) < 20 {
		return nil // too short to be useful
	}

	contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	docID := fmt.Sprintf("%x", sha256.Sum256([]byte("brain:"+b.BrainID)))

	// Check if content has changed
	existingHash, _ := idx.store.GetContentHash(ctx, docID)
	if existingHash == contentHash {
		return nil // no change
	}

	projectName := filepath.Base(b.ProjectPath)
	if projectName == "" || projectName == "." {
		projectName = b.ProjectKey
	}

	relPath := fmt.Sprintf("brains/%s/%s", projectName, b.BrainID[:8])

	doc := scanner.Document{
		ID:          docID,
		Path:        b.SessionFile,
		RelPath:     relPath,
		Repo:        projectName,
		RepoRoot:    b.ProjectPath,
		Category:    scanner.CategoryDocs,
		Language:    "",
		FileType:    scanner.FileTypeConversation,
		SourceType:  scanner.SourceTypeConversation,
		Content:     content,
		ContentHash: contentHash,
		ModTime:     time.Now(),
		Metadata: map[string]string{
			"brain_id":   b.BrainID,
			"model":      b.Model,
			"agent_type": b.AgentType,
			"git_branch": b.GitBranch,
		},
	}

	chunks, err := idx.dispatcher.Chunk(doc)
	if err != nil {
		return fmt.Errorf("chunking conversation: %w", err)
	}

	if len(chunks) == 0 {
		return nil
	}

	chunks, err = idx.pool.EmbedChunks(ctx, chunks, nil)
	if err != nil {
		return fmt.Errorf("embedding conversation: %w", err)
	}

	if err := idx.store.UpsertDocument(ctx, doc, chunks); err != nil {
		return fmt.Errorf("storing conversation: %w", err)
	}

	return nil
}

// extractConversationText reads a JSONL file and builds a readable
// conversation transcript from user and assistant messages.
func extractConversationText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var b strings.Builder
	msgNum := 0

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry struct {
			Type    string `json:"type"`
			CWD     string `json:"cwd"`
			Message *struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message,omitempty"`
		}

		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type != "user" && entry.Type != "assistant" {
			continue
		}

		if entry.Message == nil {
			continue
		}

		text := extractMessageText(entry.Message.Content)
		if text == "" {
			continue
		}

		// Strip thinking blocks from assistant messages
		if entry.Type == "assistant" {
			text = stripThinkingBlocks(text)
		}

		if len(text) < 5 {
			continue
		}

		msgNum++
		role := "User"
		if entry.Type == "assistant" {
			role = "Assistant"
		}

		b.WriteString(fmt.Sprintf("## %s (message %d)\n\n", role, msgNum))
		b.WriteString(text)
		b.WriteString("\n\n")
	}

	if err := sc.Err(); err != nil && err != io.ErrUnexpectedEOF {
		return b.String(), nil // return what we have
	}

	return b.String(), nil
}

// extractMessageText extracts plain text from a message content field.
// Content can be a JSON string or an array of content blocks.
func extractMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Try array of blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, block := range blocks {
			if block.Type == "text" && block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
	}

	return ""
}

// stripThinkingBlocks removes <think>...</think> blocks from text.
func stripThinkingBlocks(s string) string {
	for {
		start := strings.Index(s, "<think>")
		if start == -1 {
			break
		}
		end := strings.Index(s, "</think>")
		if end == -1 {
			// Unclosed think tag — strip from start to end
			s = s[:start]
			break
		}
		s = s[:start] + s[end+len("</think>"):]
	}
	return strings.TrimSpace(s)
}
