package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/store"
	stripe "github.com/stripe/stripe-go/v76"
)

// stripeEventEnvelope is the bit of the wire format we synthesize for tests.
// stripe.Event has stripe.EventData.Raw as the source of truth that
// webhook.ConstructEvent unmarshals; the type field gates the switch.
func buildEventEnvelope(t *testing.T, eventType, eventID string, object map[string]any) []byte {
	t.Helper()
	objBytes, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("marshal object: %v", err)
	}
	env := fmt.Sprintf(
		`{"id":%q,"object":"event","type":%q,"data":{"object":%s}}`,
		eventID, eventType, string(objBytes),
	)
	return []byte(env)
}

func sign(t *testing.T, payload []byte, secret string) string {
	t.Helper()
	unix := time.Now().Unix()
	signed := fmt.Sprintf("%d.%s", unix, payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	return fmt.Sprintf("t=%d,v1=%s", unix, hex.EncodeToString(mac.Sum(nil)))
}

func TestLicenseStripeHandler_CheckoutComplete_GrantsLicense(t *testing.T) {
	ls := store.NewMemoryLicenseStore()
	h := NewLicenseStripeHandler(ls)

	event := stripe.Event{
		ID:   "evt_test_1",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{
			Raw: buildCheckoutSessionRaw(t, "cs_test_1", "ck_alice", "alice@example.com", "cus_alice_obj"),
		},
	}

	if err := h.OnCheckoutComplete(context.Background(), event); err != nil {
		t.Fatalf("OnCheckoutComplete: %v", err)
	}
	paid, err := ls.IsPaid(context.Background(), "ck_alice")
	if err != nil || !paid {
		t.Fatalf("ck_alice should be paid: paid=%v err=%v", paid, err)
	}
}

func TestLicenseStripeHandler_CheckoutComplete_CustomerAsString(t *testing.T) {
	ls := store.NewMemoryLicenseStore()
	h := NewLicenseStripeHandler(ls)

	// When the event arrives without expanding the customer, .customer is
	// a bare string ID rather than an object.
	raw := []byte(`{"id":"cs_test_2","client_reference_id":"ck_bob","customer_email":"bob@x","mode":"subscription","subscription":"sub_bob","customer":"cus_bob_str"}`)
	event := stripe.Event{
		ID:   "evt_test_2",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}

	if err := h.OnCheckoutComplete(context.Background(), event); err != nil {
		t.Fatalf("OnCheckoutComplete: %v", err)
	}
	paid, _ := ls.IsPaid(context.Background(), "ck_bob")
	if !paid {
		t.Fatal("ck_bob should be paid")
	}
	// Now confirm Revoke by string-form customer ID also works end-to-end.
	if err := h.OnSubscriptionDeleted(context.Background(), stripe.Event{
		ID:   "evt_test_2_del",
		Type: "customer.subscription.deleted",
		Data: &stripe.EventData{Raw: []byte(`{"id":"sub_test","customer":"cus_bob_str"}`)},
	}); err != nil {
		t.Fatalf("OnSubscriptionDeleted: %v", err)
	}
	paid, _ = ls.IsPaid(context.Background(), "ck_bob")
	if paid {
		t.Fatal("ck_bob should be revoked")
	}
}

func TestLicenseStripeHandler_CheckoutComplete_MissingClientReferenceID_Errors(t *testing.T) {
	ls := store.NewMemoryLicenseStore()
	h := NewLicenseStripeHandler(ls)

	// Production signup MUST set client_reference_id; if it doesn't, we
	// have no way to bind the subscription to a Cerebra API key and we
	// MUST loudly fail so Stripe retries (or so the bug gets noticed).
	raw := []byte(`{"id":"cs_test_3","customer_email":"x@y","mode":"subscription","subscription":"sub_x","customer":"cus_nobody"}`)
	event := stripe.Event{
		ID:   "evt_test_3",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}

	err := h.OnCheckoutComplete(context.Background(), event)
	if err == nil {
		t.Fatal("missing client_reference_id should error")
	}
	if !strings.Contains(err.Error(), "client_reference_id") {
		t.Errorf("error should mention client_reference_id, got %v", err)
	}
}

func TestLicenseStripeHandler_SubscriptionDeleted_RevokesLicense(t *testing.T) {
	ls := store.NewMemoryLicenseStore()
	if err := ls.Grant(context.Background(), "ck_carol", "c@x", "cus_carol"); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	h := NewLicenseStripeHandler(ls)

	event := stripe.Event{
		ID:   "evt_del_1",
		Type: "customer.subscription.deleted",
		Data: &stripe.EventData{
			Raw: []byte(`{"id":"sub_x","customer":{"id":"cus_carol","object":"customer"}}`),
		},
	}
	if err := h.OnSubscriptionDeleted(context.Background(), event); err != nil {
		t.Fatalf("OnSubscriptionDeleted: %v", err)
	}
	paid, _ := ls.IsPaid(context.Background(), "ck_carol")
	if paid {
		t.Fatal("ck_carol should be revoked after subscription.deleted")
	}
}

func TestLicenseStripeHandler_SubscriptionDeleted_UnknownCustomerIsNoop(t *testing.T) {
	ls := store.NewMemoryLicenseStore()
	h := NewLicenseStripeHandler(ls)

	event := stripe.Event{
		ID:   "evt_del_unknown",
		Type: "customer.subscription.deleted",
		Data: &stripe.EventData{Raw: []byte(`{"id":"sub_y","customer":"cus_unseen"}`)},
	}
	if err := h.OnSubscriptionDeleted(context.Background(), event); err != nil {
		t.Fatalf("unknown customer should be no-op, got %v", err)
	}
}

func TestLicenseStripeHandler_NilStore_Errors(t *testing.T) {
	h := NewLicenseStripeHandler(nil)
	err := h.OnCheckoutComplete(context.Background(), stripe.Event{ID: "evt_x", Type: "checkout.session.completed", Data: &stripe.EventData{Raw: []byte(`{}`)}})
	if err == nil {
		t.Fatal("nil store should produce an error on OnCheckoutComplete")
	}
}

// TestStripeWebhook_EndToEnd_DispatchesToLicenseStore exercises the route
// + the production handler so we know the full path works on a real mux.
func TestStripeWebhook_EndToEnd_DispatchesToLicenseStore(t *testing.T) {
	const secret = "whsec_e2e_aaaaaaaaaaaaaaaaaaaaaaaa"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	ls := store.NewMemoryLicenseStore()
	mux := http.NewServeMux()
	srv := &Server{mux: mux, stripeHandler: NewLicenseStripeHandler(ls)}
	mux.HandleFunc("POST /api/stripe/webhook", srv.handleStripeWebhook)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	payload := buildEventEnvelope(t, "checkout.session.completed", "evt_e2e_1", map[string]any{
		"id":                  "cs_e2e_1",
		"client_reference_id": "ck_e2e",
		"customer_email":      "e2e@x",
		"mode":                "subscription",
		"subscription":        "sub_e2e",
		"customer":            "cus_e2e",
	})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/stripe/webhook", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Stripe-Signature", sign(t, payload, secret))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	paid, _ := ls.IsPaid(context.Background(), "ck_e2e")
	if !paid {
		t.Fatal("ck_e2e should be paid after e2e webhook flow")
	}
}

