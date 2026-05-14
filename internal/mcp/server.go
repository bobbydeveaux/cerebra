package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

type Server struct {
	store    store.Store
	embedder embedder.Embedder
}

func NewServer(s store.Store, emb embedder.Embedder) *Server {
	return &Server{store: s, embedder: emb}
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

func (s *Server) Serve(ctx context.Context) error {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	enc := json.NewEncoder(os.Stdout)

	for sc.Scan() {
		var req jsonRPCRequest
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			continue
		}

		// Notifications have no ID — don't respond to them
		if req.ID == nil || string(req.ID) == "null" {
			continue
		}

		resp := s.handle(ctx, req)
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("encoding response: %w", err)
		}
	}

	return sc.Err()
}

func (s *Server) handle(ctx context.Context, req jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{},
		}
	}
}

func (s *Server) handleInitialize(req jsonRPCRequest) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "cerebra",
				"version": "1.0.0",
			},
		},
	}
}

func (s *Server) handleToolsList(req jsonRPCRequest) jsonRPCResponse {
	tools := []toolDef{
		{
			Name:        "search",
			Description: "Semantic search across the indexed codebase knowledge base",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string", "description": "Natural language search query"},
					"limit": map[string]interface{}{"type": "integer", "default": 5, "description": "Number of results"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "list_categories",
			Description: "List all categories discovered in the indexed codebase",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "get_document",
			Description: "Retrieve a specific indexed document by file path",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "Relative file path"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "get_stats",
			Description: "Get statistics about what Jor-El knows",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "list_brains",
			Description: "List all known agent brains (AI conversation sessions) with metadata including project, model, message count, and token usage",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{"type": "string", "description": "Filter by project name substring"},
					"status":  map[string]interface{}{"type": "string", "enum": []string{"active", "completed"}, "description": "Filter by status"},
					"limit":   map[string]interface{}{"type": "integer", "default": 20, "description": "Maximum results to return"},
				},
			},
		},
		{
			Name:        "get_brain",
			Description: "Get detailed metadata for a specific agent brain by its session ID",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"brain_id": map[string]interface{}{"type": "string", "description": "Session ID (UUID) of the brain"},
				},
				"required": []string{"brain_id"},
			},
		},
		{
			Name:        "get_activity",
			Description: "Get hourly activity breakdown for agent brains. Shows message counts per hour to identify peak working times. Filter by project and/or date.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{"type": "string", "description": "Filter by project name substring"},
					"date":    map[string]interface{}{"type": "string", "description": "ISO date to filter, e.g. 2026-04-28"},
				},
			},
		},
	}

	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"tools": tools},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req jsonRPCRequest) jsonRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	switch params.Name {
	case "search":
		return s.toolSearch(ctx, req.ID, params.Arguments)
	case "list_categories":
		return s.toolListCategories(ctx, req.ID)
	case "get_document":
		return s.toolGetDocument(ctx, req.ID, params.Arguments)
	case "get_stats":
		return s.toolGetStats(ctx, req.ID)
	case "list_brains":
		return s.toolListBrains(ctx, req.ID, params.Arguments)
	case "get_brain":
		return s.toolGetBrain(ctx, req.ID, params.Arguments)
	case "get_activity":
		return s.toolGetActivity(ctx, req.ID, params.Arguments)
	default:
		return errorResponse(req.ID, -32601, "Unknown tool: "+params.Name)
	}
}

func (s *Server) toolSearch(ctx context.Context, id json.RawMessage, args json.RawMessage) jsonRPCResponse {
	var input struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	json.Unmarshal(args, &input)
	if input.Limit <= 0 {
		input.Limit = 10
	}

	// Vector search with FTS fallback
	var results []store.SearchResult
	vecs, err := s.embedder.Embed(ctx, []string{input.Query})
	if err == nil {
		results, _ = s.store.Search(ctx, vecs[0], input.Limit)
	}

	// Fallback to FTS if vector search returned nothing
	if len(results) == 0 {
		results, _ = s.store.SearchFTS(ctx, input.Query, input.Limit)
	}

	if len(results) == 0 {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "No results found for: " + input.Query},
				},
			},
		}
	}

	type resultItem struct {
		Content   string  `json:"content"`
		File      string  `json:"file"`
		Repo      string  `json:"repo"`
		Category  string  `json:"category"`
		Score     float32 `json:"score"`
		LineStart int     `json:"line_start"`
		LineEnd   int     `json:"line_end"`
	}

	items := make([]resultItem, len(results))
	for i, r := range results {
		items[i] = resultItem{
			Content:   r.Chunk.Content,
			File:      r.Chunk.Metadata.Path,
			Repo:      r.Chunk.Metadata.Repo,
			Category:  string(r.Chunk.Metadata.Category),
			Score:     r.Score,
			LineStart: r.Chunk.StartLine,
			LineEnd:   r.Chunk.EndLine,
		}
	}

	content, _ := json.Marshal(items)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": string(content)},
			},
		},
	}
}

