package chunker

import (
	"regexp"
	"strings"
)

var headingPattern = regexp.MustCompile(`(?m)^#{1,3}\s+`)

type MarkdownChunker struct {
	maxSize int
}

func NewMarkdownChunker(maxSize int) *MarkdownChunker {
	return &MarkdownChunker{maxSize: maxSize}
}

func (m *MarkdownChunker) chunk(content string) ([]rawChunk, error) {
	lines := strings.Split(content, "\n")
	locs := headingPattern.FindAllStringIndex(content, -1)

	if len(locs) == 0 {
		return chunkByParagraphs(content), nil
	}

	lineStarts := make([]int, 0, len(locs))
	for _, loc := range locs {
		lineNum := strings.Count(content[:loc[0]], "\n") + 1
		lineStarts = append(lineStarts, lineNum)
	}

	var chunks []rawChunk

	// Content before first heading
	if lineStarts[0] > 1 {
		pre := strings.Join(lines[:lineStarts[0]-1], "\n")
		if len(strings.TrimSpace(pre)) > 10 {
			chunks = append(chunks, rawChunk{
				content:   pre,
				startLine: 1,
				endLine:   lineStarts[0] - 1,
			})
		}
	}

	for i, start := range lineStarts {
		end := len(lines)
		if i+1 < len(lineStarts) {
			end = lineStarts[i+1] - 1
		}

		chunkContent := strings.Join(lines[start-1:end], "\n")
		chunks = append(chunks, rawChunk{
			content:   chunkContent,
			startLine: start,
			endLine:   end,
		})
	}

	return chunks, nil
}

func chunkByParagraphs(content string) []rawChunk {
	paragraphs := strings.Split(content, "\n\n")
	var chunks []rawChunk
	currentLine := 1

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if len(para) < 10 {
			currentLine += strings.Count(para, "\n") + 2
			continue
		}
		endLine := currentLine + strings.Count(para, "\n")
		chunks = append(chunks, rawChunk{
			content:   para,
			startLine: currentLine,
			endLine:   endLine,
		})
		currentLine = endLine + 2
	}

	return chunks
}
