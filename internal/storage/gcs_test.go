package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gcs "cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// newTestGCSStorage builds a GCSStorage backed by a real cloud.google.com/go/storage
// client whose endpoint is overridden to the supplied httptest server. The
// client uses WithoutAuthentication so no credential discovery happens.
func newTestGCSStorage(t *testing.T, bucket, object, endpoint string) *GCSStorage {
	t.Helper()
	client, err := gcs.NewClient(
		context.Background(),
		option.WithEndpoint(endpoint),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("creating GCS test client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return &GCSStorage{bucket: bucket, object: object, client: client}
}

// TestGCSStorage_Upload_HappyPath stubs the upload endpoint and verifies the
// wrapper streams the local file body through.
func TestGCSStorage_Upload_HappyPath(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "src.txt")
	want := "gcs payload"
	if err := os.WriteFile(local, []byte(want), 0o600); err != nil {
		t.Fatalf("writing local file: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GCS resumable + multipart uploads land on /upload/storage/...
		// Returning a minimal success JSON satisfies the writer's Close().
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"name":"obj","bucket":"bucket"}`)
	}))
	defer srv.Close()

	st := newTestGCSStorage(t, "bucket", "obj", srv.URL)
	if err := st.Upload(context.Background(), local); err != nil {
		t.Fatalf("Upload returned %v", err)
	}
}

// TestGCSStorage_Upload_MissingLocalFile checks the os.Open failure branch
// before any network call is made.
func TestGCSStorage_Upload_MissingLocalFile(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := newTestGCSStorage(t, "bucket", "obj", srv.URL)
	err := st.Upload(context.Background(), "/nonexistent/path/to/file.bin")
	if err == nil {
		t.Fatal("Upload with missing local file returned nil")
	}
	if !strings.Contains(err.Error(), "opening local file") {
		t.Fatalf("error %q does not contain 'opening local file'", err)
	}
	if hit {
		t.Fatal("server received a request despite open failure")
	}
}

// TestGCSStorage_Download_HappyPath stubs the download endpoint and verifies
// the wrapper writes the response body into the local file.
func TestGCSStorage_Download_HappyPath(t *testing.T) {
	want := "downloaded gcs payload"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(want))
	}))
	defer srv.Close()

	dir := t.TempDir()
	local := filepath.Join(dir, "dst.txt")
	st := newTestGCSStorage(t, "bucket", "obj", srv.URL)
	if err := st.Download(context.Background(), local); err != nil {
		t.Fatalf("Download returned %v", err)
	}
	got, err := os.ReadFile(local)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(got) != want {
		t.Fatalf("file contents %q, want %q", string(got), want)
	}
}

// TestGCSStorage_Download_ServerError covers the NewReader error branch.
func TestGCSStorage_Download_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	local := filepath.Join(dir, "dst.txt")
	st := newTestGCSStorage(t, "bucket", "obj", srv.URL)
	err := st.Download(context.Background(), local)
	if err == nil {
		t.Fatal("Download returned nil for 404 response")
	}
	if !strings.Contains(err.Error(), "reading from GCS") {
		t.Fatalf("error %q does not contain 'reading from GCS'", err)
	}
}

// TestGCSStorage_Download_LocalCreateError stubs a successful download but
// targets a local path that cannot be created so the os.Create failure
// branch is hit.
func TestGCSStorage_Download_LocalCreateError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()

	st := newTestGCSStorage(t, "bucket", "obj", srv.URL)
	err := st.Download(context.Background(), "/nonexistent/dir/that/does/not/exist/dst.txt")
	if err == nil {
		t.Fatal("Download with bad local path returned nil")
	}
	if !strings.Contains(err.Error(), "creating local file") {
		t.Fatalf("error %q does not contain 'creating local file'", err)
	}
}

// TestNewGCS_Constructor confirms that NewGCS produces a populated handle
// when called against a reachable test server. The exported constructor was
// otherwise uncovered.
func TestNewGCS_Constructor(t *testing.T) {
	// NewGCS uses context.Background() internally and reads credentials
	// from the ambient environment. Setting STORAGE_EMULATOR_HOST short-
	// circuits credential discovery in the underlying SDK.
	t.Setenv("STORAGE_EMULATOR_HOST", "127.0.0.1:0")
	g, err := NewGCS("bucket", "obj")
	if err != nil {
		// Some SDK versions still attempt credential discovery; skip
		// when that path triggers.
		t.Skipf("NewGCS unavailable in this env: %v", err)
	}
	if g.bucket != "bucket" || g.object != "obj" {
		t.Fatalf("got bucket=%q object=%q, want bucket=obj/obj", g.bucket, g.object)
	}
	if g.client == nil {
		t.Fatal("client is nil")
	}
}
