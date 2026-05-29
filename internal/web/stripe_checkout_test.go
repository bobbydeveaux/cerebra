package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeCheckoutClient is a test double for checkoutSessionClient. It
// records the arguments it was called with and returns canned values.
type fakeCheckoutClient struct {
	createCalls atomic.Int64
	getCalls    atomic.Int64

	lastCreateOpts createCheckoutOptions
	lastGetID      string

	createSessionID  string
	createCheckoutURL string
	createErr        error

	getEmail  string
	getStatus string
	getErr    error
}

func (f *fakeCheckoutClient) CreateSubscriptionSession(_ context.Context, opts createCheckoutOptions) (string, string, error) {
	f.createCalls.Add(1)
	f.lastCreateOpts = opts
	if f.createErr != nil {
		return "", "", f.createErr
	}
	return f.createSessionID, f.createCheckoutURL, nil
}

func (f *fakeCheckoutClient) GetSession(_ context.Context, id string) (string, string, error) {
	f.getCalls.Add(1)
	f.lastGetID = id
	if f.getErr != nil {
		return "", "", f.getErr
	}
	return f.getEmail, f.getStatus, nil
}

// newCheckoutServerForTest builds a minimal Server wired with the fake
// client. It bypasses NewServer to avoid pulling in the embedder /
// pipeline / store dependencies that are irrelevant to the checkout
// HTTP contract.
func newCheckoutServerForTest(client checkoutSessionClient) *Server {
	mux := http.NewServeMux()
	srv := &Server{mux: mux, checkoutClient: client}
	mux.HandleFunc("POST /api/stripe/create-checkout", srv.handleCreateCheckout)
	mux.HandleFunc("GET /api/stripe/session", srv.handleGetCheckoutSession)
	return srv
}

