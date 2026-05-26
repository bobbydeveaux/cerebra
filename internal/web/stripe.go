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
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/bobbydeveaux/cerebra/internal/store"
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

// licenseStripeHandler is the production StripeEventHandler. It translates
// Stripe events into LicenseStore.Grant / LicenseStore.Revoke calls.
//
// Wire-format expectations:
//
//   - checkout.session.completed: the API key the customer is paying for
//     arrives in client_reference_id. The Cerebra signup flow MUST set
//     this when creating the checkout session — without it we have no way
//     to bind a paid subscription to a key. customer_email is taken from
//     customer_details.email or the legacy customer_email field. The
//     Stripe customer ID is the .customer.id sub-field; the customer is
//     either inlined (when expanded) or a bare ID — we accept both.
//   - customer.subscription.deleted: only the Stripe customer ID matters.
//     We pull it from the embedded subscription object's .customer field.
//
// Any field shape we cannot parse is logged and returned as an error so
// Stripe retries; the alternative — silently succeeding — would leave
// paying users locked out or revoked users still entitled.
type licenseStripeHandler struct {
	store store.LicenseStore
}

func (h *licenseStripeHandler) OnCheckoutComplete(ctx context.Context, event stripe.Event) error {
	if h.store == nil {
		return errors.New("license store is nil")
	}
	sess, err := parseCheckoutSession(event)
	if err != nil {
		return fmt.Errorf("parse checkout session %s: %w", event.ID, err)
	}
	// Stripe fires checkout.session.completed for `payment` and `setup`
	// Checkout Sessions too. Those will never produce a matching
	// customer.subscription.deleted, so granting on them would create
	// licences we can never revoke. Skip cleanly. We accept the event
	// only if it is unambiguously a subscription — either by mode or by
	// having a subscription_id attached. Anything else (payment / setup
	// / unknown shape) is ignored with a log line.
	if !isSubscriptionCheckout(sess) {
		log.Printf("stripe: skipping checkout %s (mode=%q, subscription=%q): not a subscription session", event.ID, sess.Mode, sess.SubscriptionID)
		return nil
	}
	if sess.APIKey == "" {
		// client_reference_id was not set. We refuse to silently accept
		// because there is no other way to bind the subscription to a
		// Cerebra account — better to fail loudly so the signup flow
		// gets fixed.
		return fmt.Errorf("checkout session %s missing client_reference_id (cannot bind to api key)", event.ID)
	}
	if sess.CustomerID == "" {
		// Subscriptions without a customer ID cannot be revoked later —
		// customer.subscription.deleted only carries the customer ID, so
		// an empty customer here would mint an unrevokeable licence.
		// Fail loudly so Stripe retries (the event normally always has
		// a customer attached for subscription mode).
		return fmt.Errorf("checkout session %s missing customer id (entitlement would be unrevokeable)", event.ID)
	}
	// Codex pass 4 [P2]: async payment methods (SEPA, BACS, OXXO, etc.)
	// produce a checkout.session.completed with payment_status="unpaid"
	// BEFORE the underlying payment clears. Granting here would entitle
	// a user before they have paid, and a later async_payment_failed
	// would orphan that grant because we have no per-subscription
	// revocation path (only per-customer). The fulfillment signal for
	// async payments is checkout.session.async_payment_succeeded, which
	// the webhook dispatcher routes back through this same handler with
	// payment_status="paid". Return nil (no retry) — Stripe will deliver
	// the success event when funds clear.
	if !isPaymentCleared(sess) {
		log.Printf("stripe: deferring checkout %s (payment_status=%q): awaiting async payment clearance", event.ID, sess.PaymentStatus)
		return nil
	}
	log.Printf("stripe: granting license api_key=%s customer=%s event_created=%d", redactKey(sess.APIKey), sess.CustomerID, event.Created)
	// event.Created lets the store reject delayed/out-of-order events —
	// see LicenseStore.Grant. A canceled subscription whose delayed
	// checkout.completed shows up later would otherwise regrant access
	// (Codex pass 3 [P2]).
	return h.store.Grant(ctx, sess.APIKey, sess.Email, sess.CustomerID, event.Created)
}

