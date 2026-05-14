package chunker

import (
	"crypto/sha256"
	"fmt"

	"github.com/bobbydeveaux/cerebra/internal/scanner"
)

type Chunk struct {
	ID         string
	DocumentID string
	Content    string
	StartLine  int
	EndLine    int
	Embedding  []float32
	Metadata   ChunkMeta
}

type ChunkMeta struct {
	Path     string
	Repo     string
	Category scanner.Category
	Language string
	FileType scanner.FileType
}

type Chunker interface {
	Chunk(doc scanner.Document) ([]Chunk, error)
}

type Dispatcher struct {
	codeChunker      *CodeChunker
	markdownChunker  *MarkdownChunker
	configChunker    *ConfigChunker
	gitChunker       *GitHistoryChunker
	maxChunkSize     int
}

func NewDispatcher(maxChunkSize int) *Dispatcher {
	if maxChunkSize <= 0 {
		maxChunkSize = 512
	}
	return &Dispatcher{
		codeChunker:     NewCodeChunker(maxChunkSize),
		markdownChunker: NewMarkdownChunker(maxChunkSize),
		configChunker:   NewConfigChunker(maxChunkSize),
		gitChunker:      NewGitHistoryChunker(maxChunkSize),
		maxChunkSize:    maxChunkSize,
	}
}

func (d *Dispatcher) Chunk(doc scanner.Document) ([]Chunk, error) {
	var rawChunks []rawChunk
	var err error

	switch doc.FileType {
	case scanner.FileTypeCode:
		rawChunks, err = d.codeChunker.chunk(doc.Content, doc.Language)
	case scanner.FileTypeMarkdown, scanner.FileTypeWiki, scanner.FileTypeConversation:
		rawChunks, err = d.markdownChunker.chunk(doc.Content)
	case scanner.FileTypeConfig:
		rawChunks, err = d.configChunker.chunk(doc.Content)
	case scanner.FileTypeGitHistory:
		rawChunks, err = d.gitChunker.chunk(doc.Content)
	default:
		rawChunks, err = d.markdownChunker.chunk(doc.Content)
	}

	if err != nil {
		return nil, err
	}

	if len(rawChunks) == 0 && len(doc.Content) > 0 {
		rawChunks = []rawChunk{{content: doc.Content, startLine: 1, endLine: countLines(doc.Content)}}
	}

	meta := ChunkMeta{
		Path:     doc.RelPath,
		Repo:     doc.Repo,
		Category: doc.Category,
		Language: doc.Language,
		FileType: doc.FileType,
	}

	chunks := make([]Chunk, 0, len(rawChunks))
	for _, rc := range rawChunks {
		if len(rc.content) < 10 {
			continue
		}
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", doc.ID, rc.startLine))))
		chunks = append(chunks, Chunk{
			ID:         id,
			DocumentID: doc.ID,
			Content:    rc.content,
			StartLine:  rc.startLine,
			EndLine:    rc.endLine,
			Metadata:   meta,
		})
	}

	return chunks, nil
}

type rawChunk struct {
	content   string
	startLine int
	endLine   int
}

func countLines(s string) int {
	n := 1
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n
}
