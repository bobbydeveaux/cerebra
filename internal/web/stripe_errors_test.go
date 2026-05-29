package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestStripeWebhookCheckoutHandlerError covers the 500 branch when the
// configured StripeEventHandler.OnCheckoutComplete returns an error —
// Stripe must see a 500 so it retries.
func TestStripeWebhookCheckoutHandlerError(t *testing.T) {
	const secret = "whsec_test_secret_ffffffffffffffffffffff"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	rec := &recordingStripeHandler{checkoutErr: errors.New("downstream-down")}
	srv := newServerForTest(rec)

	payload := buildEventPayload(t, "checkout.session.completed", "evt_checkout_err")
	sig := stripeSignature(t, payload, secret, time.Now())

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()

	srv.handleStripeWebhook(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 (so Stripe retries), got %d. body=%q", w.Code, w.Body.String())
	}
}

// TestStripeWebhookDeletionHandlerError mirrors the above for the
// customer.subscription.deleted branch.
func TestStripeWebhookDeletionHandlerError(t *testing.T) {
	const secret = "whsec_test_secret_gggggggggggggggggggggg"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	rec := &recordingStripeHandler{deletionErr: errors.New("downstream-down")}
	srv := newServerForTest(rec)

	payload := buildEventPayload(t, "customer.subscription.deleted", "evt_sub_err")
	sig := stripeSignature(t, payload, secret, time.Now())

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()

	srv.handleStripeWebhook(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d. body=%q", w.Code, w.Body.String())
	}
}

// TestStripeWebhookNilHandlerFallsBackToLoggingStub covers the path where
// Server.stripeHandler is nil — handleStripeWebhook substitutes a
// loggingStripeHandler so the webhook still 200s instead of NPEing.
func TestStripeWebhookNilHandlerFallsBackToLoggingStub(t *testing.T) {
	const secret = "whsec_test_secret_hhhhhhhhhhhhhhhhhhhhhh"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	srv := &Server{mux: http.NewServeMux()} // stripeHandler intentionally nil

	payload := buildEventPayload(t, "checkout.session.completed", "evt_nilh_1")
	sig := stripeSignature(t, payload, secret, time.Now())

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()

	srv.handleStripeWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d. body=%q", w.Code, w.Body.String())
	}
}
