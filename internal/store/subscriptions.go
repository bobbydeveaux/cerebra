package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// subscriptionsSchemaSQL defines the table backing the AgentOps paid tier.
//
// Cerebra is a single-user local-first tool (stackramp.yaml sets
// database: false; all state lives in the SQLite jor-el.db). There is no
// per-request user identity to attach a subscription to, so the gate is
// instance-level: if any row is active, this Cerebra instance is licensed.
// The Stripe customer ID is the natural key so repeated webhook deliveries
// for the same customer upsert rather than duplicate.
const subscriptionsSchemaSQL = `
CREATE TABLE IF NOT EXISTS subscriptions (
    stripe_customer_id  TEXT PRIMARY KEY,
    status              TEXT NOT NULL DEFAULT 'active',
    stripe_session_id   TEXT NOT NULL DEFAULT '',
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status);
`

// Subscription is a single Stripe customer paid-tier state.
type Subscription struct {
	StripeCustomerID string
	Status           string
	StripeSessionID  string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Subscription status values.
const (
	SubscriptionActive   = "active"
	SubscriptionInactive = "inactive"
)

// SetSubscriptionActive marks a Stripe customer as having an active paid
// subscription. Called from the checkout.session.completed webhook. The
// customer ID is the upsert key so duplicate webhook deliveries are
// idempotent. A blank customerID is rejected: an active row with no key
// would be impossible to deactivate later and would silently unlock the
// instance forever.
func (s *SQLiteStore) SetSubscriptionActive(ctx context.Context, customerID, sessionID string) error {
	if customerID == "" {
		return errors.New("subscription: empty stripe customer id")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO subscriptions (stripe_customer_id, status, stripe_session_id, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(stripe_customer_id) DO UPDATE SET
			status            = excluded.status,
			stripe_session_id = excluded.stripe_session_id,
			updated_at        = CURRENT_TIMESTAMP`,
		customerID, SubscriptionActive, sessionID,
	)
	if err != nil {
		return fmt.Errorf("activating subscription: %w", err)
	}
	return nil
}

// SetSubscriptionInactive marks a Stripe customer subscription as ended.
// Called from the customer.subscription.deleted webhook. Deactivating a
// customer that was never recorded is a no-op (no row updated), which is
// the correct behaviour for an out-of-order or replayed delete event.
func (s *SQLiteStore) SetSubscriptionInactive(ctx context.Context, customerID string) error {
	if customerID == "" {
		return errors.New("subscription: empty stripe customer id")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE stripe_customer_id = ?`,
		SubscriptionInactive, customerID,
	)
	if err != nil {
		return fmt.Errorf("deactivating subscription: %w", err)
	}
	return nil
}

// HasActiveSubscription reports whether this Cerebra instance has at least
// one active subscription. This is the instance-level licence check the
// RequirePaid gate reads.
func (s *SQLiteStore) HasActiveSubscription(ctx context.Context) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM subscriptions WHERE status = ? LIMIT 1`,
		SubscriptionActive,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking active subscription: %w", err)
	}
	return true, nil
}

// GetSubscription returns a single customer subscription, or nil if no
// row exists. Used by tests and future per-customer reporting.
func (s *SQLiteStore) GetSubscription(ctx context.Context, customerID string) (*Subscription, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT stripe_customer_id, status, stripe_session_id, created_at, updated_at
		FROM subscriptions WHERE stripe_customer_id = ?`,
		customerID,
	)
	var sub Subscription
	err := row.Scan(&sub.StripeCustomerID, &sub.Status, &sub.StripeSessionID, &sub.CreatedAt, &sub.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting subscription: %w", err)
	}
	return &sub, nil
}