func TestLicenseStripeHandler_CheckoutComplete_MissingCustomerErrors(t *testing.T) {
	ls := store.NewMemoryLicenseStore()
	h := NewLicenseStripeHandler(ls)

	// Subscription mode + client_reference_id + subscription id present,
	// but customer is missing. We MUST refuse to grant — revocation only
	// matches on customer id, so without one the entitlement would be
	// unrevokeable.
	raw := []byte(`{"id":"cs_nocust","client_reference_id":"ck_nocust","mode":"subscription","subscription":"sub_x"}`)
	event := stripe.Event{
		ID:   "evt_nocust",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}

	err := h.OnCheckoutComplete(context.Background(), event)
	if err == nil {
		t.Fatal("missing customer should error")
	}
	if !strings.Contains(err.Error(), "customer") {
		t.Errorf("error should mention customer, got %v", err)
	}
	if paid, _ := ls.IsPaid(context.Background(), "ck_nocust"); paid {
		t.Error("must not grant an unrevokeable licence")
	}
}

func TestLicenseStripeHandler_CheckoutComplete_PaymentModeIsSkipped(t *testing.T) {
	ls := store.NewMemoryLicenseStore()
	h := NewLicenseStripeHandler(ls)

	// A one-off payment Checkout Session. We deliberately set
	// client_reference_id here so the test isolates the mode check
	// rather than the missing-key check.
	raw := []byte(`{"id":"cs_pay","client_reference_id":"ck_pay","customer_email":"p@x","mode":"payment","customer":"cus_pay"}`)
	event := stripe.Event{
		ID:   "evt_pay",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}

	if err := h.OnCheckoutComplete(context.Background(), event); err != nil {
		t.Fatalf("payment-mode checkout should be skipped without error, got %v", err)
	}
	if paid, _ := ls.IsPaid(context.Background(), "ck_pay"); paid {
		t.Fatal("payment-mode checkout must NOT grant a licence (it never produces a deletion event)")
	}
}

