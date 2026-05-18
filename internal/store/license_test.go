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
	if err := s.Grant(ctx, "ck_alice", "alice@example.com", "cus_alice"); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if paid, err := s.IsPaid(ctx, "ck_alice"); err != nil || !paid {
		t.Fatalf("IsPaid(ck_alice): want (true, nil), got (%v, %v)", paid, err)
	}

	// Revoke by stripe customer id and confirm gone.
	if err := s.Revoke(ctx, "cus_alice"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if paid, err := s.IsPaid(ctx, "ck_alice"); err != nil || paid {
		t.Fatalf("IsPaid after revoke: want (false, nil), got (%v, %v)", paid, err)
	}
}

func TestMemoryLicenseStore_RevokeUnknownIsNoop(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	if err := s.Revoke(ctx, "cus_never_seen"); err != nil {
		t.Fatalf("Revoke unknown should be a no-op, got %v", err)
	}
}

func TestMemoryLicenseStore_GrantIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryLicenseStore()

	if err := s.Grant(ctx, "ck_bob", "bob@old.example.com", "cus_bob"); err != nil {
		t.Fatalf("first Grant: %v", err)
	}
	// Re-grant with a new email; same key, same customer.
	if err := s.Grant(ctx, "ck_bob", "bob@new.example.com", "cus_bob"); err != nil {
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
	if err := s.Grant(ctx, "ck_charlie", "c@example.com", "cus_old"); err != nil {
		t.Fatalf("Grant 1: %v", err)
	}
	// Re-bind ck_charlie to cus_new (e.g. customer migrated billing).
	if err := s.Grant(ctx, "ck_charlie", "c@example.com", "cus_new"); err != nil {
		t.Fatalf("Grant 2: %v", err)
	}
	// Revoking cus_old must NOT evict ck_charlie (which now belongs to cus_new).
	if err := s.Revoke(ctx, "cus_old"); err != nil {
		t.Fatalf("Revoke old: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_charlie"); !paid {
		t.Fatal("Revoking the previous customer id must not evict the re-bound key")
	}
	// Revoking cus_new SHOULD evict ck_charlie.
	if err := s.Revoke(ctx, "cus_new"); err != nil {
		t.Fatalf("Revoke new: %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_charlie"); paid {
		t.Fatal("Revoking the current customer id should evict the key")
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

	if err := s.Grant(ctx, "", "x@y", "cus_x"); err == nil {
		t.Fatal("Grant with empty apiKey should fail")
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

	if err := s.Grant(ctx, "ck_alice", "alice@example.com", "cus_alice"); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if paid, err := s.IsPaid(ctx, "ck_alice"); err != nil || !paid {
		t.Fatalf("IsPaid(alice): want (true, nil), got (%v, %v)", paid, err)
	}

	if err := s.Revoke(ctx, "cus_alice"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if paid, err := s.IsPaid(ctx, "ck_alice"); err != nil || paid {
		t.Fatalf("IsPaid after revoke: want (false, nil), got (%v, %v)", paid, err)
	}
}

func TestSQLiteStore_LicenseGrantIsIdempotent(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.Grant(ctx, "ck_bob", "old@x", "cus_bob"); err != nil {
		t.Fatalf("first Grant: %v", err)
	}
	if err := s.Grant(ctx, "ck_bob", "new@x", "cus_bob"); err != nil {
		t.Fatalf("second Grant (idempotent): %v", err)
	}
	if paid, _ := s.IsPaid(ctx, "ck_bob"); !paid {
		t.Fatal("ck_bob still paid after re-grant")
	}
}

func TestSQLiteStore_LicenseGrantRejectsEmptyKey(t *testing.T) {
	s := testDB(t)
	if err := s.Grant(context.Background(), "", "x@y", "cus_x"); err == nil {
		t.Fatal("Grant with empty apiKey should fail")
	}
}