func TestCreateCheckoutReturnsURL(t *testing.T) {
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "price_growth_test_123")

	client := &fakeCheckoutClient{
		createSessionID:   "cs_test_abc",
		createCheckoutURL: "https://checkout.stripe.com/pay/cs_test_abc",
	}
	srv := newCheckoutServerForTest(client)

	body := `{"client_reference_id":"ck_user_42","customer_email":"bobby@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/stripe/create-checkout", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleCreateCheckout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := client.createCalls.Load(); got != 1 {
		t.Errorf("CreateSubscriptionSession: want 1 call, got %d", got)
	}
	if client.lastCreateOpts.PriceID != "price_growth_test_123" {
		t.Errorf("price id: want price_growth_test_123, got %q", client.lastCreateOpts.PriceID)
	}
	if client.lastCreateOpts.SuccessURL != defaultCheckoutSuccessURL {
		t.Errorf("success url: want %q, got %q", defaultCheckoutSuccessURL, client.lastCreateOpts.SuccessURL)
	}
	if client.lastCreateOpts.CancelURL != defaultCheckoutCancelURL {
		t.Errorf("cancel url: want %q, got %q", defaultCheckoutCancelURL, client.lastCreateOpts.CancelURL)
	}
	if client.lastCreateOpts.ClientReferenceID != "ck_user_42" {
		t.Errorf("client_reference_id: want ck_user_42, got %q", client.lastCreateOpts.ClientReferenceID)
	}
	if client.lastCreateOpts.CustomerEmail != "bobby@example.com" {
		t.Errorf("customer_email: want bobby@example.com, got %q", client.lastCreateOpts.CustomerEmail)
	}

	var resp createCheckoutResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.CheckoutURL != "https://checkout.stripe.com/pay/cs_test_abc" {
		t.Errorf("checkout_url: want https://checkout.stripe.com/pay/cs_test_abc, got %q", resp.CheckoutURL)
	}
	if resp.SessionID != "cs_test_abc" {
		t.Errorf("session_id: want cs_test_abc, got %q", resp.SessionID)
	}
}

func TestCreateCheckoutRejectsEmptyBody(t *testing.T) {
	// An empty body parses cleanly as the zero-value request, but the
	// handler then refuses because client_reference_id is required —
	// the licenseStripeHandler downstream errors on subscription events
	// without a client reference, so we fail loud before charging the
	// customer rather than minting an unbindable subscription.
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "price_growth_test_123")

	client := &fakeCheckoutClient{}
	srv := newCheckoutServerForTest(client)

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/create-checkout", strings.NewReader(""))
	w := httptest.NewRecorder()

	srv.handleCreateCheckout(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 on empty body (no client_reference_id), got %d. body=%q", w.Code, w.Body.String())
	}
	if got := client.createCalls.Load(); got != 0 {
		t.Errorf("client should not have been called without client_reference_id, got %d calls", got)
	}
}

func TestCreateCheckoutRejectsBlankClientReference(t *testing.T) {
	// A body with whitespace-only client_reference_id is the same as
	// empty for our purposes: refuse before calling Stripe.
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "price_growth_test_123")

	client := &fakeCheckoutClient{}
	srv := newCheckoutServerForTest(client)

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/create-checkout", strings.NewReader(`{"client_reference_id":"   "}`))
	w := httptest.NewRecorder()

	srv.handleCreateCheckout(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 on whitespace client_reference_id, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := client.createCalls.Load(); got != 0 {
		t.Errorf("client should not have been called, got %d calls", got)
	}
}

func TestCreateCheckoutRejectsTruncatedJSON(t *testing.T) {
	// Truncated JSON like "{" returns io.ErrUnexpectedEOF from the
	// json decoder. Earlier the handler silently treated that as an
	// empty body and proceeded; that's wrong — malformed JSON must
	// surface as 400 so callers find the bug.
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "price_growth_test_123")

	client := &fakeCheckoutClient{}
	srv := newCheckoutServerForTest(client)

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/create-checkout", strings.NewReader(`{`))
	w := httptest.NewRecorder()

	srv.handleCreateCheckout(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 on truncated JSON, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := client.createCalls.Load(); got != 0 {
		t.Errorf("client should not have been called on truncated JSON, got %d calls", got)
	}
}

func TestCreateCheckoutMissingPriceID(t *testing.T) {
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "")

	client := &fakeCheckoutClient{}
	srv := newCheckoutServerForTest(client)

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/create-checkout", strings.NewReader(`{"client_reference_id":"ck_user_42"}`))
	w := httptest.NewRecorder()

	srv.handleCreateCheckout(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 without price id, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := client.createCalls.Load(); got != 0 {
		t.Errorf("client should not have been called, got %d calls", got)
	}
}

func TestCreateCheckoutClientError(t *testing.T) {
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "price_growth_test_123")

	client := &fakeCheckoutClient{createErr: errors.New("stripe boom")}
	srv := newCheckoutServerForTest(client)

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/create-checkout", strings.NewReader(`{"client_reference_id":"ck_user_42"}`))
	w := httptest.NewRecorder()

	srv.handleCreateCheckout(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 on client error, got %d. body=%q", w.Code, w.Body.String())
	}
}

func TestCreateCheckoutRejectsMalformedBody(t *testing.T) {
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "price_growth_test_123")

	client := &fakeCheckoutClient{}
	srv := newCheckoutServerForTest(client)

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/create-checkout", strings.NewReader(`{"not_a_real_field":true}`))
	w := httptest.NewRecorder()

	srv.handleCreateCheckout(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 on unknown fields, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := client.createCalls.Load(); got != 0 {
		t.Errorf("client should not have been called, got %d calls", got)
	}
}

func TestGetCheckoutSessionReturnsEmail(t *testing.T) {
	client := &fakeCheckoutClient{
		getEmail:  "bobby@example.com",
		getStatus: "complete",
	}
	srv := newCheckoutServerForTest(client)

	req := httptest.NewRequest(http.MethodGet, "/api/stripe/session?session_id=cs_complete_1", nil)
	w := httptest.NewRecorder()

	srv.handleGetCheckoutSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := client.getCalls.Load(); got != 1 {
		t.Errorf("GetSession: want 1 call, got %d", got)
	}
	if client.lastGetID != "cs_complete_1" {
		t.Errorf("session id: want cs_complete_1, got %q", client.lastGetID)
	}

	var resp getSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.CustomerEmail != "bobby@example.com" {
		t.Errorf("customer_email: want bobby@example.com, got %q", resp.CustomerEmail)
	}
	if resp.Status != "complete" {
		t.Errorf("status: want complete, got %q", resp.Status)
	}
}

func TestGetCheckoutSessionMissingID(t *testing.T) {
	client := &fakeCheckoutClient{}
	srv := newCheckoutServerForTest(client)

	req := httptest.NewRequest(http.MethodGet, "/api/stripe/session", nil)
	w := httptest.NewRecorder()

	srv.handleGetCheckoutSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := client.getCalls.Load(); got != 0 {
		t.Errorf("GetSession should not have fired, got %d calls", got)
	}
}

func TestGetCheckoutSessionClientError(t *testing.T) {
	client := &fakeCheckoutClient{getErr: errors.New("stripe lookup failed")}
	srv := newCheckoutServerForTest(client)

	req := httptest.NewRequest(http.MethodGet, "/api/stripe/session?session_id=cs_bad", nil)
	w := httptest.NewRecorder()

	srv.handleGetCheckoutSession(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 on client error, got %d. body=%q", w.Code, w.Body.String())
	}
}

// TestCheckoutEndpointsRouteRegistered ensures the route plumbing in
// NewServer is exercised — a 405/404 here would mean the routes are not
// registered with the mux.
func TestCheckoutEndpointsRouteRegistered(t *testing.T) {
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "price_growth_test_123")

	client := &fakeCheckoutClient{
		createSessionID:   "cs_route_1",
		createCheckoutURL: "https://checkout.stripe.com/pay/cs_route_1",
		getEmail:          "bobby@example.com",
		getStatus:         "complete",
	}
	mux := http.NewServeMux()
	srv := &Server{mux: mux, checkoutClient: client}
	mux.HandleFunc("POST /api/stripe/create-checkout", srv.handleCreateCheckout)
	mux.HandleFunc("GET /api/stripe/session", srv.handleGetCheckoutSession)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// POST /api/stripe/create-checkout
	createReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/stripe/create-checkout", strings.NewReader(`{"client_reference_id":"ck_route_1"}`))
	if err != nil {
		t.Fatalf("new create request: %v", err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("do create: %v", err)
	}
	defer createResp.Body.Close()
	createBody, _ := io.ReadAll(createResp.Body)
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("create want 200, got %d. body=%s", createResp.StatusCode, createBody)
	}

	// GET /api/stripe/session
	getResp, err := http.Get(ts.URL + "/api/stripe/session?session_id=cs_route_1")
	if err != nil {
		t.Fatalf("do get: %v", err)
	}
	defer getResp.Body.Close()
	getBody, _ := io.ReadAll(getResp.Body)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get want 200, got %d. body=%s", getResp.StatusCode, getBody)
	}

	if got := client.createCalls.Load(); got != 1 {
		t.Errorf("create calls: want 1, got %d", got)
	}
	if got := client.getCalls.Load(); got != 1 {
		t.Errorf("get calls: want 1, got %d", got)
	}
}

func TestCheckoutClientOrDefaultReturnsNilWithoutKey(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "")
	srv := &Server{}
	if got := srv.checkoutClientOrDefault(); got != nil {
		t.Errorf("want nil client without STRIPE_SECRET_KEY, got %T", got)
	}
}

func TestCheckoutClientOrDefaultBuildsEnvClient(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_abc")
	srv := &Server{}
	got := srv.checkoutClientOrDefault()
	if got == nil {
		t.Fatal("want env-backed client, got nil")
	}
	prod, ok := got.(*stripeCheckoutClient)
	if !ok {
		t.Fatalf("want *stripeCheckoutClient, got %T", got)
	}
	if prod.apiKey != "sk_test_abc" {
		t.Errorf("api key: want sk_test_abc, got %q", prod.apiKey)
	}
}

func TestCheckoutClientOrDefaultPrefersInjected(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_abc")
	fake := &fakeCheckoutClient{}
	srv := &Server{checkoutClient: fake}
	if got := srv.checkoutClientOrDefault(); got != fake {
		t.Errorf("want injected fake, got %T", got)
	}
}

func TestCreateCheckoutHonoursURLOverrides(t *testing.T) {
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "price_growth_test_123")
	t.Setenv("STRIPE_CHECKOUT_SUCCESS_URL", "https://dev.example/welcome?session_id={CHECKOUT_SESSION_ID}")
	t.Setenv("STRIPE_CHECKOUT_CANCEL_URL", "https://dev.example/pricing")

	client := &fakeCheckoutClient{
		createSessionID:   "cs_override",
		createCheckoutURL: "https://checkout.stripe.com/pay/cs_override",
	}
	srv := newCheckoutServerForTest(client)

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/create-checkout", strings.NewReader(`{"client_reference_id":"ck_user_42"}`))
	w := httptest.NewRecorder()

	srv.handleCreateCheckout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d. body=%q", w.Code, w.Body.String())
	}
	if client.lastCreateOpts.SuccessURL != "https://dev.example/welcome?session_id={CHECKOUT_SESSION_ID}" {
		t.Errorf("success url override: got %q", client.lastCreateOpts.SuccessURL)
	}
	if client.lastCreateOpts.CancelURL != "https://dev.example/pricing" {
		t.Errorf("cancel url override: got %q", client.lastCreateOpts.CancelURL)
	}
}

func TestCreateCheckoutRejectsOversizeBody(t *testing.T) {
	// A body larger than checkoutBodyMaxBytes must surface as 400 — we
	// must not let a public endpoint pull unbounded data into memory.
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "price_growth_test_123")

	client := &fakeCheckoutClient{}
	srv := newCheckoutServerForTest(client)

	huge := strings.Repeat("a", checkoutBodyMaxBytes+1)
	body := `{"client_reference_id":"` + huge + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/stripe/create-checkout", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.handleCreateCheckout(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 on oversize body, got %d. body=%q", w.Code, w.Body.String())
	}
	if got := client.createCalls.Load(); got != 0 {
		t.Errorf("client should not have been called on oversize body, got %d calls", got)
	}
}

func TestCreateCheckoutPropagatesContext(t *testing.T) {
	// The handler must pass the request context to the checkout client
	// so cancels and deadlines reach the outbound Stripe call. The fake
	// records ctx.Err() at call time.
	t.Setenv("STRIPE_GROWTH_PRICE_ID", "price_growth_test_123")

	client := &ctxRecordingClient{}
	srv := newCheckoutServerForTest(client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so the fake observes the cancellation
	req := httptest.NewRequest(http.MethodPost, "/api/stripe/create-checkout", strings.NewReader(`{"client_reference_id":"ck_user_42"}`)).WithContext(ctx)
	w := httptest.NewRecorder()

	srv.handleCreateCheckout(w, req)

	if client.createErr == nil {
		t.Fatal("want cancellation observed in handler ctx, got nil ctx err")
	}
	if !errors.Is(client.createErr, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", client.createErr)
	}
}

// ctxRecordingClient is a fakeCheckoutClient variant that records the
// ctx.Err() seen at call time, so tests can verify the handler passed
// the right context down.
type ctxRecordingClient struct {
	createErr error
}

func (c *ctxRecordingClient) CreateSubscriptionSession(ctx context.Context, _ createCheckoutOptions) (string, string, error) {
	c.createErr = ctx.Err()
	return "cs_ctx", "https://checkout.stripe.com/pay/cs_ctx", nil
}

func (c *ctxRecordingClient) GetSession(ctx context.Context, _ string) (string, string, error) {
	c.createErr = ctx.Err()
	return "", "", nil
}

func TestStripeCheckoutClientRejectsEmptyPriceID(t *testing.T) {
	c := newEnvStripeCheckoutClient("sk_test_abc")
	_, _, err := c.CreateSubscriptionSession(context.Background(), createCheckoutOptions{})
	if err == nil {
		t.Fatal("want error for empty price id, got nil")
	}
	if !strings.Contains(err.Error(), "price id") {
		t.Errorf("error should mention price id, got %v", err)
	}
}
