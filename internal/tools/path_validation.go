package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathValidator provides safe path validation and normalization
type PathValidator struct {
	// AllowedPaths is a list of directories that are allowed for access
	// If empty, all paths are allowed (less secure)
	AllowedPaths []string
	// DenyPaths is a list of directories that are explicitly denied
	DenyPaths []string
}

// NewPathValidator creates a new path validator with default deny list
func NewPathValidator() *PathValidator {
	return &PathValidator{
		DenyPaths: []string{
			"/etc",
			"/sys",
			"/proc",
			"/dev",
			"/boot",
			"/root",
			"/.ssh",
			"/.gnupg",
		},
	}
}

// ValidatePath checks if a path is safe to access
func (v *PathValidator) ValidatePath(path string) (string, error) {
	// Expand home directory if needed
	expandedPath := path
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to expand home directory: %w", err)
		}
		expandedPath = filepath.Join(homeDir, path[1:])
	}

	// Clean and resolve the path to prevent traversal
	cleanPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	// Ensure the resolved path doesn't contain traversal attempts
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("path traversal detected in resolved path")
	}

	// Check against deny list
	for _, denyPath := range v.DenyPaths {
		// Resolve deny path too
		absDenyPath, _ := filepath.Abs(denyPath)
		if strings.HasPrefix(cleanPath, absDenyPath) {
			return "", fmt.Errorf("access to path %s is denied", denyPath)
		}
	}

	// Check against allow list if configured
	if len(v.AllowedPaths) > 0 {
		allowed := false
		for _, allowPath := range v.AllowedPaths {
			absAllowPath, _ := filepath.Abs(allowPath)
			if strings.HasPrefix(cleanPath, absAllowPath) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("path is not in allowed directories")
		}
	}

	// Additional security checks
	if err := v.checkSymlinkSafety(cleanPath); err != nil {
		return "", err
	}

	return cleanPath, nil
}

// checkSymlinkSafety ensures symlinks don't point to restricted areas
func (v *PathValidator) checkSymlinkSafety(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		// Path doesn't exist yet, which is okay for write operations
		return nil
	}

	// If it's a symlink, check where it points
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return fmt.Errorf("failed to read symlink: %w", err)
		}

		// Recursively validate the target
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}

		_, err = v.ValidatePath(target)
		if err != nil {
			return fmt.Errorf("symlink points to restricted location: %w", err)
		}
	}

	return nil
}

// Global path validator instance
var defaultPathValidator = NewPathValidator()

// ValidateAndCleanPath is a convenience function using the default validator
func ValidateAndCleanPath(path string) (string, error) {
	return defaultPathValidator.ValidatePath(path)
}
