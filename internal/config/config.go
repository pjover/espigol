// Package config resolves the espigol home directory and loads settings.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveHome returns the espigol home directory: $ESPIGOL_HOME when set,
// otherwise <user-home>/.config/espigol.
func ResolveHome() (string, error) {
	if h := os.Getenv("ESPIGOL_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving user home: %w", err)
	}
	return filepath.Join(home, ".config", "espigol"), nil
}
