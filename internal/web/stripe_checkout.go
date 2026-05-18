// Package web — Stripe Checkout Session create + retrieve endpoints
// (agentops-013).
//
// Two HTTP endpoints make up the front half of the Cerebra paid funnel:
//
//   - POST /api/stripe/create-checkout
//     Starts a subscription Checkout Session for the Growth tier and
//     returns the hosted checkout URL the browser should redirect to.
//
//   - GET /api/stripe/session?session_id=cs_...
//     Retrieves a completed (or open) session and returns the customer
//     email + status. Used by /welcome to issue an API key.
//
// The actual Stripe SDK call is hidden behind a checkoutSessionClient
// interface (declared here) so tests can swap a fake in without touching
// env vars or hitting the network. Production wiring lazy-loads
// STRIPE_SECRET_KEY at first use via newEnvStripeCheckoutClient().
package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	stripe "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/checkout/session"
)

// checkoutSessionClient is the seam between the HTTP layer and stripe-go.
// Only the methods Cerebra actually needs are exposed; the SDK's full
// CheckoutSession surface stays hidden so tests can stub it cheaply.
type checkoutSessionClient interface {
	CreateSubscriptionSession(ctx context.Context, opts createCheckoutOptions) (sessionID, checkoutURL string, err error)
	GetSession(ctx context.Context, id string) (customerEmail string, status string, err error)
}

// createCheckoutOptions is the set of inputs the HTTP layer passes to the
// checkoutSessionClient. Kept as a plain struct so swapping the SDK out
// later (e.g. for a managed Stripe-on-Cloud-Run variant) does not ripple
// through every caller.
type createCheckoutOptions struct {
	PriceID           string // Stripe price ID (STRIPE_GROWTH_PRICE_ID)
	SuccessURL        string
	CancelURL         string
	ClientReferenceID string // the Cerebra API key the customer is paying for
	CustomerEmail     string // optional pre-fill
}

const (
	// successURL is the page customers land on after a successful payment.
	// {CHECKOUT_SESSION_ID} is a Stripe template variable expanded by
	// Stripe before redirect.
	defaultCheckoutSuccessURL = "https://cerebra.stackramp.io/welcome?session_id={CHECKOUT_SESSION_ID}"
	defaultCheckoutCancelURL  = "https://cerebra.stackramp.io/pricing"
)

// createCheckoutRequest is the optional JSON body accepted by
// POST /api/stripe/create-checkout. All fields are optional — the handler
// still creates a session if the body is empty (Stripe will prompt the
// customer for an email in that case).
type createCheckoutRequest struct {
	ClientReferenceID string `json:"client_reference_id"`
	CustomerEmail     string `json:"customer_email"`
}

// createCheckoutResponse is the wire-format response from the create
// endpoint. session_id is included so the caller can correlate the
// returned URL with the subsequent welcome-page lookup if needed.
type createCheckoutResponse struct {
	CheckoutURL string `json:"checkout_url"`
	SessionID   string `json:"session_id"`
}

// getSessionResponse is the wire-format response from the retrieve
// endpoint.
type getSessionResponse struct {
	CustomerEmail string `json:"customer_email"`
	Status        string `json:"status"`
}

// handleCreateCheckout is POST /api/stripe/create-checkout.
//
// It reads STRIPE_GROWTH_PRICE_ID from the environment, builds a
// subscription-mode Checkout Session via the configured
// checkoutSessionClient, and returns the hosted URL. Errors are
// translated into 4xx for client mistakes (missing price id is a 500
// because that's an operator misconfiguration, not a caller bug) and
// 500 for upstream Stripe failures.
func (s *Server) handleCreateCheckout(w http.ResponseWriter, r *http.Request) {
	priceID := strings.TrimSpace(os.Getenv("STRIPE_GROWTH_PRICE_ID"))
	if priceID == "" {
		log.Printf("stripe: STRIPE_GROWTH_PRICE_ID is not set")
		http.Error(w, "growth price id not configured", http.StatusInternalServerError)
		return
	}

	// Body is optional. An unreadable / malformed body is the caller's
	// fault (400); an empty body is fine (handled below).
	var req createCheckoutRequest
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil && !errors.Is(err, ErrEmptyBody) && !isEmptyJSONBody(err) {
			log.Printf("stripe: create-checkout decode body: %v", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}

	client := s.checkoutClientOrDefault()
	if client == nil {
		log.Printf("stripe: checkout client is nil (STRIPE_SECRET_KEY missing?)")
		http.Error(w, "stripe checkout client not configured", http.StatusInternalServerError)
		return
	}

	sessID, url, err := client.CreateSubscriptionSession(r.Context(), createCheckoutOptions{
		PriceID:           priceID,
		SuccessURL:        defaultCheckoutSuccessURL,
		CancelURL:         defaultCheckoutCancelURL,
		ClientReferenceID: strings.TrimSpace(req.ClientReferenceID),
		CustomerEmail:     strings.TrimSpace(req.CustomerEmail),
	})
	if err != nil {
		log.Printf("stripe: create checkout session: %v", err)
		http.Error(w, "could not create checkout session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(createCheckoutResponse{
		CheckoutURL: url,
		SessionID:   sessID,
	})
}

// handleGetCheckoutSession is GET /api/stripe/session?session_id=cs_...
//
// Returns the customer email and session status. The welcome page calls
// this after Stripe redirects with the session_id query parameter; the
// caller uses the email to mint or look up the customer's API key.
func (s *Server) handleGetCheckoutSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if id == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	client := s.checkoutClientOrDefault()
	if client == nil {
		log.Printf("stripe: checkout client is nil (STRIPE_SECRET_KEY missing?)")
		http.Error(w, "stripe checkout client not configured", http.StatusInternalServerError)
		return
	}

	email, status, err := client.GetSession(r.Context(), id)
	if err != nil {
		log.Printf("stripe: get checkout session %s: %v", id, err)
		http.Error(w, "could not retrieve checkout session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(getSessionResponse{
		CustomerEmail: email,
		Status:        status,
	})
}

