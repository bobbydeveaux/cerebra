// Package web — structured HTTP request logging middleware (agentops-036).
//
// loggingMiddleware wraps any http.Handler and emits one JSON line per
// request to the supplied slog.Logger. The line carries the method, path,
// status code, duration in milliseconds, and (when set by the handler via
// SetError) an error string. The middleware is applied at the Handler()
// boundary so both Serve() and tests using httptest exercise the same
// wrapping.
package web

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder is a minimal http.ResponseWriter shim that captures the
// status code written by the underlying handler. When a handler never
// calls WriteHeader (the implicit-200 case from net/http) the recorder
// reports 200, matching what net/http actually sends on the wire.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	errMsg      string
}

// WriteHeader captures the first status code and forwards it. Subsequent
// calls are dropped, matching the net/http documented behaviour where only
// the first WriteHeader takes effect.
func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

// Write defaults the status to 200 when the underlying handler writes
// body bytes without an explicit WriteHeader call. This is how net/http
// itself behaves and keeps the recorded status consistent with the wire.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.status = http.StatusOK
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}

// Flush forwards the call so streaming handlers (chat SSE) keep working
// when wrapped by the middleware.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// loggingMiddleware wraps next and emits one structured JSON log line per
// request to logger. If logger is nil the middleware behaves as a pure
// passthrough — callers who want logs must supply a logger.
func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	if logger == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status_code", rec.status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		}
		if rec.errMsg != "" {
			attrs = append(attrs, slog.String("error", rec.errMsg))
		}
		logger.LogAttrs(r.Context(), slog.LevelInfo, "http_request", attrs...)
	})
}
