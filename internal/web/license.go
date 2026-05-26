// Package web — license validation middleware (agentops-012).
//
// RequirePaid wraps an http.Handler and only forwards requests whose
// Authorization: Bearer <key> header matches a paid entry in the
// LicenseStore. The free tier is controlled by an env var so local dev
// keeps working without Stripe: CEREBRA_FREE_TIER_ENABLED unset or "true"
// makes the middleware a no-op. Setting it to "false" turns the wall on.
//
// On rejection the middleware returns:
//
//	HTTP/1.1 402 Payment Required
//	Content-Type: application/json
//
//	{"error":"paid subscription required",
//	 "upgrade_url":"https://cerebra.stackramp.io/pricing"}
//
// Free-tier read endpoints (/api/search, /api/brains/{id}) intentionally
// do NOT wear this middleware; only /api/chat/stream does (see server.go).
package web

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/bobbydeveaux/cerebra/internal/store"
)

// upgradeURL is the public pricing page Cerebra points free-tier callers
// at when they hit a paid endpoint without entitlement. Hard-coded rather
// than env-config-driven because it is the brand URL and we want the same
// URL in every deployment.
const upgradeURL = "https://cerebra.stackramp.io/pricing"

// freeTierEnvVar is the env var that toggles the wall. Default behaviour
// (unset or "true") leaves the wall down for local dev; setting it to
// "false" turns the wall on for prod.
const freeTierEnvVar = "CEREBRA_FREE_TIER_ENABLED"

// freeTierEnabled reports whether RequirePaid should short-circuit and
// allow every caller. Default true so a missing env var does not break
// existing local dev workflows.
func freeTierEnabled() bool {
	v := strings.TrimSpace(os.Getenv(freeTierEnvVar))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// licenseStoreFunc is how the middleware reaches the current LicenseStore.
// We do not capture the store value at wrap time because the Server is
// constructed before WithLicenseStore is called — the route closure must
// read the latest store on each request.
type licenseStoreFunc func() store.LicenseStore

// RequirePaid returns middleware that gates the next handler behind a paid
// licence. The store is resolved at request time so callers can wire it
// up after route registration (e.g. via Server.WithLicenseStore). If the
// resolver returns nil OR the free-tier env var is on, the middleware is
// a transparent pass-through.
//
// The API key MUST arrive in an `Authorization: Bearer <key>` header.
// Earlier versions of this middleware accepted a `?key=<key>` query
// parameter as a fallback for EventSource consumers — that path was
// removed in response to Codex pass 3 [P1] because URLs leak into
// browser history, proxy/access logs, and Referer headers. The browser
// chat page now uses `fetch()` + ReadableStream so it can carry the
// Authorization header directly; other consumers should do the same or
// proxy through a server-side endpoint that holds the key.
func RequirePaid(resolve licenseStoreFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if freeTierEnabled() {
				next.ServeHTTP(w, r)
				return
			}
			var licenses store.LicenseStore
			if resolve != nil {
				licenses = resolve()
			}
			if licenses == nil {
				next.ServeHTTP(w, r)
				return
			}

			apiKey := extractAPIKey(r)
			if apiKey == "" {
				writePaymentRequired(w)
				return
			}

			paid, err := licenses.IsPaid(r.Context(), apiKey)
			if err != nil {
				log.Printf("license: IsPaid failed: %v", err)
				http.Error(w, "license lookup failed", http.StatusInternalServerError)
				return
			}
			if !paid {
				writePaymentRequired(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractAPIKey reads the Cerebra API key from the request. The only
// accepted source is the `Authorization: Bearer <key>` header — the
// previously supported `?key=<key>` query parameter was removed because
// URLs leak through logs and history (Codex pass 3 [P1]).
func extractAPIKey(r *http.Request) string {
	return bearerToken(r.Header.Get("Authorization"))
}

// bearerToken extracts the token from a "Bearer <token>" Authorization
// header. Returns "" on a missing or malformed header. Case-insensitive
// on the scheme, per RFC 7235.
func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	const bearer = "bearer "
	if len(header) <= len(bearer) || !strings.EqualFold(header[:len(bearer)], bearer) {
		return ""
	}
	return strings.TrimSpace(header[len(bearer):])
}

// writePaymentRequired sends the standard 402 payload.
func writePaymentRequired(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":       "paid subscription required",
		"upgrade_url": upgradeURL,
	})
}
