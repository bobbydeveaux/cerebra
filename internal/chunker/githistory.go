package chunker

import (
	"fmt"
	"strings"
)

type GitHistoryChunker struct {
	maxSize        int
	commitsPerChunk int
}

func NewGitHistoryChunker(maxSize int) *GitHistoryChunker {
	return &GitHistoryChunker{
		maxSize:        maxSize,
		commitsPerChunk: 20,
	}
}

func (g *GitHistoryChunker) chunk(content string) ([]rawChunk, error) {
	lines := strings.Split(content, "\n")
	var chunks []rawChunk

	for i := 0; i < len(lines); i += g.commitsPerChunk {
		end := i + g.commitsPerChunk
		if end > len(lines) {
			end = len(lines)
		}

		chunkLines := lines[i:end]
		chunkContent := strings.Join(chunkLines, "\n")
		if len(strings.TrimSpace(chunkContent)) == 0 {
			continue
		}

		header := fmt.Sprintf("[Git History] Commits %d-%d\n\n", i+1, end)
		chunks = append(chunks, rawChunk{
			content:   header + chunkContent,
			startLine: i + 1,
			endLine:   end,
		})
	}

	return chunks, nil
}
