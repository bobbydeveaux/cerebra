package chunker

import (
	"regexp"
	"strings"
)

var codePatterns = map[string]*regexp.Regexp{
	"go":         regexp.MustCompile(`(?m)^func\s+`),
	"python":     regexp.MustCompile(`(?m)^(def |class )\w+`),
	"javascript": regexp.MustCompile(`(?m)^(function |const \w+ = |class )\s*`),
	"typescript": regexp.MustCompile(`(?m)^(export )?(function |const \w+ = |class |interface |type )\s*`),
	"rust":       regexp.MustCompile(`(?m)^(pub )?(fn |impl |struct |enum |trait )\w+`),
	"java":       regexp.MustCompile(`(?m)^\s*(public|private|protected)\s+.*\{`),
	"ruby":       regexp.MustCompile(`(?m)^(def |class |module )\w+`),
	"php":        regexp.MustCompile(`(?m)^(function |class |public |private |protected )\w+`),
	"c":          regexp.MustCompile(`(?m)^\w[\w\s\*]+\w+\s*\([^)]*\)\s*\{`),
	"cpp":        regexp.MustCompile(`(?m)^[\w\s\*:~]+\w+\s*\([^)]*\)\s*(const\s*)?\{`),
	"csharp":     regexp.MustCompile(`(?m)^\s*(public|private|protected|internal)\s+.*\{`),
	"swift":      regexp.MustCompile(`(?m)^(func |class |struct |enum |protocol )\w+`),
	"kotlin":     regexp.MustCompile(`(?m)^(fun |class |object |interface )\w+`),
	"elixir":     regexp.MustCompile(`(?m)^(def |defp |defmodule )\w+`),
	"shell":      regexp.MustCompile(`(?m)^\w+\s*\(\)\s*\{`),
}

type CodeChunker struct {
	maxSize int
}

func NewCodeChunker(maxSize int) *CodeChunker {
	return &CodeChunker{maxSize: maxSize}
}

func (c *CodeChunker) chunk(content string, language string) ([]rawChunk, error) {
	lines := strings.Split(content, "\n")
	pattern, hasPattern := codePatterns[language]

	if !hasPattern {
		return chunkByLines(lines, c.maxSize), nil
	}

	locs := pattern.FindAllStringIndex(content, -1)
	if len(locs) == 0 {
		return chunkByLines(lines, c.maxSize), nil
	}

	// Convert byte offsets to line numbers
	lineStarts := make([]int, 0, len(locs))
	for _, loc := range locs {
		lineNum := strings.Count(content[:loc[0]], "\n") + 1
		lineStarts = append(lineStarts, lineNum)
	}

	var chunks []rawChunk
	for i, start := range lineStarts {
		end := len(lines)
		if i+1 < len(lineStarts) {
			end = lineStarts[i+1] - 1
		}

		if start < 1 {
			start = 1
		}
		if end > len(lines) {
			end = len(lines)
		}

		chunkLines := lines[start-1 : end]
		chunkContent := strings.Join(chunkLines, "\n")

		if len(chunkLines) < 3 {
			continue
		}

		chunks = append(chunks, rawChunk{
			content:   chunkContent,
			startLine: start,
			endLine:   end,
		})
	}

	// Add any content before the first function as a preamble chunk
	if len(lineStarts) > 0 && lineStarts[0] > 1 {
		preamble := strings.Join(lines[:lineStarts[0]-1], "\n")
		if len(strings.TrimSpace(preamble)) > 10 {
			chunks = append([]rawChunk{{
				content:   preamble,
				startLine: 1,
				endLine:   lineStarts[0] - 1,
			}}, chunks...)
		}
	}

	return chunks, nil
}

func chunkByLines(lines []string, maxSize int) []rawChunk {
	linesPerChunk := maxSize / 4 // rough estimate: 4 tokens per line
	if linesPerChunk < 20 {
		linesPerChunk = 20
	}

	var chunks []rawChunk
	for i := 0; i < len(lines); i += linesPerChunk {
		end := i + linesPerChunk
		if end > len(lines) {
			end = len(lines)
		}
		content := strings.Join(lines[i:end], "\n")
		if len(strings.TrimSpace(content)) > 0 {
			chunks = append(chunks, rawChunk{
				content:   content,
				startLine: i + 1,
				endLine:   end,
			})
		}
	}
	return chunks
}
