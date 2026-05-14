package scanner

import (
	"crypto/sha256"
	"fmt"
	"os"
)

func FileContentHash(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(content)), nil
}
