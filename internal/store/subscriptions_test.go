package store

import (
	"context"
	"strings"
	"testing"
)

func TestSubscriptionLifecycle(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	// No subscriptions yet: instance is not licensed.
	active, err := s.HasActiveSubscription(ctx)
	if err != nil {
		t.Fatalf("HasActiveSubscription empty: %v", err)
	}
	if active {
		t.Fatal("expected no active subscription on a fresh store")
	}

	// Activate a customer: instance becomes licensed.
	if err := s.SetSubscriptionActive(ctx, "cus_123", "cs_abc"); err != nil {
		t.Fatalf("SetSubscriptionActive: %v", err)
	}
	active, err = s.HasActiveSubscription(ctx)
	if err != nil {
		t.Fatalf("HasActiveSubscription active: %v", err)
	}
	if !active {
		t.Fatal("expected active subscription after activation")
	}

	// Re-deliver the same checkout event: must be idempotent (no duplicate
	// rows, still exactly one customer).
	if err := s.SetSubscriptionActive(ctx, "cus_123", "cs_def"); err != nil {
		t.Fatalf("SetSubscriptionActive idempotent: %v", err)
	}
	sub, err := s.GetSubscription(ctx, "cus_123")
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if sub == nil {
		t.Fatal("expected a subscription row")
	}
	if sub.Status != SubscriptionActive {
		t.Errorf("status: got %q want %q", sub.Status, SubscriptionActive)
	}
	if sub.StripeSessionID != "cs_def" {
		t.Errorf("session id should update on re-delivery: got %q", sub.StripeSessionID)
	}

	// Cancel: instance loses access.
	if err := s.SetSubscriptionInactive(ctx, "cus_123"); err != nil {
		t.Fatalf("SetSubscriptionInactive: %v", err)
	}
	active, err = s.HasActiveSubscription(ctx)
	if err != nil {
		t.Fatalf("HasActiveSubscription cancelled: %v", err)
	}
	if active {
		t.Fatal("expected no active subscription after cancellation")
	}
}

// TestGetSubscription_Found upserts a subscription and reads it back, asserting
// every field is populated. TestSubscriptionLifecycle exercises GetSubscription
// in passing; this isolates the happy path so a regression points straight here.
func TestGetSubscription_Found(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.SetSubscriptionActive(ctx, "cus_found", "cs_found"); err != nil {
		t.Fatalf("SetSubscriptionActive: %v", err)
	}

	sub, err := s.GetSubscription(ctx, "cus_found")
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if sub == nil {
		t.Fatal("expected a subscription, got nil")
	}
	if sub.StripeCustomerID != "cus_found" {
		t.Errorf("customer id: got %q want %q", sub.StripeCustomerID, "cus_found")
	}
	if sub.Status != SubscriptionActive {
		t.Errorf("status: got %q want %q", sub.Status, SubscriptionActive)
	}
	if sub.StripeSessionID != "cs_found" {
		t.Errorf("session id: got %q want %q", sub.StripeSessionID, "cs_found")
	}
	if sub.CreatedAt.IsZero() {
		t.Error("created_at should be populated by the CURRENT_TIMESTAMP default")
	}
	if sub.UpdatedAt.IsZero() {
		t.Error("updated_at should be populated by the CURRENT_TIMESTAMP default")
	}
}

// TestGetSubscription_NotFound asserts that an unknown customer returns
// (nil, nil), not an error. Callers distinguish "no subscription" from "lookup
// failed" by the nil pointer, so this contract must hold.
func TestGetSubscription_NotFound(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	sub, err := s.GetSubscription(ctx, "cus_does_not_exist")
	if err != nil {
		t.Fatalf("expected no error for an unknown customer, got %v", err)
	}
	if sub != nil {
		t.Fatalf("expected nil subscription for an unknown customer, got %+v", sub)
	}
}

// TestGetSubscription_ClosedDB asserts the error-wrap branch is reached when the
// underlying connection is gone. Closing the store before the call forces the
// driver to return ErrConnDone rather than sql.ErrNoRows, so the (nil, nil)
// short-circuit is not taken and the "getting subscription" wrap fires.
func TestGetSubscription_ClosedDB(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.SetSubscriptionActive(ctx, "cus_closed_get", "cs_seed"); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	sub, err := s.GetSubscription(ctx, "cus_closed_get")
	if err == nil {
		t.Fatal("expected GetSubscription to error after Close")
	}
	if !strings.Contains(err.Error(), "getting subscription") {
		t.Errorf("expected getting-subscription wrap, got %v", err)
	}
	if sub != nil {
		t.Errorf("expected nil subscription on error, got %+v", sub)
	}
}

