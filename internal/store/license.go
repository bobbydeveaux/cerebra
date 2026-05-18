// Package store — license store for paid-tier gating (agentops-012).
//
// A LicenseStore tracks which Cerebra API keys are entitled to paid-tier
// features. The Stripe webhook from agentops-011 calls Grant on
// checkout.session.completed and Revoke on customer.subscription.deleted.
// The RequirePaid HTTP middleware (internal/web/license.go) calls IsPaid on
// every gated request.
//
// Two implementations live in this file:
//
//  1. MemoryLicenseStore — sync.RWMutex-backed maps. Useful for tests and
//     for local dev where no SQLite path is configured.
//  2. The methods on *SQLiteStore (see also internal/store/store.go) —
//     persisted across restarts. Schema is in schema.go (licenseSchemaSQL).
//
// Both honour the same LicenseStore interface so the web layer never knows
// which backend it has.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// LicenseStore tracks paid-tier entitlements keyed by Cerebra API key, with
// a reverse index from the Stripe customer ID so subscription deletions can
// be processed without the original API key being present in the webhook
// payload.
//
// Grant is idempotent — re-running it for the same apiKey updates email and
// stripeCustomerID. Revoke is idempotent — revoking an unknown customer ID
// is not an error.
type LicenseStore interface {
	// Grant records that the given apiKey is entitled to the paid tier.
	// email is informational (for receipts/support); stripeCustomerID is
	// the reverse lookup key for Revoke.
	Grant(ctx context.Context, apiKey, email, stripeCustomerID string) error

	// Revoke removes the entry for the subscription associated with the
	// given Stripe customer ID. Idempotent — returns nil for unknown IDs.
	Revoke(ctx context.Context, stripeCustomerID string) error

	// IsPaid reports whether the given apiKey is currently entitled. An
	// empty apiKey is never paid. Errors are returned only on backing-store
	// failure; "not found" is reported as (false, nil).
	IsPaid(ctx context.Context, apiKey string) (bool, error)
}

// License is the row shape persisted by the SQLite backend and held in
// memory by MemoryLicenseStore. Exported so callers can introspect for
// admin/debug tooling later; not part of the LicenseStore contract.
type License struct {
	APIKey           string
	Email            string
	StripeCustomerID string
	GrantedAt        time.Time
}

// MemoryLicenseStore is an in-process LicenseStore. Safe for concurrent use.
// Zero value is not usable; construct with NewMemoryLicenseStore.
type MemoryLicenseStore struct {
	mu          sync.RWMutex
	byKey       map[string]License // apiKey → License
	byCustomer  map[string]string  // stripeCustomerID → apiKey
}

// NewMemoryLicenseStore returns an empty in-memory store.
func NewMemoryLicenseStore() *MemoryLicenseStore {
	return &MemoryLicenseStore{
		byKey:      make(map[string]License),
		byCustomer: make(map[string]string),
	}
}

// Grant inserts or updates the entitlement for apiKey, and enforces the
// invariant that at most one apiKey is bound to a given Stripe customer
// at any time. If the customer previously paid for a different apiKey
// (e.g. they changed their Cerebra key between billing cycles), the old
// entitlement is evicted as part of the grant. Symmetrically, if this
// apiKey was previously bound to a different customer, the stale reverse
// index is cleared.
func (s *MemoryLicenseStore) Grant(_ context.Context, apiKey, email, stripeCustomerID string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("license: apiKey is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// If this apiKey was previously bound to a different customer, drop
	// the stale reverse-index entry.
	if prev, ok := s.byKey[apiKey]; ok && prev.StripeCustomerID != "" && prev.StripeCustomerID != stripeCustomerID {
		delete(s.byCustomer, prev.StripeCustomerID)
	}
	// If this customer was previously bound to a different apiKey, evict
	// that old apiKey — cancellation events only carry the customer id,
	// so we MUST keep customer→apiKey as a single source of truth.
	if stripeCustomerID != "" {
		if prevKey, ok := s.byCustomer[stripeCustomerID]; ok && prevKey != apiKey {
			delete(s.byKey, prevKey)
		}
	}

	s.byKey[apiKey] = License{
		APIKey:           apiKey,
		Email:            email,
		StripeCustomerID: stripeCustomerID,
		GrantedAt:        time.Now().UTC(),
	}
	if stripeCustomerID != "" {
		s.byCustomer[stripeCustomerID] = apiKey
	}
	return nil
}

// Revoke removes the entry whose stripeCustomerID matches. Unknown ID is
// not an error — Stripe occasionally retries deletion events.
func (s *MemoryLicenseStore) Revoke(_ context.Context, stripeCustomerID string) error {
	stripeCustomerID = strings.TrimSpace(stripeCustomerID)
	if stripeCustomerID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	apiKey, ok := s.byCustomer[stripeCustomerID]
	if !ok {
		return nil
	}
	delete(s.byCustomer, stripeCustomerID)
	delete(s.byKey, apiKey)
	return nil
}

// IsPaid reports whether apiKey is currently entitled.
func (s *MemoryLicenseStore) IsPaid(_ context.Context, apiKey string) (bool, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.byKey[apiKey]
	return ok, nil
}

// --- SQLite backend ---------------------------------------------------------
//
// Methods are attached to the existing *SQLiteStore so it satisfies the
// LicenseStore interface alongside Store. Schema lives in schema.go.

// Grant inserts or updates the entitlement for apiKey, persisted in the
// licenses table. Mirrors MemoryLicenseStore.Grant's invariant: at most
// one apiKey may be bound to a given stripe_customer_id at any time. We
// can't rely on a UNIQUE constraint on stripe_customer_id because empty
// strings collide; so the eviction is explicit and runs in a
// transaction with the upsert.
func (s *SQLiteStore) Grant(ctx context.Context, apiKey, email, stripeCustomerID string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("license: apiKey is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Evict any other apiKey already bound to this customer. Restricted
	// to non-empty customer IDs so an empty bucket doesn't sweep
	// unrelated rows.
	if stripeCustomerID != "" {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM licenses WHERE stripe_customer_id = ? AND api_key <> ?`,
			stripeCustomerID, apiKey); err != nil {
			return fmt.Errorf("evicting prior key for customer: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO licenses (api_key, email, stripe_customer_id, granted_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(api_key) DO UPDATE SET
		   email=excluded.email,
		   stripe_customer_id=excluded.stripe_customer_id,
		   granted_at=CURRENT_TIMESTAMP`,
		apiKey, email, stripeCustomerID); err != nil {
		return err
	}
	return tx.Commit()
}

// Revoke removes the row whose stripe_customer_id matches.
func (s *SQLiteStore) Revoke(ctx context.Context, stripeCustomerID string) error {
	stripeCustomerID = strings.TrimSpace(stripeCustomerID)
	if stripeCustomerID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM licenses WHERE stripe_customer_id = ?`, stripeCustomerID)
	return err
}

// IsPaid reports whether apiKey has a row in the licenses table.
func (s *SQLiteStore) IsPaid(ctx context.Context, apiKey string) (bool, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return false, nil
	}
	var present int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM licenses WHERE api_key = ? LIMIT 1`, apiKey).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
