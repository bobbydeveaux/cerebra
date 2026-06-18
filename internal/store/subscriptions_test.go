package store

import (
	"context"
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
