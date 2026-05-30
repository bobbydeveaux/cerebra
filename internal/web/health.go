// Package web — liveness/readiness probe handler (agentops-038).
//
// handleHealth serves GET /health for Cloud Run probes and any external
// uptime monitor. It is intentionally decoupled from the store, embedder,
// pipeline, and Stripe handler: a degraded dependency must not flip the
// health endpoint to 5xx, otherwise Cloud Run will recycle a pod that is
// still serving traffic correctly for unaffected routes.
package web

import (
	"encoding/json"
	"net/http"
)

// buildVersion is the build identity reported by /health. The default
// "dev" is overridden at link time via
//
//	-ldflags "-X github.com/bobbydeveaux/cerebra/internal/web.buildVersion=<git-sha>"
//
// when StackRamp builds the production image. Tests rely on the default
// value, so we keep it as a package-level var rather than a constant.
var buildVersion = "dev"

// healthResponse is the documented response shape: status is always "ok"
// when the endpoint serves at all (a non-ok status would arrive as a
// non-200 from the load balancer above us), and version surfaces the
// build identity for incident triage.
type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// handleHealth responds with 200 and a small JSON document. It does not
// query the database, call Stripe, or touch any external service — the
// endpoint is for *liveness*, not readiness-with-dependencies. If we ever
// need a separate readiness probe with deeper checks, that goes at
// /ready, not here.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:  "ok",
		Version: buildVersion,
	})
}