func TestLicenseStripeHandler_CheckoutComplete_SetupModeIsSkipped(t *testing.T) {
	ls := store.NewMemoryLicenseStore()
	h := NewLicenseStripeHandler(ls)

	raw := []byte(`{"id":"cs_setup","client_reference_id":"ck_setup","mode":"setup","customer":"cus_setup"}`)
	event := stripe.Event{
		ID:   "evt_setup",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}

	if err := h.OnCheckoutComplete(context.Background(), event); err != nil {
		t.Fatalf("setup-mode checkout should be skipped without error, got %v", err)
	}
	if paid, _ := ls.IsPaid(context.Background(), "ck_setup"); paid {
		t.Fatal("setup-mode checkout must NOT grant a licence")
	}
}

func TestLicenseStripeHandler_CheckoutComplete_SubscriptionIDImpliesSubscription(t *testing.T) {
	// Belt-and-braces: if the event arrives with no mode (older API
	// version, weird middleware) but does have a subscription field, we
	// should still grant.
	ls := store.NewMemoryLicenseStore()
	h := NewLicenseStripeHandler(ls)

	raw := []byte(`{"id":"cs_impl","client_reference_id":"ck_impl","subscription":"sub_impl","customer":"cus_impl"}`)
	event := stripe.Event{
		ID:   "evt_impl",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}

	if err := h.OnCheckoutComplete(context.Background(), event); err != nil {
		t.Fatalf("OnCheckoutComplete: %v", err)
	}
	if paid, _ := ls.IsPaid(context.Background(), "ck_impl"); !paid {
		t.Fatal("presence of subscription id should be enough to grant")
	}
}

// buildCheckoutSessionRaw builds a JSON body for a Stripe CheckoutSession
// with the customer field as an object (the "expanded" wire shape), in
// subscription mode (the only mode the licence handler accepts).
func buildCheckoutSessionRaw(t *testing.T, sessID, apiKey, email, customerID string) []byte {
	t.Helper()
	return []byte(fmt.Sprintf(
		`{"id":%q,"client_reference_id":%q,"customer_email":%q,"mode":"subscription","subscription":%q,"customer":{"id":%q,"object":"customer"},"customer_details":{"email":%q}}`,
		sessID, apiKey, email, "sub_"+sessID, customerID, email,
	))
}
