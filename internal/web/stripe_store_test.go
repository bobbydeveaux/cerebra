package web

import (
	"context"
	"errors"
	"fmt"
	"testing"

	stripe "github.com/stripe/stripe-go/v76"
)

// fakeSubWriter records subscription writes for assertions.
type fakeSubWriter struct {
	activeCustomer   string
	activeSession    string
	inactiveCustomer string
	activeCalls      int
	inactiveCalls    int
	// activeErr, when set, is returned from SetSubscriptionActive so handler
	// error-propagation paths can be exercised.
	activeErr error
}

func (f *fakeSubWriter) SetSubscriptionActive(_ context.Context, customerID, sessionID string) error {
	f.activeCalls++
	f.activeCustomer = customerID
	f.activeSession = sessionID
	return f.activeErr
}

func (f *fakeSubWriter) SetSubscriptionInactive(_ context.Context, customerID string) error {
	f.inactiveCalls++
	f.inactiveCustomer = customerID
	return nil
}

func eventWithRaw(t *testing.T, eventType, raw string) stripe.Event {
	t.Helper()
	return stripe.Event{
		ID:   "evt_test",
		Type: stripe.EventType(eventType),
		Data: &stripe.EventData{Raw: []byte(raw)},
	}
}

func TestStoreStripeHandlerCheckoutComplete(t *testing.T) {
	fake := &fakeSubWriter{}
	h := storeStripeHandler{store: fake}

	raw := fmt.Sprintf(`{"id":"cs_test_1","customer":"cus_test_1"}`)
	if err := h.OnCheckoutComplete(context.Background(), eventWithRaw(t, "checkout.session.completed", raw)); err != nil {
		t.Fatalf("OnCheckoutComplete: %v", err)
	}
	if fake.activeCalls != 1 {
		t.Fatalf("activeCalls: got %d want 1", fake.activeCalls)
	}
	if fake.activeCustomer != "cus_test_1" {
		t.Errorf("customer: got %q", fake.activeCustomer)
	}
	if fake.activeSession != "cs_test_1" {
		t.Errorf("session: got %q", fake.activeSession)
	}
}

func TestStoreStripeHandlerSubscriptionDeleted(t *testing.T) {
	fake := &fakeSubWriter{}
	h := storeStripeHandler{store: fake}

	raw := fmt.Sprintf(`{"id":"sub_test_1","customer":"cus_test_2"}`)
	if err := h.OnSubscriptionDeleted(context.Background(), eventWithRaw(t, "customer.subscription.deleted", raw)); err != nil {
		t.Fatalf("OnSubscriptionDeleted: %v", err)
	}
	if fake.inactiveCalls != 1 {
		t.Fatalf("inactiveCalls: got %d want 1", fake.inactiveCalls)
	}
	if fake.inactiveCustomer != "cus_test_2" {
		t.Errorf("customer: got %q", fake.inactiveCustomer)
	}
}

func TestStoreStripeHandlerNoCustomerIgnored(t *testing.T) {
	fake := &fakeSubWriter{}
	h := storeStripeHandler{store: fake}

	// Event with no customer field must be ignored, not errored, so Stripe
	// does not retry forever.
	if err := h.OnCheckoutComplete(context.Background(), eventWithRaw(t, "checkout.session.completed", `{"id":"cs_x"}`)); err != nil {
		t.Fatalf("OnCheckoutComplete: %v", err)
	}
	if fake.activeCalls != 0 {
		t.Fatalf("expected no write for an event with no customer, got %d", fake.activeCalls)
	}
}

// TestStoreStripeHandlerCheckoutComplete_StoreError documents that a store
// failure is NOT swallowed: OnCheckoutComplete returns the error verbatim so
// the webhook responds non-2xx and Stripe retries delivery. Swallowing it
// would silently leave a paying customer un-provisioned.
func TestStoreStripeHandlerCheckoutComplete_StoreError(t *testing.T) {
	wantErr := errors.New("db unavailable")
	fake := &fakeSubWriter{activeErr: wantErr}
	h := storeStripeHandler{store: fake}

	raw := `{"id":"cs_test_err","customer":"cus_test_err"}`
	err := h.OnCheckoutComplete(context.Background(), eventWithRaw(t, "checkout.session.completed", raw))
	if err == nil {
		t.Fatal("expected the store error to propagate, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped wantErr, got %v", err)
	}
	if fake.activeCalls != 1 {
		t.Errorf("store should still be called once before erroring, got %d", fake.activeCalls)
	}
}
