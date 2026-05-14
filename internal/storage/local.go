package storage

import "context"

type LocalStorage struct{}

func (l *LocalStorage) Upload(ctx context.Context, localPath string) error {
	// No-op for local storage — the file is already on disk
	return nil
}

func (l *LocalStorage) Download(ctx context.Context, localPath string) error {
	// No-op for local storage — the file is already on disk
	return nil
}
