package mcp

// registry.go implements the per-user store cache for the multi-tenant HTTP
// transport. Isolation is structural, not query-level: each user gets their own
// SQLite file at <dataDir>/<user_id>/jor-el.db and their own *Server wrapping
// only that store. There is no shared store and no code path that can query
// across two users' databases — a request for user A holds a *Server that was
// constructed from user A's file and nothing else.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// userIDPattern constrains user ids to a safe, filesystem-friendly charset.
// This is a hard gate against path traversal: a user id is used as a directory
// name, so "..", "/", and similar must never reach the filesystem. Anything not
// matching is rejected before any store is opened.
var userIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// validUserID reports whether id is safe to use as a per-user directory name.
func validUserID(id string) bool {
	return userIDPattern.MatchString(id)
}

// storeOpener opens a store at the given path. It is a seam so tests can inject
// an in-memory/temp store without the real sqlite-vec path; production uses
// store.New.
type storeOpener func(dbPath string, dimensions int) (store.Store, error)

func defaultOpener(dbPath string, dimensions int) (store.Store, error) {
	return store.New(dbPath, dimensions)
}

// Registry lazily opens and caches one *Server per user id. Servers (and their
// underlying stores) are opened once and reused; concurrent requests for the
// same user share a single store, which is correct for SQLite readers and
// keeps us within SQLite's single-writer constraint per file.
type Registry struct {
	dataDir    string
	embedder   embedder.Embedder
	dimensions int
	open       storeOpener

	mu      sync.Mutex
	servers map[string]*Server
}

// NewRegistry creates a registry rooted at dataDir. Each user's database lives
// at <dataDir>/<user_id>/jor-el.db. The embedder is shared (it is stateless
// w.r.t. tenancy — it only turns query text into vectors).
func NewRegistry(dataDir string, emb embedder.Embedder) *Registry {
	return &Registry{
		dataDir:    dataDir,
		embedder:   emb,
		dimensions: emb.Dimensions(),
		open:       defaultOpener,
		servers:    make(map[string]*Server),
	}
}

// dbPathFor returns the on-disk path for a user's database. It is only called
// after validUserID has passed, so userID cannot contain path separators.
func (r *Registry) dbPathFor(userID string) string {
	return filepath.Join(r.dataDir, userID, "jor-el.db")
}

// ServerFor returns the *Server bound to userID's store, opening and caching it
// on first use. It returns an error if userID is invalid or the store cannot be
// opened. The returned *Server can ONLY see userID's data.
func (r *Registry) ServerFor(userID string) (*Server, error) {
	if !validUserID(userID) {
		return nil, fmt.Errorf("invalid user id")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if srv, ok := r.servers[userID]; ok {
		return srv, nil
	}

	st, err := r.open(r.dbPathFor(userID), r.dimensions)
	if err != nil {
		return nil, fmt.Errorf("opening store for user: %w", err)
	}

	srv := NewServer(st, r.embedder)
	r.servers[userID] = srv
	return srv, nil
}

// Close closes every cached store. Safe to call once at shutdown.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var firstErr error
	for id, srv := range r.servers {
		if err := srv.store.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(r.servers, id)
	}
	return firstErr
}
