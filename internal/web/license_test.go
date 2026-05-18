package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/store"
)

// alwaysOK is the sentinel downstream handler the tests gate. It records
// that it was called and returns 200.
type alwaysOK struct {
	called bool
}

func (a *alwaysOK) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	a.called = true
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// staticStore wraps a LicenseStore so tests can pass it to RequirePaid,
// which expects a resolver func() rather than a value.
func staticStore(ls store.LicenseStore) licenseStoreFunc {
	return func() store.LicenseStore { return ls }
}

func TestRequirePaid_FreeTierEnabled_PassesThrough(t *testing.T) {
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "true")
	ls := store.NewMemoryLicenseStore()
	next := &alwaysOK{}
	h := RequirePaid(staticStore(ls))(next)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !next.called {
		t.Fatalf("free-tier enabled: want pass-through 200, got %d called=%v", w.Code, next.called)
	}
}

func TestRequirePaid_FreeTierEnabled_DefaultEnvVar(t *testing.T) {
	// Explicitly unset — the rule says default is enabled.
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "")
	ls := store.NewMemoryLicenseStore()
	next := &alwaysOK{}
	h := RequirePaid(staticStore(ls))(next)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !next.called {
		t.Fatalf("unset env: want pass-through 200, got %d called=%v", w.Code, next.called)
	}
}

func TestRequirePaid_WallOn_PaidKey_PassesThrough(t *testing.T) {
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "false")
	ls := store.NewMemoryLicenseStore()
	if err := ls.Grant(t.Context(), "ck_paid", "p@x", "cus_paid"); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	next := &alwaysOK{}
	h := RequirePaid(staticStore(ls))(next)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	req.Header.Set("Authorization", "Bearer ck_paid")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !next.called {
		t.Fatalf("paid key: want 200, got %d called=%v", w.Code, next.called)
	}
}

func TestRequirePaid_WallOn_FreeKey_402(t *testing.T) {
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "false")
	ls := store.NewMemoryLicenseStore()

	next := &alwaysOK{}
	h := RequirePaid(staticStore(ls))(next)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	req.Header.Set("Authorization", "Bearer ck_not_paid")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("free key: want 402, got %d", w.Code)
	}
	if next.called {
		t.Fatal("downstream handler must not run on 402")
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v body=%q", err, w.Body.String())
	}
	if body["error"] != "paid subscription required" {
		t.Errorf("error: want %q, got %q", "paid subscription required", body["error"])
	}
	if !strings.Contains(body["upgrade_url"], "cerebra") {
		t.Errorf("upgrade_url should mention cerebra, got %q", body["upgrade_url"])
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type: want application/json, got %q", got)
	}
}

func TestRequirePaid_WallOn_MissingHeader_402(t *testing.T) {
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "false")
	ls := store.NewMemoryLicenseStore()

	next := &alwaysOK{}
	h := RequirePaid(staticStore(ls))(next)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	// No Authorization header.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("missing header: want 402, got %d", w.Code)
	}
	if next.called {
		t.Fatal("downstream must not run when header is missing")
	}
}

func TestRequirePaid_WallOn_MalformedHeader_402(t *testing.T) {
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "false")
	ls := store.NewMemoryLicenseStore()
	if err := ls.Grant(t.Context(), "ck_paid", "p@x", "cus_paid"); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	next := &alwaysOK{}
	h := RequirePaid(staticStore(ls))(next)

	// Missing scheme — just the raw key.
	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	req.Header.Set("Authorization", "ck_paid")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("malformed header: want 402, got %d", w.Code)
	}
	if next.called {
		t.Fatal("downstream must not run on malformed header")
	}
}

func TestRequirePaid_BearerSchemeCaseInsensitive(t *testing.T) {
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "false")
	ls := store.NewMemoryLicenseStore()
	if err := ls.Grant(t.Context(), "ck_case", "p@x", "cus_case"); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	next := &alwaysOK{}
	h := RequirePaid(staticStore(ls))(next)

	for _, scheme := range []string{"Bearer ", "bearer ", "BEARER ", "BeArEr "} {
		next.called = false
		req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
		req.Header.Set("Authorization", scheme+"ck_case")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("scheme=%q: want 200, got %d", scheme, w.Code)
		}
	}
}

func TestRequirePaid_NilStore_PassesThrough(t *testing.T) {
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "false")
	next := &alwaysOK{}
	h := RequirePaid(nil)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !next.called {
		t.Fatalf("nil store: want pass-through 200, got %d called=%v", w.Code, next.called)
	}
}

func TestRequirePaid_QueryParamKey_PaidPasses(t *testing.T) {
	// Browser EventSource cannot set Authorization headers, so we accept
	// the API key as a `key` query param as a fallback.
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "false")
	ls := store.NewMemoryLicenseStore()
	if err := ls.Grant(t.Context(), "ck_query", "q@x", "cus_query"); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	next := &alwaysOK{}
	h := RequirePaid(staticStore(ls))(next)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream?q=hi&key=ck_query", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !next.called {
		t.Fatalf("query-param key: want 200, got %d called=%v body=%q", w.Code, next.called, w.Body.String())
	}
}

func TestRequirePaid_QueryParamKey_HeaderTakesPrecedence(t *testing.T) {
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "false")
	ls := store.NewMemoryLicenseStore()
	if err := ls.Grant(t.Context(), "ck_header", "h@x", "cus_header"); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	next := &alwaysOK{}
	h := RequirePaid(staticStore(ls))(next)

	// Header is the paid key; query is a different (free) key. Header
	// should win and the request should pass.
	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream?q=hi&key=ck_free", nil)
	req.Header.Set("Authorization", "Bearer ck_header")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !next.called {
		t.Fatalf("header should win over query: want 200, got %d called=%v", w.Code, next.called)
	}
}

func TestRequirePaid_LateBoundStore(t *testing.T) {
	// The resolver pattern means a store wired in AFTER route registration
	// (e.g. Server.WithLicenseStore) takes effect on the next request.
	t.Setenv("CEREBRA_FREE_TIER_ENABLED", "false")
	var ls store.LicenseStore
	next := &alwaysOK{}
	h := RequirePaid(func() store.LicenseStore { return ls })(next)

	// First request: resolver returns nil → pass-through (no wall yet).
	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pre-wire request should pass through, got %d", w.Code)
	}

	// Now wire the store late. Same request should be rejected.
	ls = store.NewMemoryLicenseStore()
	next.called = false
	req = httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("post-wire request without auth should be 402, got %d", w.Code)
	}
}

func TestFreeTierEnabled_Toggles(t *testing.T) {
	cases := map[string]bool{
		"":      true,
		"true":  true,
		"TRUE":  true,
		"1":     true,
		"on":    true,
		"yes":   true,
		"false": false,
		"FALSE": false,
		"0":     false,
		"off":   false,
		"no":    false,
	}
	for v, want := range cases {
		t.Setenv("CEREBRA_FREE_TIER_ENABLED", v)
		if got := freeTierEnabled(); got != want {
			t.Errorf("freeTierEnabled(%q): want %v, got %v", v, want, got)
		}
	}
}
