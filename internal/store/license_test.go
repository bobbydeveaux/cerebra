package store

import (
	"context"
	"testing"
)

func TestMemoryLicenseStore_GrantIsPaidRevoke(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	// Unknown key: not paid.
	if paid, err := s.IsPaid(ctx, "ck_unknown"); err != nil || paid {
		t.Fatalf("IsPaid(unknown): want (false, nil), got (%v, %v)", paid, err)
	}

	// Grant and confirm.
	if err := s.Grant(ctx, "ck_alice", "alice@example.com", "cus_alice", 0); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if paid, err := s.IsPaid(ctx, "ck_alice"); err != nil || !paid {
		t.Fatalf("IsPaid(ck_alice): want (true, nil), got (%v, %v)", paid, err)
	}

	// Revoke by stripe customer id and confirm gone.
	if err := s.Revoke(ctx, "cus_alice", 0); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if paid, err := s.IsPaid(ctx, "ck_alice"); err != nil || paid {
		t.Fatalf("IsPaid after revoke: want (false, nil), got (%v, %v)", paid, err)
	}
}

func TestMemoryLicenseStore_RevokeUnknownIsNoop(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	if err := s.Revoke(ctx, "cus_never_seen", 0); err != nil {
		t.Fatalf("Revoke unknown should be a no-op, got %v", err)
	}
}

func TestMemoryLicenseStore_GrantIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	if err := s.Grant(ctx, "ck_bob", "bob@old.example.com", "cus_bob", 0); err != nil {
		t.Fatalf("first Grant: %v", err)
	}
	// Re-grant with a new email; same key, same customer.
	if err := s.Grant(ctx, "ck_bob", "bob@new.example.com", "cus_bob", 0); err != nil {
		t.Fatalf("second Grant: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_bob"); !paid {
		t.Fatal("ck_bob should still be paid after re-grant")
	}
}

