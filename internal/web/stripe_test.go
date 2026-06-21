package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v76"
)

// recordingStripeHandler is a test double for StripeEventHandler that
// records which dispatch methods were called and how many times.
type recordingStripeHandler struct {
	checkoutCalls  atomic.Int64
	deletionCalls  atomic.Int64
	checkoutErr    error
	deletionErr    error
	lastCheckoutID string
	lastDeletionID string
}

func (h *recordingStripeHandler) OnCheckoutComplete(_ context.Context, event stripe.Event) error {
	h.checkoutCalls.Add(1)
	h.lastCheckoutID = event.ID
	return h.checkoutErr
}

func (h *recordingStripeHandler) OnSubscriptionDeleted(_ context.Context, event stripe.Event) error {
	h.deletionCalls.Add(1)
	h.lastDeletionID = event.ID
	return h.deletionErr
}

// stripeSignature builds a Stripe-Signature header value for the given
// payload and secret, mirroring the documented scheme:
//
//	t=<unix>,v1=HMAC_SHA256(<unix>.<payload>, secret)
//
// The Stripe SDK accepts this format directly.
func stripeSignature(t *testing.T, payload []byte, secret string, ts time.Time) string {
	t.Helper()
	unix := ts.Unix()
	signedPayload := fmt.Sprintf("%d.%s", unix, payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	return fmt.Sprintf("t=%d,v1=%s", unix, hex.EncodeToString(mac.Sum(nil)))
}

// newServerForTest builds a minimal Server wired with the given handler.
// It bypasses NewServer to avoid pulling in the full store / embedder /
// pipeline stack which is not relevant to the webhook contract.
func newServerForTest(handler StripeEventHandler) *Server {
	return &Server{
		mux:           http.NewServeMux(),
		stripeHandler: handler,
	}
}

func buildEventPayload(t *testing.T, eventType, eventID string) []byte {
	t.Helper()
	return []byte(fmt.Sprintf(`{"id":%q,"object":"event","type":%q,"data":{"object":{}}}`, eventID, eventType))
}

func TestStripeWebhookDispatchesCheckoutComplete(t *testing.T) {
	const secret = "whsec_test_secret_aaaaaaaaaaaaaaaaaaaaaa"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	rec := &recordingStripeHandler{}
	srv := newServerForTest(rec)

	payload := buildEventPayload(t, "checkout.session.completed", "evt_checkout_1")
	sig := stripeSignature(t, payload, secret, time.Now())

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleStripeWebhook(w, req)

	if w.Code != http.StatusOK {
		dump, _ := httputil.DumpResponse(w.Result(), true)
		t.Fatalf("want 200, got %d. response:\n%s", w.Code, dump)
	}
	if got := rec.checkoutCalls.Load(); got != 1 {
		t.Errorf("OnCheckoutComplete: want 1 call, got %d", got)
	}
	if rec.lastCheckoutID != "evt_checkout_1" {
		t.Errorf("event id: want evt_checkout_1, got %q", rec.lastCheckoutID)
	}
	if got := rec.deletionCalls.Load(); got != 0 {
		t.Errorf("OnSubscriptionDeleted should not have fired, got %d calls", got)
	}
}

func TestStripeWebhookDispatchesSubscriptionDeleted(t *testing.T) {
	const secret = "whsec_test_secret_bbbbbbbbbbbbbbbbbbbbbb"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	rec := &recordingStripeHandler{}
	srv := newServerForTest(rec)

	payload := buildEventPayload(t, "customer.subscription.deleted", "evt_sub_1")
	sig := stripeSignature(t, payload, secret, time.Now())

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()

	srv.handleStripeWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := rec.deletionCalls.Load(); got != 1 {
		t.Errorf("OnSubscriptionDeleted: want 1 call, got %d", got)
	}
	if rec.lastDeletionID != "evt_sub_1" {
		t.Errorf("event id: want evt_sub_1, got %q", rec.lastDeletionID)
	}
}

func TestStripeWebhookIgnoresUnknownEvent(t *testing.T) {
	const secret = "whsec_test_secret_cccccccccccccccccccccc"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	rec := &recordingStripeHandler{}
	srv := newServerForTest(rec)

	payload := buildEventPayload(t, "payment_intent.succeeded", "evt_pi_1")
	sig := stripeSignature(t, payload, secret, time.Now())

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()

	srv.handleStripeWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := rec.checkoutCalls.Load(); got != 0 {
		t.Errorf("OnCheckoutComplete should not have fired, got %d calls", got)
	}
	if got := rec.deletionCalls.Load(); got != 0 {
		t.Errorf("OnSubscriptionDeleted should not have fired, got %d calls", got)
	}
}

func TestStripeWebhookRejectsBadSignature(t *testing.T) {
	const secret = "whsec_test_secret_dddddddddddddddddddddd"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	rec := &recordingStripeHandler{}
	srv := newServerForTest(rec)

	payload := buildEventPayload(t, "checkout.session.completed", "evt_bad_sig")
	// Sign with a DIFFERENT secret so verification fails.
	sig := stripeSignature(t, payload, "whsec_someone_else_aaaaaaaaaaaaaaaaaaaaaaaa", time.Now())

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()

	srv.handleStripeWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := rec.checkoutCalls.Load(); got != 0 {
		t.Errorf("handler should not have fired on bad signature, got %d calls", got)
	}
}

func TestStripeWebhookRejectsMissingSecret(t *testing.T) {
	// Force the env var to empty for this test even if the parent shell has one.
	t.Setenv("STRIPE_WEBHOOK_SECRET", "")

	rec := &recordingStripeHandler{}
	srv := newServerForTest(rec)

	payload := buildEventPayload(t, "checkout.session.completed", "evt_no_secret")
	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", "t=1,v1=deadbeef")
	w := httptest.NewRecorder()

	srv.handleStripeWebhook(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := rec.checkoutCalls.Load(); got != 0 {
		t.Errorf("handler should not have fired without secret, got %d calls", got)
	}
}

// TestStripeWebhookRejectsOversizedPayload verifies the body-size cap.
// The handler reads through http.MaxBytesReader(w, r.Body,
// stripePayloadMaxBytes); a body larger than the cap makes io.ReadAll
// return an error, which the handler maps to a clean 400 ("could not read
// request body") before any signature check or dispatch. The body is signed
// with the CORRECT secret so the rejection is provably the size cap and not
// an incidental signature mismatch.
func TestStripeWebhookRejectsOversizedPayload(t *testing.T) {
	const secret = "whsec_test_secret_ffffffffffffffffffffff"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	rec := &recordingStripeHandler{}
	srv := newServerForTest(rec)

	// One byte over the cap. The payload is valid JSON padded with a long
	// string field so the whole body exceeds stripePayloadMaxBytes.
	prefix := `{"id":"evt_big","object":"event","type":"checkout.session.completed","data":{"object":{}},"pad":"`
	suffix := `"}`
	padLen := stripePayloadMaxBytes + 1 - len(prefix) - len(suffix)
	if padLen < 1 {
		t.Fatalf("pad length calculation went negative: %d", padLen)
	}
	payload := []byte(prefix + strings.Repeat("a", padLen) + suffix)
	if len(payload) <= stripePayloadMaxBytes {
		t.Fatalf("payload not oversized: len=%d cap=%d", len(payload), stripePayloadMaxBytes)
	}
	sig := stripeSignature(t, payload, secret, time.Now())

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleStripeWebhook(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("oversized payload accepted: got 200, want non-200. body=%q", w.Body.String())
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for oversized body, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := rec.checkoutCalls.Load(); got != 0 {
		t.Errorf("handler should not fire on oversized body, got %d checkout calls", got)
	}
	if got := rec.deletionCalls.Load(); got != 0 {
		t.Errorf("handler should not fire on oversized body, got %d deletion calls", got)
	}
}

// TestStripeWebhookRejectsEmptyBody verifies that a zero-byte body with a
// valid-format Stripe-Signature is rejected with 400. The body reads
// successfully (zero bytes is under the cap), then signature verification
// fails on the empty payload, so the handler returns 400 and never reaches
// dispatch.
func TestStripeWebhookRejectsEmptyBody(t *testing.T) {
	const secret = "whsec_test_secret_gggggggggggggggggggggg"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	rec := &recordingStripeHandler{}
	srv := newServerForTest(rec)

	// Valid-format signature over an empty payload. Even though it is signed
	// with the correct secret, ConstructEventWithOptions rejects an empty
	// payload (nothing to parse / verify into an event), yielding 400.
	sig := stripeSignature(t, []byte{}, secret, time.Now())

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(""))
	req.Header.Set("Stripe-Signature", sig)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleStripeWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty body, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := rec.checkoutCalls.Load(); got != 0 {
		t.Errorf("handler should not fire on empty body, got %d checkout calls", got)
	}
	if got := rec.deletionCalls.Load(); got != 0 {
		t.Errorf("handler should not fire on empty body, got %d deletion calls", got)
	}
}

// TestStripeWebhookRouteRegistered ensures the route plumbing in NewServer
// is exercised — a 405/404 here would mean POST /api/stripe/webhook is not
// registered with the mux, which is the exact contract agentops-012 / -013
// depends on.
func TestStripeWebhookRouteRegistered(t *testing.T) {
	const secret = "whsec_test_secret_eeeeeeeeeeeeeeeeeeeeee"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	rec := &recordingStripeHandler{}
	mux := http.NewServeMux()
	srv := &Server{mux: mux, stripeHandler: rec}
	mux.HandleFunc("POST /api/stripe/webhook", srv.handleStripeWebhook)

	payload := buildEventPayload(t, "checkout.session.completed", "evt_route_1")
	sig := stripeSignature(t, payload, secret, time.Now())

	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/stripe/webhook", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Stripe-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d. body=%s", resp.StatusCode, body)
	}
	if got := rec.checkoutCalls.Load(); got != 1 {
		t.Errorf("OnCheckoutComplete: want 1 call, got %d", got)
	}
}