func (s *Server) toolListCategories(ctx context.Context, id json.RawMessage) jsonRPCResponse {
	cats, err := s.store.ListCategories(ctx)
	if err != nil {
		return errorResponse(id, -32000, "Error: "+err.Error())
	}

	content, _ := json.Marshal(cats)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": string(content)},
			},
		},
	}
}

func (s *Server) toolGetDocument(ctx context.Context, id json.RawMessage, args json.RawMessage) jsonRPCResponse {
	var input struct {
		Path string `json:"path"`
	}
	json.Unmarshal(args, &input)

	doc, chunks, err := s.store.GetDocument(ctx, input.Path)
	if err != nil {
		return errorResponse(id, -32000, "Not found: "+err.Error())
	}

	type chunkItem struct {
		Content   string `json:"content"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}

	chunkItems := make([]chunkItem, len(chunks))
	for i, c := range chunks {
		chunkItems[i] = chunkItem{Content: c.Content, StartLine: c.StartLine, EndLine: c.EndLine}
	}

	result := map[string]interface{}{
		"path":     doc.RelPath,
		"content":  doc.Content,
		"category": string(doc.Category),
		"language": doc.Language,
		"chunks":   chunkItems,
	}

	content, _ := json.Marshal(result)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": string(content)},
			},
		},
	}
}

func (s *Server) toolGetStats(ctx context.Context, id json.RawMessage) jsonRPCResponse {
	stats, err := s.store.GetStats(ctx)
	if err != nil {
		return errorResponse(id, -32000, "Error: "+err.Error())
	}

	content, _ := json.Marshal(stats)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": string(content)},
			},
		},
	}
}

func (s *Server) toolListBrains(ctx context.Context, id json.RawMessage, args json.RawMessage) jsonRPCResponse {
	var input struct {
		Project string `json:"project"`
		Status  string `json:"status"`
		Limit   int    `json:"limit"`
	}
	json.Unmarshal(args, &input)
	if input.Limit <= 0 {
		input.Limit = 20
	}

	brains, err := s.store.ListBrains(ctx, input.Project, input.Status, input.Limit)
	if err != nil {
		return errorResponse(id, -32000, "Error: "+err.Error())
	}

	content, _ := json.Marshal(brains)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": string(content)},
			},
		},
	}
}

func (s *Server) toolGetBrain(ctx context.Context, id json.RawMessage, args json.RawMessage) jsonRPCResponse {
	var input struct {
		BrainID string `json:"brain_id"`
	}
	json.Unmarshal(args, &input)

	brain, err := s.store.GetBrain(ctx, input.BrainID)
	if err != nil {
		return errorResponse(id, -32000, "Error: "+err.Error())
	}
	if brain == nil {
		return errorResponse(id, -32000, "Brain not found: "+input.BrainID)
	}

	content, _ := json.Marshal(brain)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": string(content)},
			},
		},
	}
}

func (s *Server) toolGetActivity(ctx context.Context, id json.RawMessage, args json.RawMessage) jsonRPCResponse {
	var input struct {
		Project string `json:"project"`
		Date    string `json:"date"`
	}
	json.Unmarshal(args, &input)

	rows, err := s.store.ListActivity(ctx, input.Project, input.Date)
	if err != nil {
		return errorResponse(id, -32000, "Error: "+err.Error())
	}

	content, _ := json.Marshal(rows)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": string(content)},
			},
		},
	}
}

func errorResponse(id json.RawMessage, code int, message string) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonRPCError{Code: code, Message: message},
	}
}