func TestMemoryLicenseStore_GrantWithNewCustomerDropsOldReverseIndex(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	// Bind ck_charlie to cus_old.
	if err := s.Grant(ctx, "ck_charlie", "c@example.com", "cus_old", 0); err != nil {
		t.Fatalf("Grant 1: %v", err)
	}
	// Re-bind ck_charlie to cus_new (e.g. customer migrated billing).
	if err := s.Grant(ctx, "ck_charlie", "c@example.com", "cus_new", 0); err != nil {
		t.Fatalf("Grant 2: %v", err)
	}
	// Revoking cus_old must NOT evict ck_charlie (which now belongs to cus_new).
	if err := s.Revoke(ctx, "cus_old", 0); err != nil {
		t.Fatalf("Revoke old: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_charlie"); !paid {
		t.Fatal("Revoking the previous customer id must not evict the re-bound key")
	}
	// Revoking cus_new SHOULD evict ck_charlie.
	if err := s.Revoke(ctx, "cus_new", 0); err != nil {
		t.Fatalf("Revoke new: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_charlie"); paid {
		t.Fatal("Revoking the current customer id should evict the key")
	}
}

func TestMemoryLicenseStore_GrantEvictsPriorKeyForSameCustomer(t *testing.T) {
	// If the same Stripe customer pays again under a different
	// client_reference_id, the old key MUST be evicted — otherwise both
	// keys stay paid, and the later cancellation event only revokes one.
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	if err := s.Grant(ctx, "ck_old", "x@y", "cus_shared", 0); err != nil {
		t.Fatalf("Grant ck_old: %v", err)
	}
	if err := s.Grant(ctx, "ck_new", "x@y", "cus_shared", 0); err != nil {
		t.Fatalf("Grant ck_new: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_old"); paid {
		t.Error("ck_old should be evicted when cus_shared rebinds to ck_new")
	}
	if paid, _ := s.IsPaid(ctx, "ck_new"); !paid {
		t.Error("ck_new should be paid")
	}
	// Revoking the customer should now clear ck_new.
	if err := s.Revoke(ctx, "cus_shared", 0); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_new"); paid {
		t.Error("ck_new should be revoked")
	}
}

func TestMemoryLicenseStore_EmptyApiKeyIsNeverPaid(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	if paid, err := s.IsPaid(ctx, ""); err != nil || paid {
		t.Fatalf("IsPaid(\"\"): want (false, nil), got (%v, %v)", paid, err)
	}
	if paid, err := s.IsPaid(ctx, "   "); err != nil || paid {
		t.Fatalf("IsPaid(\"   \"): want (false, nil), got (%v, %v)", paid, err)
	}
}

func TestMemoryLicenseStore_GrantRejectsEmptyApiKey(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	if err := s.Grant(ctx, "", "x@y", "cus_x", 0); err == nil {
		t.Fatal("Grant with empty apiKey should fail")
	}
}

// TestMemoryLicenseStore_StaleGrantAfterRevokeIsRejected — Codex pass 3 [P2].
// Stripe events can arrive out of order. A delayed checkout.completed for a
// customer who has already been cancelled must NOT regrant the key.
func TestMemoryLicenseStore_StaleGrantAfterRevokeIsRejected(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	// Real-world order of arrivals: grant at T=10, deletion at T=20.
	if err := s.Grant(ctx, "ck_paid", "x@y", "cus_p", 10); err != nil {
		t.Fatalf("Grant T=10: %v", err)
	}
	if err := s.Revoke(ctx, "cus_p", 20); err != nil {
		t.Fatalf("Revoke T=20: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_paid"); paid {
		t.Fatal("ck_paid should be revoked after T=20")
	}

	// Now a delayed checkout.completed at T=5 arrives. Watermark is 20,
	// event is older — must be silently dropped.
	if err := s.Grant(ctx, "ck_paid", "x@y", "cus_p", 5); err != nil {
		t.Fatalf("stale Grant T=5: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_paid"); paid {
		t.Fatal("stale Grant must not regrant a cancelled licence")
	}
}

// TestMemoryLicenseStore_StaleRevokeIsRejected covers the reverse:
// a delayed deletion event arriving after a fresh grant should not
// revoke the new entitlement.
func TestMemoryLicenseStore_StaleRevokeIsRejected(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	// Grant at T=20 (after a previous cancellation at T=10 that we have
	// already processed). A late delivery of the T=10 deletion must not
	// revoke the new entitlement.
	if err := s.Revoke(ctx, "cus_p", 10); err != nil {
		t.Fatalf("Revoke T=10: %v", err)
	}
	if err := s.Grant(ctx, "ck_new", "x@y", "cus_p", 20); err != nil {
		t.Fatalf("Grant T=20: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_new"); !paid {
		t.Fatal("ck_new should be paid at T=20")
	}

	// Delayed duplicate of the T=10 deletion — must be silently dropped.
	if err := s.Revoke(ctx, "cus_p", 10); err != nil {
		t.Fatalf("stale Revoke T=10: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_new"); !paid {
		t.Fatal("stale Revoke must not affect a newer entitlement")
	}
}

// TestMemoryLicenseStore_ZeroEventDisablesOrderingCheck preserves the
// behaviour that callers without a Stripe-event context (admin tooling,
// older tests) can opt out of the watermark by passing 0.
func TestMemoryLicenseStore_ZeroEventDisablesOrderingCheck(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	if err := s.Grant(ctx, "ck_one", "x@y", "cus_z", 100); err != nil {
		t.Fatalf("Grant T=100: %v", err)
	}
	// eventCreatedAt=0 must bypass the watermark check entirely.
	if err := s.Grant(ctx, "ck_two", "x@y", "cus_z", 0); err != nil {
		t.Fatalf("Grant T=0 (ordering bypass): %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_two"); !paid {
		t.Fatal("ck_two should be paid (zero-timestamp grant bypasses ordering)")
	}
}

// SQLite-backed coverage. Uses the same testDB helper that store_test.go
// already provides; the licenses table is created at init time.

func TestSQLiteStore_LicenseGrantIsPaidRevoke(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if paid, err := s.IsPaid(ctx, "ck_unknown"); err != nil || paid {
		t.Fatalf("IsPaid(unknown): want (false, nil), got (%v, %v)", paid, err)
	}

	if err := s.Grant(ctx, "ck_alice", "alice@example.com", "cus_alice", 0); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if paid, err := s.IsPaid(ctx, "ck_alice"); err != nil || !paid {
		t.Fatalf("IsPaid(alice): want (true, nil), got (%v, %v)", paid, err)
	}

	if err := s.Revoke(ctx, "cus_alice", 0); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if paid, err := s.IsPaid(ctx, "ck_alice"); err != nil || paid {
		t.Fatalf("IsPaid after revoke: want (false, nil), got (%v, %v)", paid, err)
	}
}

func TestSQLiteStore_LicenseGrantIsIdempotent(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.Grant(ctx, "ck_bob", "old@x", "cus_bob", 0); err != nil {
		t.Fatalf("first Grant: %v", err)
	}
	if err := s.Grant(ctx, "ck_bob", "new@x", "cus_bob", 0); err != nil {
		t.Fatalf("second Grant (idempotent): %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_bob"); !paid {
		t.Fatal("ck_bob still paid after re-grant")
	}
}

func TestSQLiteStore_LicenseGrantRejectsEmptyKey(t *testing.T) {
	s := testDB(t)
	if err := s.Grant(context.Background(), "", "x@y", "cus_x", 0); err == nil {
		t.Fatal("Grant with empty apiKey should fail")
	}
}

func TestSQLiteStore_LicenseGrantEvictsPriorKeyForSameCustomer(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.Grant(ctx, "ck_old", "x@y", "cus_shared", 0); err != nil {
		t.Fatalf("Grant ck_old: %v", err)
	}
	if err := s.Grant(ctx, "ck_new", "x@y", "cus_shared", 0); err != nil {
		t.Fatalf("Grant ck_new: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_old"); paid {
		t.Error("SQLite: ck_old should be evicted when cus_shared rebinds")
	}
	if paid, _ := s.IsPaid(ctx, "ck_new"); !paid {
		t.Error("SQLite: ck_new should be paid")
	}
	if err := s.Revoke(ctx, "cus_shared", 0); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_new"); paid {
		t.Error("SQLite: ck_new should be revoked")
	}
}

// TestSQLiteStore_StaleGrantAfterRevokeIsRejected — Codex pass 3 [P2]
// applied to the persistent backend.
func TestSQLiteStore_StaleGrantAfterRevokeIsRejected(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.Grant(ctx, "ck_paid", "x@y", "cus_p", 10); err != nil {
		t.Fatalf("Grant T=10: %v", err)
	}
	if err := s.Revoke(ctx, "cus_p", 20); err != nil {
		t.Fatalf("Revoke T=20: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_paid"); paid {
		t.Fatal("ck_paid should be revoked after T=20")
	}

	if err := s.Grant(ctx, "ck_paid", "x@y", "cus_p", 5); err != nil {
		t.Fatalf("stale Grant T=5: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_paid"); paid {
		t.Fatal("SQLite: stale Grant must not regrant a cancelled licence")
	}
}

func TestSQLiteStore_StaleRevokeIsRejected(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.Revoke(ctx, "cus_p", 10); err != nil {
		t.Fatalf("Revoke T=10: %v", err)
	}
	if err := s.Grant(ctx, "ck_new", "x@y", "cus_p", 20); err != nil {
		t.Fatalf("Grant T=20: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_new"); !paid {
		t.Fatal("ck_new should be paid at T=20")
	}

	if err := s.Revoke(ctx, "cus_p", 10); err != nil {
		t.Fatalf("stale Revoke T=10: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_new"); !paid {
		t.Fatal("SQLite: stale Revoke must not affect a newer entitlement")
	}
}

func TestSQLiteStore_ZeroEventDisablesOrderingCheck(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.Grant(ctx, "ck_one", "x@y", "cus_z", 100); err != nil {
		t.Fatalf("Grant T=100: %v", err)
	}
	if err := s.Grant(ctx, "ck_two", "x@y", "cus_z", 0); err != nil {
		t.Fatalf("Grant T=0 (ordering bypass): %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_two"); !paid {
		t.Fatal("SQLite: ck_two should be paid (zero-timestamp grant bypasses ordering)")
	}
}
