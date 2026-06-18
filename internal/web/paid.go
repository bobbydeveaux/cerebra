// Package web - RequirePaid gating for the AgentOps paid tier (agentops-090).
//
// RequirePaid wraps the premium HTTP handlers (search, chat, agent
// activity) and refuses access with 402 Payment Required unless this
// Cerebra instance has an active subscription recorded by the Stripe
// webhook. The gate FAILS OPEN when STRIPE_WEBHOOK_SECRET is unset: local
// development and the eval CI gate run without Stripe configured and must
// not be blocked. Gating only engages once the operator has wired Stripe.
package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
)

// paidChecker is the narrow seam the gate reads. *store.SQLiteStore
// satisfies it via HasActiveSubscription. Declaring it here (rather than
// importing the broad store.Store) keeps the web tests able to inject a
// tiny fake without implementing the whole store surface.
type paidChecker interface {
	HasActiveSubscription(ctx context.Context) (bool, error)
}

// gatingEnabled reports whether paid-tier gating should engage. Gating is
// off (fail open) unless STRIPE_WEBHOOK_SECRET is set, mirroring the
// webhook handler which also keys off that variable. This keeps a single
// switch for the whole Stripe integration.
func gatingEnabled() bool {
	return os.Getenv("STRIPE_WEBHOOK_SECRET") != ""
}

// checkoutURL returns the configured Stripe checkout URL the 402 body
// points the caller at, or empty if unset.
func checkoutURL() string {
	return os.Getenv("STRIPE_CHECKOUT_URL")
}

// RequirePaid wraps next so it is only reached when the instance has an
// active subscription. Behaviour:
//   - gating disabled (no STRIPE_WEBHOOK_SECRET): pass straight through.
//   - checker is nil: pass through (no subscription source wired).
//   - active subscription: pass through.
//   - no active subscription: 402 with JSON {error, checkout_url}.
//   - checker error: 503 (do not leak access on a transient store fault,
//     but do not pretend it is a payment problem either).
func (s *Server) RequirePaid(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !gatingEnabled() || s.paid == nil {
			next(w, r)
			return
		}
		active, err := s.paid.HasActiveSubscription(r.Context())
		if err != nil {
			log.Printf("paid: subscription check failed: %v", err)
			http.Error(w, "subscription check unavailable", http.StatusServiceUnavailable)
			return
		}
		if active {
			next(w, r)
			return
		}
		writePaymentRequired(w)
	}
}

// writePaymentRequired emits the 402 response carrying the checkout URL so
// the caller knows where to subscribe.
func writePaymentRequired(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":        "payment required: this endpoint needs an active AgentOps subscription",
		"checkout_url": checkoutURL(),
	})
}
