package storage

import (
	"context"
	"fmt"
	"io"
	"os"

	gcs "cloud.google.com/go/storage"
)

type GCSStorage struct {
	bucket string
	object string
	client *gcs.Client
}

func NewGCS(bucket, object string) (*GCSStorage, error) {
	client, err := gcs.NewClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("creating GCS client: %w", err)
	}
	return &GCSStorage{bucket: bucket, object: object, client: client}, nil
}

func (g *GCSStorage) Upload(ctx context.Context, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening local file: %w", err)
	}
	defer f.Close()

	w := g.client.Bucket(g.bucket).Object(g.object).NewWriter(ctx)
	if _, err := io.Copy(w, f); err != nil {
		w.Close()
		return fmt.Errorf("uploading to GCS: %w", err)
	}
	return w.Close()
}

func (g *GCSStorage) Download(ctx context.Context, localPath string) error {
	r, err := g.client.Bucket(g.bucket).Object(g.object).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("reading from GCS: %w", err)
	}
	defer r.Close()

	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating local file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("downloading from GCS: %w", err)
	}
	return nil
}
