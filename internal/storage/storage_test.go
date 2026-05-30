package storage

import (
	"context"
	"strings"
	"testing"
)

// TestNew_LocalDefaults verifies that empty or non-scheme URIs return a
// LocalStorage handle. This is the default path used by the CLI when no
// remote storage is configured.
func TestNew_LocalDefaults(t *testing.T) {
	cases := []struct {
		name string
		uri  string
	}{
		{"empty string", ""},
		{"plain path", "/tmp/data"},
		{"relative path", "./data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(tc.uri)
			if err != nil {
				t.Fatalf("New(%q) returned error: %v", tc.uri, err)
			}
			if _, ok := s.(*LocalStorage); !ok {
				t.Fatalf("New(%q) returned %T, want *LocalStorage", tc.uri, s)
			}
		})
	}
}

// TestNew_GCSParsing checks that URIs in the gcs:// form are routed to the
// GCS constructor and that malformed URIs return a parse error before any
// network call. NewGCS itself may or may not succeed depending on whether
// the test environment has GCP credentials available, so the success branch
// only checks the typed handle when err is nil.
func TestNew_GCSParsing(t *testing.T) {
	t.Run("malformed missing object", func(t *testing.T) {
		_, err := New("gcs://bucketonly")
		if err == nil {
			t.Fatal("New(gcs://bucketonly) returned no error, want parse error")
		}
		if !strings.Contains(err.Error(), "invalid GCS URI") {
			t.Fatalf("error %q does not contain 'invalid GCS URI'", err)
		}
	})

	t.Run("well-formed routes to GCS constructor", func(t *testing.T) {
		s, err := New("gcs://bucket/path/to/object")
		if err != nil {
			// Acceptable: NewGCS hit credential discovery and failed. The
			// route through New() is still covered.
			t.Logf("NewGCS unavailable in this env: %v", err)
			return
		}
		if _, ok := s.(*GCSStorage); !ok {
			t.Fatalf("New(gcs://...) returned %T, want *GCSStorage", s)
		}
	})
}

// TestNew_S3Parsing checks that URIs in the s3:// form are routed to the S3
// constructor and that malformed URIs return a parse error before any
// network call.
func TestNew_S3Parsing(t *testing.T) {
	t.Run("malformed missing key", func(t *testing.T) {
		_, err := New("s3://bucketonly")
		if err == nil {
			t.Fatal("New(s3://bucketonly) returned no error, want parse error")
		}
		if !strings.Contains(err.Error(), "invalid S3 URI") {
			t.Fatalf("error %q does not contain 'invalid S3 URI'", err)
		}
	})

	t.Run("well-formed routes to S3 constructor", func(t *testing.T) {
		s, err := New("s3://bucket/key/inside")
		if err != nil {
			t.Logf("NewS3 unavailable in this env: %v", err)
			return
		}
		if _, ok := s.(*S3Storage); !ok {
			t.Fatalf("New(s3://...) returned %T, want *S3Storage", s)
		}
	})
}

// TestNew_UnsupportedScheme checks that any scheme other than gcs:// or
// s3:// is rejected with a clear error.
func TestNew_UnsupportedScheme(t *testing.T) {
	cases := []string{
		"http://example.com/path",
		"azure://account/container/blob",
		"file://etc/hosts",
	}
	for _, uri := range cases {
		t.Run(uri, func(t *testing.T) {
			_, err := New(uri)
			if err == nil {
				t.Fatalf("New(%q) returned no error, want unsupported scheme", uri)
			}
			if !strings.Contains(err.Error(), "unsupported storage URI scheme") {
				t.Fatalf("error %q does not contain 'unsupported storage URI scheme'", err)
			}
		})
	}
}

// TestStorageInterfaceSatisfied is a compile-time check that the three
// implementations all satisfy the Storage interface. The runtime call here
// only exercises LocalStorage so it does not require any cloud credentials.
func TestStorageInterfaceSatisfied(t *testing.T) {
	var _ Storage = (*LocalStorage)(nil)
	var _ Storage = (*GCSStorage)(nil)
	var _ Storage = (*S3Storage)(nil)

	// Smoke-test the interface dispatch on the local backend.
	var s Storage = &LocalStorage{}
	if err := s.Upload(context.Background(), "/tmp/whatever"); err != nil {
		t.Fatalf("LocalStorage.Upload via interface returned %v, want nil", err)
	}
	if err := s.Download(context.Background(), "/tmp/whatever"); err != nil {
		t.Fatalf("LocalStorage.Download via interface returned %v, want nil", err)
	}
}
