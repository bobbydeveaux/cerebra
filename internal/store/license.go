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
//
// Both Grant and Revoke take an eventCreatedAt unix-seconds timestamp so
// the store can reject out-of-order Stripe webhook deliveries. Stripe can
// (and does) deliver events out of order — a delayed checkout.completed
// for a customer who has since cancelled would otherwise mistakenly
// regrant access. The store keeps a per-customer high-watermark: any
// event whose timestamp is strictly less than the stored watermark is
// dropped without effect (Codex pass 3 [P2]). Passing eventCreatedAt=0
// disables the check (used by callers that don't have a Stripe context,
// e.g. admin tooling or tests that don't care about ordering).
type LicenseStore interface {
	// Grant records that the given apiKey is entitled to the paid tier.
	// email is informational (for receipts/support); stripeCustomerID is
	// the reverse lookup key for Revoke. eventCreatedAt is Stripe's
	// event.Created (unix seconds) — Grant becomes a no-op if a strictly
	// newer event for this customer has already been processed.
	Grant(ctx context.Context, apiKey, email, stripeCustomerID string, eventCreatedAt int64) error

	// Revoke removes the entry for the subscription associated with the
	// given Stripe customer ID. Idempotent — returns nil for unknown IDs.
	// eventCreatedAt is the Stripe event.Created timestamp; a Revoke
	// older than the stored watermark is dropped.
	Revoke(ctx context.Context, stripeCustomerID string, eventCreatedAt int64) error

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
	mu sync.RWMutex
	byKey       map[string]License // apiKey → License
	byCustomer  map[string]string  // stripeCustomerID → apiKey
	// lastEvent is the per-customer high-watermark used to reject
	// out-of-order Stripe webhook events (Codex pass 3 [P2]). The
	// timestamp survives Revoke so a delayed Grant for the same
	// customer cannot regrant a cancelled key.
	lastEvent map[string]int64 // stripeCustomerID → unix seconds
}

// NewMemoryLicenseStore returns an empty in-memory store.
func NewMemoryLicenseStore() *MemoryLicenseStore {
	return &MemoryLicenseStore{
		byKey:      make(map[string]License),
		byCustomer: make(map[string]string),
		lastEvent:  make(map[string]int64),
	}
}

// Grant inserts or updates the entitlement for apiKey, and enforces the
// invariant that at most one apiKey is bound to a given Stripe customer
// at any time. If the customer previously paid for a different apiKey
// (e.g. they changed their Cerebra key between billing cycles), the old
// entitlement is evicted as part of the grant. Symmetrically, if this
// apiKey was previously bound to a different customer, the stale reverse
// index is cleared.
//
// eventCreatedAt is the Stripe event.Created unix timestamp. If a
// strictly newer event for this customer has already been processed,
// the Grant is dropped without effect — Stripe can deliver events out
// of order and a delayed checkout.completed must not regrant a key
// that was subsequently cancelled (Codex pass 3 [P2]). Pass 0 to
// disable the check (admin/test callers without Stripe ordering
// semantics).
func (s *MemoryLicenseStore) Grant(_ context.Context, apiKey, email, stripeCustomerID string, eventCreatedAt int64) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("license: apiKey is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stale-event check. A customer with no recorded watermark is fresh
	// (first event seen). eventCreatedAt == 0 means the caller has opted
	// out of ordering enforcement.
	if eventCreatedAt > 0 && stripeCustomerID != "" {
		if prev, ok := s.lastEvent[stripeCustomerID]; ok && eventCreatedAt < prev {
			return nil
		}
	}

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
		if eventCreatedAt > 0 {
			s.lastEvent[stripeCustomerID] = eventCreatedAt
		}
	}
	return nil
}

