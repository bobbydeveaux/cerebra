// Package web — Stripe webhook handler for AgentOps paid tier (agentops-011).
//
// Implements POST /api/stripe/webhook. Verifies the Stripe-Signature header
// against STRIPE_WEBHOOK_SECRET, parses the event, and dispatches the two
// events Cerebra cares about: checkout.session.completed (subscription start)
// and customer.subscription.deleted (subscription end). All other events
// return 200 immediately so Stripe stops retrying.
//
// The actual subscription state is owned by a StripeEventHandler the Server
// holds. This file ships the stub (logging) implementation; agentops-012
// replaces it with the LicenseStore-backed implementation.
package web

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	stripe "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/webhook"
)

// stripePayloadMaxBytes caps the body size we are willing to read from a
// Stripe webhook request. Stripe events are typically a few KB; 1 MiB is
// well over the documented maximum and protects against memory abuse from
// a misrouted client.
const stripePayloadMaxBytes = 1 << 20

// StripeEventHandler is the seam between the webhook handler and the
// subscription state store. agentops-011 ships only the interface plus a
// logging stub; agentops-012 wires the real LicenseStore implementation.
type StripeEventHandler interface {
	OnCheckoutComplete(ctx context.Context, event stripe.Event) error
	OnSubscriptionDeleted(ctx context.Context, event stripe.Event) error
}

// loggingStripeHandler is the default StripeEventHandler. It records the
// event ID and type but performs no state change. Replaced by the real
// LicenseStore-backed handler in agentops-012.
type loggingStripeHandler struct{}

func (loggingStripeHandler) OnCheckoutComplete(_ context.Context, event stripe.Event) error {
	log.Printf("stripe: checkout.session.completed id=%s", event.ID)
	return nil
}

func (loggingStripeHandler) OnSubscriptionDeleted(_ context.Context, event stripe.Event) error {
	log.Printf("stripe: customer.subscription.deleted id=%s", event.ID)
	return nil
}

// handleStripeWebhook is the POST /api/stripe/webhook endpoint.
//
// Behaviour:
//   - Reads the raw body (capped) and the Stripe-Signature header.
//   - Calls webhook.ConstructEvent to verify the HMAC-SHA256 signature
//     using STRIPE_WEBHOOK_SECRET from the environment.
//   - Returns 500 if STRIPE_WEBHOOK_SECRET is unset (loud failure beats
//     silently accepting any payload).
//   - Returns 400 if the signature verification fails.
//   - Dispatches checkout.session.completed and customer.subscription.deleted
//     to s.stripeHandler; all other event types are accepted with a 200.
//   - Any handler error becomes a 500 so Stripe retries.
func (s *Server) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	secret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if secret == "" {
		log.Printf("stripe: STRIPE_WEBHOOK_SECRET is not set")
		http.Error(w, "webhook secret not configured", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, stripePayloadMaxBytes))
	if err != nil {
		log.Printf("stripe: read body: %v", err)
		http.Error(w, "could not read request body", http.StatusBadRequest)
		return
	}

	sigHeader := r.Header.Get("Stripe-Signature")
	event, err := webhook.ConstructEventWithOptions(body, sigHeader, secret, webhook.ConstructEventOptions{
		// Stripe accounts pin their own API version. We do not want a
		// version skew between Cerebra's stripe-go and the account's
		// configured version to drop legitimate events on the floor.
		// The fields we read (event.ID, event.Type) are stable across
		// versions; downstream handlers can re-parse if they need
		// version-sensitive fields later.
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		log.Printf("stripe: signature verification failed: %v", err)
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	handler := s.stripeHandler
	if handler == nil {
		handler = loggingStripeHandler{}
	}

	switch event.Type {
	case "checkout.session.completed":
		log.Printf("stripe: dispatching checkout.session.completed id=%s", event.ID)
		if err := handler.OnCheckoutComplete(r.Context(), event); err != nil {
			log.Printf("stripe: OnCheckoutComplete: %v", err)
			http.Error(w, "handler error", http.StatusInternalServerError)
			return
		}
	case "customer.subscription.deleted":
		log.Printf("stripe: dispatching customer.subscription.deleted id=%s", event.ID)
		if err := handler.OnSubscriptionDeleted(r.Context(), event); err != nil {
			log.Printf("stripe: OnSubscriptionDeleted: %v", err)
			http.Error(w, "handler error", http.StatusInternalServerError)
			return
		}
	default:
		// Stripe sends many event types we do not care about. Accept and
		// ignore so Stripe stops retrying.
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// subscriptionWriter is the seam the store-backed Stripe handler writes
// through. *store.SQLiteStore satisfies it. Kept narrow so the webhook
// path does not depend on the whole store surface and so tests can inject
// a fake recorder.
type subscriptionWriter interface {
	SetSubscriptionActive(ctx context.Context, customerID, sessionID string) error
	SetSubscriptionInactive(ctx context.Context, customerID string) error
}

// storeStripeHandler is the real StripeEventHandler (replacing the no-op
// loggingStripeHandler). It translates the two subscription-lifecycle
// events into subscription state writes (agentops-090).
type storeStripeHandler struct {
	store subscriptionWriter
}

// OnCheckoutComplete records an active subscription for the checkout
// session customer. A session with no resolvable customer ID is ignored
// (logged, not errored) so Stripe is not driven into a retry loop over an
// event we cannot key on.
func (h storeStripeHandler) OnCheckoutComplete(ctx context.Context, event stripe.Event) error {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return err
	}
	customerID := ""
	if session.Customer != nil {
		customerID = session.Customer.ID
	}
	if customerID == "" {
		log.Printf("stripe: checkout.session.completed id=%s has no customer; ignoring", event.ID)
		return nil
	}
	return h.store.SetSubscriptionActive(ctx, customerID, session.ID)
}

// OnSubscriptionDeleted marks the customer subscription inactive. A
// missing customer ID is ignored for the same reason as above.
func (h storeStripeHandler) OnSubscriptionDeleted(ctx context.Context, event stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return err
	}
	customerID := ""
	if sub.Customer != nil {
		customerID = sub.Customer.ID
	}
	if customerID == "" {
		log.Printf("stripe: customer.subscription.deleted id=%s has no customer; ignoring", event.ID)
		return nil
	}
	return h.store.SetSubscriptionInactive(ctx, customerID)
}