// checkoutClientOrDefault returns the explicitly-wired checkoutClient if
// one was set via WithCheckoutClient, otherwise it lazily constructs the
// env-backed stripeCheckoutClient. This mirrors the late-binding pattern
// used by the LicenseStore wiring so tests can inject fakes without
// changing the production zero-value behaviour.
func (s *Server) checkoutClientOrDefault() checkoutSessionClient {
	if s.checkoutClient != nil {
		return s.checkoutClient
	}
	key := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	if key == "" {
		return nil
	}
	return newEnvStripeCheckoutClient(key)
}

// WithCheckoutClient is a test seam — passing a fake here disables the
// env-driven production client for the lifetime of the Server. Production
// callers should leave this unset and rely on STRIPE_SECRET_KEY.
func (s *Server) WithCheckoutClient(c checkoutSessionClient) *Server {
	s.checkoutClient = c
	return s
}

// stripeCheckoutClient is the production checkoutSessionClient. It calls
// the stripe-go SDK directly, configuring the per-call Key so credential
// rotation does not require a server restart.
type stripeCheckoutClient struct {
	apiKey string
}

func newEnvStripeCheckoutClient(apiKey string) *stripeCheckoutClient {
	return &stripeCheckoutClient{apiKey: apiKey}
}

func (c *stripeCheckoutClient) CreateSubscriptionSession(_ context.Context, opts createCheckoutOptions) (string, string, error) {
	if opts.PriceID == "" {
		return "", "", fmt.Errorf("price id is required")
	}
	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL: stripe.String(opts.SuccessURL),
		CancelURL:  stripe.String(opts.CancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(opts.PriceID),
				Quantity: stripe.Int64(1),
			},
		},
	}
	if opts.ClientReferenceID != "" {
		params.ClientReferenceID = stripe.String(opts.ClientReferenceID)
	}
	if opts.CustomerEmail != "" {
		params.CustomerEmail = stripe.String(opts.CustomerEmail)
	}
	client := session.Client{B: stripe.GetBackend(stripe.APIBackend), Key: c.apiKey}
	sess, err := client.New(params)
	if err != nil {
		return "", "", fmt.Errorf("stripe new session: %w", err)
	}
	return sess.ID, sess.URL, nil
}

func (c *stripeCheckoutClient) GetSession(_ context.Context, id string) (string, string, error) {
	client := session.Client{B: stripe.GetBackend(stripe.APIBackend), Key: c.apiKey}
	sess, err := client.Get(id, nil)
	if err != nil {
		return "", "", fmt.Errorf("stripe get session %s: %w", id, err)
	}
	email := ""
	if sess.CustomerDetails != nil {
		email = sess.CustomerDetails.Email
	}
	if email == "" {
		email = sess.CustomerEmail
	}
	return email, string(sess.Status), nil
}

// ErrEmptyBody is returned by callers when the request body is empty and
// should be treated as the zero-value request. Kept here so handlers in
// this package share the convention.
var ErrEmptyBody = errors.New("empty request body")

// isEmptyJSONBody returns true when a json.Decoder error is just EOF
// against an empty body — this is the case when the caller does not send
// a body at all, which we want to accept as the zero-value request.
func isEmptyJSONBody(err error) bool {
	if err == nil {
		return false
	}
	// io.EOF or io.ErrUnexpectedEOF both happen with empty bodies; the
	// json package wraps neither in a typed error, so a substring match
	// is the most portable check.
	msg := err.Error()
	return msg == "EOF" || msg == "unexpected EOF"
}
