package storage

import (
	"context"
	"testing"
)

// TestLocalStorage_UploadDownloadNoop verifies the no-op semantics of the
// local backend. The contract is documented in local.go: the file is already
// on disk, so Upload and Download simply return nil regardless of context or
// path. These checks pin the contract so a future change that introduces an
// error path will need to update the tests deliberately.
func TestLocalStorage_UploadDownloadNoop(t *testing.T) {
	l := &LocalStorage{}

	t.Run("upload returns nil for any path", func(t *testing.T) {
		for _, p := range []string{"", "/dev/null", "/nonexistent/path/somewhere"} {
			if err := l.Upload(context.Background(), p); err != nil {
				t.Fatalf("Upload(%q) returned %v, want nil", p, err)
			}
		}
	})

	t.Run("download returns nil for any path", func(t *testing.T) {
		for _, p := range []string{"", "/dev/null", "/nonexistent/path/somewhere"} {
			if err := l.Download(context.Background(), p); err != nil {
				t.Fatalf("Download(%q) returned %v, want nil", p, err)
			}
		}
	})

	t.Run("nil context is accepted", func(t *testing.T) {
		// LocalStorage does not touch the context so a nil ctx must not
		// cause a panic. This guards against an accidental ctx.Done()
		// call being added without a nil check.
		//nolint:staticcheck // SA1012 - intentionally passing a nil context to confirm the no-op contract.
		if err := l.Upload(nil, "/tmp/x"); err != nil {
			t.Fatalf("Upload with nil ctx returned %v, want nil", err)
		}
		//nolint:staticcheck // SA1012 - intentionally passing a nil context to confirm the no-op contract.
		if err := l.Download(nil, "/tmp/x"); err != nil {
			t.Fatalf("Download with nil ctx returned %v, want nil", err)
		}
	})
}
