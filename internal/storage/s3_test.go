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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// newTestS3Client builds an S3 client wired to the supplied httptest server.
// Path-style addressing keeps the bucket out of the host header so the stub
// receives /bucket/key directly.
func newTestS3Client(endpoint string) *s3.Client {
	return s3.New(s3.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		UsePathStyle: true,
	})
}

// TestS3Storage_Upload_HappyPath stubs an S3 PUT and confirms the wrapper
// reads the local file and forwards the bytes.
func TestS3Storage_Upload_HappyPath(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "src.txt")
	want := "hello world"
	if err := os.WriteFile(local, []byte(want), 0o600); err != nil {
		t.Fatalf("writing local file: %v", err)
	}

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected method %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := &S3Storage{
		bucket: "bucket",
		key:    "key",
		client: newTestS3Client(srv.URL),
	}
	if err := st.Upload(context.Background(), local); err != nil {
		t.Fatalf("Upload returned %v", err)
	}
	if gotBody != want {
		t.Fatalf("server received %q, want %q", gotBody, want)
	}
}

// TestS3Storage_Upload_MissingLocalFile checks the os.Open failure is
// wrapped with the documented prefix and no PUT is issued.
func TestS3Storage_Upload_MissingLocalFile(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := &S3Storage{
		bucket: "bucket",
		key:    "key",
		client: newTestS3Client(srv.URL),
	}
	err := st.Upload(context.Background(), "/nonexistent/path/to/file.bin")
	if err == nil {
		t.Fatal("Upload with missing local file returned nil error")
	}
	if !strings.Contains(err.Error(), "opening local file") {
		t.Fatalf("error %q does not contain 'opening local file'", err)
	}
	if hit {
		t.Fatal("server received a request despite open failure")
	}
}

// TestS3Storage_Upload_ServerError covers the PutObject error branch.
func TestS3Storage_Upload_ServerError(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(local, []byte("data"), 0o600); err != nil {
		t.Fatalf("writing local file: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	st := &S3Storage{
		bucket: "bucket",
		key:    "key",
		client: newTestS3Client(srv.URL),
	}
	err := st.Upload(context.Background(), local)
	if err == nil {
		t.Fatal("Upload returned nil for 500 response")
	}
	if !strings.Contains(err.Error(), "uploading to S3") {
		t.Fatalf("error %q does not contain 'uploading to S3'", err)
	}
}

// TestS3Storage_Download_HappyPath stubs an S3 GET and confirms the
// wrapper writes the bytes to the local file.
func TestS3Storage_Download_HappyPath(t *testing.T) {
	want := "downloaded payload"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(want))
	}))
	defer srv.Close()

	dir := t.TempDir()
	local := filepath.Join(dir, "dst.txt")
	st := &S3Storage{
		bucket: "bucket",
		key:    "key",
		client: newTestS3Client(srv.URL),
	}
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

// TestS3Storage_Download_ServerError covers the GetObject error branch.
func TestS3Storage_Download_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	local := filepath.Join(dir, "dst.txt")
	st := &S3Storage{
		bucket: "bucket",
		key:    "key",
		client: newTestS3Client(srv.URL),
	}
	err := st.Download(context.Background(), local)
	if err == nil {
		t.Fatal("Download returned nil for 404 response")
	}
	if !strings.Contains(err.Error(), "downloading from S3") {
		t.Fatalf("error %q does not contain 'downloading from S3'", err)
	}
}

// TestS3Storage_Download_LocalCreateError stubs a successful GET but
// targets a local path that cannot be created (under a non-existent
// directory) so the os.Create failure branch is hit.
func TestS3Storage_Download_LocalCreateError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()

	st := &S3Storage{
		bucket: "bucket",
		key:    "key",
		client: newTestS3Client(srv.URL),
	}
	err := st.Download(context.Background(), "/nonexistent/dir/that/does/not/exist/dst.txt")
	if err == nil {
		t.Fatal("Download with bad local path returned nil")
	}
	if !strings.Contains(err.Error(), "creating local file") {
		t.Fatalf("error %q does not contain 'creating local file'", err)
	}
}
