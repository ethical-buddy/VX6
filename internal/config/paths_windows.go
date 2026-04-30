//go:build windows

package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func DefaultPath() (string, error) {
	if p := os.Getenv("VX6_CONFIG_PATH"); p != "" {
		return p, nil
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	return filepath.Join(configRoot, "vx6", "config.json"), nil
}

func DefaultDataDir() (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve data directory: %w", err)
	}
	return filepath.Join(cacheRoot, "vx6"), nil
}

func DefaultDownloadDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, "Downloads"), nil
}