func TestHasActiveSubscriptionWithMultipleCustomers(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.SetSubscriptionActive(ctx, "cus_a", "cs_a"); err != nil {
		t.Fatalf("activate a: %v", err)
	}
	if err := s.SetSubscriptionActive(ctx, "cus_b", "cs_b"); err != nil {
		t.Fatalf("activate b: %v", err)
	}
	// Cancelling one of two active customers must leave the instance
	// licensed (instance-level gate).
	if err := s.SetSubscriptionInactive(ctx, "cus_a"); err != nil {
		t.Fatalf("cancel a: %v", err)
	}
	active, err := s.HasActiveSubscription(ctx)
	if err != nil {
		t.Fatalf("HasActiveSubscription: %v", err)
	}
	if !active {
		t.Fatal("instance should stay licensed while any customer is active")
	}
}

func TestSubscriptionEmptyCustomerRejected(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.SetSubscriptionActive(ctx, "", "cs_x"); err == nil {
		t.Fatal("expected error activating with empty customer id")
	}
	if err := s.SetSubscriptionInactive(ctx, ""); err == nil {
		t.Fatal("expected error deactivating with empty customer id")
	}
}

func TestSetSubscriptionInactiveUnknownCustomerIsNoop(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	// Deactivating a customer we never recorded must not error (replayed
	// or out-of-order delete event).
	if err := s.SetSubscriptionInactive(ctx, "cus_never_seen"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sub, err := s.GetSubscription(ctx, "cus_never_seen")
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if sub != nil {
		t.Fatal("expected no row for an unknown customer")
	}
}

// TestSubscription_ClosedDBErrors fills the error-return branches of
// subscriptions.go. The happy paths, idempotent re-delivery, empty-customer
// guard and unknown-customer no-op are exercised above; the remaining
// uncovered code is the fmt.Errorf wrap in each method, reached only when the
// underlying connection fails. Closing the store before each call forces the
// driver to return ErrConnDone, the same convention used by
// TestActivity_ClosedDBErrors and TestAgentMessages_ClosedDBErrors.
func TestSubscription_ClosedDBErrors(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	// Seed one active customer so the queries have a real target before the
	// close trips them.
	if err := s.SetSubscriptionActive(ctx, "cus_closed", "cs_seed"); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// SetSubscriptionActive on closed DB -> "activating subscription" wrap.
	// A non-empty customer ID is required so the empty-customer guard does not
	// short-circuit before the Exec that actually trips the closed connection.
	if err := s.SetSubscriptionActive(ctx, "cus_closed", "cs_after"); err == nil {
		t.Error("expected SetSubscriptionActive to error after Close")
	} else if !strings.Contains(err.Error(), "activating subscription") {
		t.Errorf("expected activate wrap, got %v", err)
	}

	// SetSubscriptionInactive on closed DB -> "deactivating subscription" wrap.
	if err := s.SetSubscriptionInactive(ctx, "cus_closed"); err == nil {
		t.Error("expected SetSubscriptionInactive to error after Close")
	} else if !strings.Contains(err.Error(), "deactivating subscription") {
		t.Errorf("expected deactivate wrap, got %v", err)
	}

	// HasActiveSubscription on closed DB -> "checking active subscription"
	// wrap. The closed connection returns a driver error rather than
	// sql.ErrNoRows, so the false/nil short-circuit is not taken.
	if _, err := s.HasActiveSubscription(ctx); err == nil {
		t.Error("expected HasActiveSubscription to error after Close")
	} else if !strings.Contains(err.Error(), "checking active subscription") {
		t.Errorf("expected check wrap, got %v", err)
	}

	// GetSubscription on closed DB -> "getting subscription" wrap. Distinct
	// from the unknown-customer nil-return path, which returns (nil, nil).
	if _, err := s.GetSubscription(ctx, "cus_closed"); err == nil {
		t.Error("expected GetSubscription to error after Close")
	} else if !strings.Contains(err.Error(), "getting subscription") {
		t.Errorf("expected get wrap, got %v", err)
	}
}