// Revoke removes the entry whose stripeCustomerID matches. Unknown ID is
// not an error — Stripe occasionally retries deletion events.
//
// eventCreatedAt is the Stripe event.Created unix timestamp; an older
// event is dropped without effect (Codex pass 3 [P2]). The watermark
// is updated even when the entitlement is already absent so a later
// stale Grant for the same customer cannot regrant access. Pass 0 to
// disable the ordering check.
func (s *MemoryLicenseStore) Revoke(_ context.Context, stripeCustomerID string, eventCreatedAt int64) error {
	stripeCustomerID = strings.TrimSpace(stripeCustomerID)
	if stripeCustomerID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if eventCreatedAt > 0 {
		if prev, ok := s.lastEvent[stripeCustomerID]; ok && eventCreatedAt < prev {
			return nil
		}
		s.lastEvent[stripeCustomerID] = eventCreatedAt
	}

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
//
// See LicenseStore.Grant for the eventCreatedAt contract — older events
// are dropped to defend against out-of-order Stripe webhook delivery
// (Codex pass 3 [P2]). The watermark is held in customer_events and
// survives Revoke, so a stale grant for a cancelled customer cannot
// regrant access.
func (s *SQLiteStore) Grant(ctx context.Context, apiKey, email, stripeCustomerID string, eventCreatedAt int64) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("license: apiKey is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if eventCreatedAt > 0 && stripeCustomerID != "" {
		stale, err := isStaleEvent(ctx, tx, stripeCustomerID, eventCreatedAt)
		if err != nil {
			return fmt.Errorf("checking event watermark: %w", err)
		}
		if stale {
			return nil
		}
	}

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
	if eventCreatedAt > 0 && stripeCustomerID != "" {
		if err := upsertEventWatermark(ctx, tx, stripeCustomerID, eventCreatedAt); err != nil {
			return fmt.Errorf("updating event watermark: %w", err)
		}
	}
	return tx.Commit()
}

// Revoke removes the row whose stripe_customer_id matches.
//
// See LicenseStore.Revoke for the eventCreatedAt contract. The watermark
// is updated even when no row is removed so a later stale grant cannot
// silently regrant a cancelled subscription (Codex pass 3 [P2]).
func (s *SQLiteStore) Revoke(ctx context.Context, stripeCustomerID string, eventCreatedAt int64) error {
	stripeCustomerID = strings.TrimSpace(stripeCustomerID)
	if stripeCustomerID == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if eventCreatedAt > 0 {
		stale, err := isStaleEvent(ctx, tx, stripeCustomerID, eventCreatedAt)
		if err != nil {
			return fmt.Errorf("checking event watermark: %w", err)
		}
		if stale {
			return nil
		}
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM licenses WHERE stripe_customer_id = ?`, stripeCustomerID); err != nil {
		return err
	}
	if eventCreatedAt > 0 {
		if err := upsertEventWatermark(ctx, tx, stripeCustomerID, eventCreatedAt); err != nil {
			return fmt.Errorf("updating event watermark: %w", err)
		}
	}
	return tx.Commit()
}

// isStaleEvent reports whether an event with the given timestamp should
// be dropped because a strictly newer event for the same customer has
// already been processed.
func isStaleEvent(ctx context.Context, tx *sql.Tx, stripeCustomerID string, eventCreatedAt int64) (bool, error) {
	var prev int64
	err := tx.QueryRowContext(ctx,
		`SELECT last_event_at FROM customer_events WHERE stripe_customer_id = ?`,
		stripeCustomerID).Scan(&prev)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return eventCreatedAt < prev, nil
}

// upsertEventWatermark records eventCreatedAt as the latest event seen
// for stripeCustomerID. The MAX guards against a race where a parallel
// transaction has already raised the watermark.
func upsertEventWatermark(ctx context.Context, tx *sql.Tx, stripeCustomerID string, eventCreatedAt int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO customer_events (stripe_customer_id, last_event_at)
		 VALUES (?, ?)
		 ON CONFLICT(stripe_customer_id) DO UPDATE SET
		   last_event_at = MAX(customer_events.last_event_at, excluded.last_event_at)`,
		stripeCustomerID, eventCreatedAt)
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