func (h *licenseStripeHandler) OnSubscriptionDeleted(ctx context.Context, event stripe.Event) error {
	if h.store == nil {
		return errors.New("license store is nil")
	}
	customerID, err := parseSubscriptionCustomer(event)
	if err != nil {
		return fmt.Errorf("parse subscription deletion %s: %w", event.ID, err)
	}
	if customerID == "" {
		return fmt.Errorf("subscription deletion %s missing customer id", event.ID)
	}
	log.Printf("stripe: revoking license customer=%s event_created=%d", customerID, event.Created)
	return h.store.Revoke(ctx, customerID, event.Created)
}

// checkoutSessionMinimal is a minimal projection of CheckoutSession used
// purely so we can JSON-decode the bits we need without depending on
// every nested type in the stripe-go SDK. The SDK's CheckoutSession would
// also work, but it pulls in many transitive types whose JSON shapes can
// shift between API versions — a flat struct of strings is the most
// version-tolerant option.
type checkoutSessionMinimal struct {
	ID                string `json:"id"`
	ClientReferenceID string `json:"client_reference_id"`
	CustomerEmail     string `json:"customer_email"`
	Mode              string `json:"mode"`           // payment | setup | subscription
	PaymentStatus     string `json:"payment_status"` // paid | unpaid | no_payment_required
	// Customer and Subscription arrive either as a string (the ID) or
	// an object with an id field, depending on whether the event was
	// expanded. We decode to json.RawMessage and resolve below.
	Customer        json.RawMessage `json:"customer"`
	Subscription    json.RawMessage `json:"subscription"`
	CustomerDetails struct {
		Email string `json:"email"`
	} `json:"customer_details"`
}

type subscriptionMinimal struct {
	ID       string          `json:"id"`
	Customer json.RawMessage `json:"customer"`
}

// parsedSession is the resolved view of a checkout.session.completed event.
type parsedSession struct {
	APIKey         string // client_reference_id
	Email          string
	CustomerID     string
	Mode           string // "subscription" expected
	SubscriptionID string
	// PaymentStatus is the Stripe payment_status field. For card-only
	// subscriptions this is "paid" immediately on completion. For async
	// payment methods (SEPA debit, BACS, OXXO, etc.) Stripe fires
	// checkout.session.completed with payment_status="unpaid" first, then
	// emits checkout.session.async_payment_succeeded or
	// async_payment_failed later. We MUST only grant on cleared payments.
	PaymentStatus string
}

// parseCheckoutSession decodes the checkout session event into a flat
// parsedSession the dispatcher can act on.
func parseCheckoutSession(event stripe.Event) (parsedSession, error) {
	var sess checkoutSessionMinimal
	if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
		return parsedSession{}, fmt.Errorf("unmarshal session: %w", err)
	}
	email := sess.CustomerDetails.Email
	if email == "" {
		email = sess.CustomerEmail
	}
	customerID, err := decodeIDField(sess.Customer)
	if err != nil {
		return parsedSession{}, fmt.Errorf("customer: %w", err)
	}
	subscriptionID, err := decodeIDField(sess.Subscription)
	if err != nil {
		return parsedSession{}, fmt.Errorf("subscription: %w", err)
	}
	return parsedSession{
		APIKey:         sess.ClientReferenceID,
		Email:          email,
		CustomerID:     customerID,
		Mode:           sess.Mode,
		SubscriptionID: subscriptionID,
		PaymentStatus:  sess.PaymentStatus,
	}, nil
}

