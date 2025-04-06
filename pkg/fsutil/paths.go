package fsutil

import (
	"os"
	"path/filepath"
)

// ResolvePath resolves a relative path to an absolute path
func ResolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return filepath.Join(cwd, path), nil
}
