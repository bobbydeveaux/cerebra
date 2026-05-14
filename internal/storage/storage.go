package storage

import (
	"context"
	"fmt"
	"strings"
)

type Storage interface {
	Upload(ctx context.Context, localPath string) error
	Download(ctx context.Context, localPath string) error
}

func New(uri string) (Storage, error) {
	switch {
	case uri == "" || !strings.Contains(uri, "://"):
		return &LocalStorage{}, nil
	case strings.HasPrefix(uri, "gcs://"):
		parts := strings.SplitN(strings.TrimPrefix(uri, "gcs://"), "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid GCS URI: %s (expected gcs://bucket/path)", uri)
		}
		return NewGCS(parts[0], parts[1])
	case strings.HasPrefix(uri, "s3://"):
		parts := strings.SplitN(strings.TrimPrefix(uri, "s3://"), "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid S3 URI: %s (expected s3://bucket/path)", uri)
		}
		return NewS3(parts[0], parts[1])
	default:
		return nil, fmt.Errorf("unsupported storage URI scheme: %s", uri)
	}
}
