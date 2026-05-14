package chunker

type ConfigChunker struct {
	maxSize int
}

func NewConfigChunker(maxSize int) *ConfigChunker {
	return &ConfigChunker{maxSize: maxSize}
}

func (c *ConfigChunker) chunk(content string) ([]rawChunk, error) {
	if len(content) < 10000 {
		return []rawChunk{{
			content:   content,
			startLine: 1,
			endLine:   countLines(content),
		}}, nil
	}

	// For large configs, split by top-level keys (lines without leading whitespace)
	return chunkByParagraphs(content), nil
}
