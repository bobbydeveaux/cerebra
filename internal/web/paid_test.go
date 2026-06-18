package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakePaidChecker is a paidChecker test double.
type fakePaidChecker struct {
	active bool
	err    error
}

func (f fakePaidChecker) HasActiveSubscription(_ context.Context) (bool, error) {
	return f.active, f.err
}

// nextOK is a sentinel downstream handler that records it was reached.
func nextOK(reached *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func TestRequirePaid(t *testing.T) {
	const checkout = "https://buy.stripe.com/test_cerebra"

	tests := []struct {
		name           string
		gatingSecret   string
		checker        paidChecker
		wantStatus     int
		wantNextCalled bool
		wantCheckout   bool
	}{
		{
			name:           "fail open when STRIPE_WEBHOOK_SECRET unset",
			gatingSecret:   "",
			checker:        fakePaidChecker{active: false},
			wantStatus:     http.StatusOK,
			wantNextCalled: true,
		},
		{
			name:           "fail open when no checker wired",
			gatingSecret:   "whsec_x",
			checker:        nil,
			wantStatus:     http.StatusOK,
			wantNextCalled: true,
		},
		{
			name:           "paid path passes through",
			gatingSecret:   "whsec_x",
			checker:        fakePaidChecker{active: true},
			wantStatus:     http.StatusOK,
			wantNextCalled: true,
		},
		{
			name:           "unpaid path returns 402 with checkout url",
			gatingSecret:   "whsec_x",
			checker:        fakePaidChecker{active: false},
			wantStatus:     http.StatusPaymentRequired,
			wantNextCalled: false,
			wantCheckout:   true,
		},
		{
			name:           "checker error returns 503",
			gatingSecret:   "whsec_x",
			checker:        fakePaidChecker{err: errors.New("boom")},
			wantStatus:     http.StatusServiceUnavailable,
			wantNextCalled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STRIPE_WEBHOOK_SECRET", tc.gatingSecret)
			t.Setenv("STRIPE_CHECKOUT_URL", checkout)

			srv := &Server{paid: tc.checker}
			reached := false
			h := srv.RequirePaid(nextOK(&reached))

			req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
			w := httptest.NewRecorder()
			h(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status: got %d want %d", w.Code, tc.wantStatus)
			}
			if reached != tc.wantNextCalled {
				t.Fatalf("next called: got %v want %v", reached, tc.wantNextCalled)
			}
			if tc.wantCheckout {
				var body map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode 402 body: %v", err)
				}
				if body["checkout_url"] != checkout {
					t.Errorf("checkout_url: got %q want %q", body["checkout_url"], checkout)
				}
				if body["error"] == "" {
					t.Errorf("expected non-empty error message in 402 body")
				}
			}
		})
	}
}
