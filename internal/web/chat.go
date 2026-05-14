package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title": "Chat with Jor-El",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.tmpls["chat.html"].ExecuteTemplate(w, "chat.html", data)
}

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Missing query", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Parse conversation history if provided
	var history []map[string]string
	if h := r.URL.Query().Get("history"); h != "" {
		json.Unmarshal([]byte(h), &history)
	}

	ctx := context.Background()

	ch, err := s.pipeline.AnswerWithHistory(ctx, query, history)
	if err != nil {
		fmt.Fprintf(w, "data: Error: %s\n\n", err.Error())
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	// Strip <think>...</think> blocks from streaming output
	inThink := false
	var buf strings.Builder
	for token := range ch {
		buf.WriteString(token)
		text := buf.String()

		if inThink {
			if idx := strings.Index(text, "</think>"); idx >= 0 {
				inThink = false
				remaining := text[idx+len("</think>"):]
				buf.Reset()
				remaining = strings.TrimLeft(remaining, " \n\r\t")
				if remaining != "" {
					fmt.Fprintf(w, "data: %s\n\n", remaining)
					flusher.Flush()
				}
			} else {
				// Still inside think block, keep buffering
			}
			continue
		}

		if idx := strings.Index(text, "<think>"); idx >= 0 {
			// Send anything before <think>
			before := text[:idx]
			if before != "" {
				fmt.Fprintf(w, "data: %s\n\n", before)
				flusher.Flush()
			}
			inThink = true
			buf.Reset()
			buf.WriteString(text[idx+len("<think>"):])
			continue
		}

		// No think tags, send normally
		buf.Reset()
		fmt.Fprintf(w, "data: %s\n\n", token)
		flusher.Flush()
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}