// isPaymentCleared returns true when the parsed checkout session's
// payment_status is consistent with a fulfilled payment. Stripe sets:
//
//   - "paid"                — the standard cleared case for card
//                              subscriptions (synchronous capture).
//   - "no_payment_required" — used for trials and 100%-discount coupons
//                              where Stripe still considers the session
//                              valid for fulfillment.
//   - "unpaid"              — async payment method (SEPA / BACS / OXXO /
//                              etc.) has been initiated but the payment
//                              has not cleared. We MUST NOT grant on this;
//                              the fulfillment signal arrives later in
//                              checkout.session.async_payment_succeeded.
//
// Empty / absent payment_status is treated as cleared for backwards
// compatibility. Stripe started emitting payment_status reliably long
// before this code was written, so absence is overwhelmingly a test
// fixture or an older account API version — and the upstream account
// configuration controls whether async payment methods can show up at
// all. Defaulting absent → cleared keeps existing card-only flows
// working without forcing every fixture to set the field.
func isPaymentCleared(sess parsedSession) bool {
	switch sess.PaymentStatus {
	case "", "paid", "no_payment_required":
		return true
	default:
		return false
	}
}

// isSubscriptionCheckout returns true when the parsed session looks like
// a subscription checkout we should grant on. Stripe sets Mode to
// "subscription" on subscription sessions, and a non-empty SubscriptionID
// is a stronger signal still. Either condition is enough; both together
// is the typical production case.
func isSubscriptionCheckout(sess parsedSession) bool {
	return sess.Mode == "subscription" || sess.SubscriptionID != ""
}

// parseSubscriptionCustomer pulls the Stripe customer ID out of a
// customer.subscription.deleted event.
func parseSubscriptionCustomer(event stripe.Event) (string, error) {
	var sub subscriptionMinimal
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return "", fmt.Errorf("unmarshal subscription: %w", err)
	}
	return decodeIDField(sub.Customer)
}

// decodeIDField handles the two wire shapes Stripe uses for object refs
// on events: a bare string ID ("cus_abc" / "sub_abc"), or an embedded
// object ({"id":"cus_abc",...}).
func decodeIDField(raw json.RawMessage) (string, error) {
	raw = bytesTrim(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	// Bare string case: "cus_abc"
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("unmarshal customer string: %w", err)
		}
		return s, nil
	}
	// Object case: {"id":"cus_abc", ...}
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("unmarshal customer object: %w", err)
	}
	return obj.ID, nil
}

// bytesTrim is the byte equivalent of strings.TrimSpace for json.RawMessage.
func bytesTrim(b []byte) []byte {
	start, end := 0, len(b)
	for start < end {
		c := b[start]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		start++
	}
	for end > start {
		c := b[end-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		end--
	}
	return b[start:end]
}

// redactKey returns the apiKey with all but the last 4 characters replaced
// by '*' for log lines.
func redactKey(k string) string {
	if len(k) <= 4 {
		return "****"
	}
	return "****" + k[len(k)-4:]
}

// NewLicenseStripeHandler returns the production StripeEventHandler that
// translates checkout / subscription events into LicenseStore updates.
func NewLicenseStripeHandler(s store.LicenseStore) StripeEventHandler {
	return &licenseStripeHandler{store: s}
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
	case "checkout.session.async_payment_succeeded":
		// Async payment methods (SEPA, BACS, OXXO, etc.) clear out of band.
		// When the funds arrive, Stripe re-delivers the session shape with
		// payment_status="paid" under this event type. The grant logic is
		// identical to checkout.session.completed for cleared payments —
		// route through the same handler so the path stays uniform and
		// the existing parsing / customer / event-ordering checks all apply.
		log.Printf("stripe: dispatching checkout.session.async_payment_succeeded id=%s", event.ID)
		if err := handler.OnCheckoutComplete(r.Context(), event); err != nil {
			log.Printf("stripe: OnCheckoutComplete (async_payment_succeeded): %v", err)
			http.Error(w, "handler error", http.StatusInternalServerError)
			return
		}
	case "checkout.session.async_payment_failed":
		// The async payment did not clear. The earlier
		// checkout.session.completed for the same session would have been
		// deferred (payment_status="unpaid" → no grant), so there is
		// nothing to revoke. Log for visibility and acknowledge the event
		// so Stripe stops retrying.
		log.Printf("stripe: acknowledging checkout.session.async_payment_failed id=%s", event.ID)
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
