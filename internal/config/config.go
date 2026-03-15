package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Path returns ~/.babi/sync_config.json. Panics if the home directory cannot
// be determined (unrecoverable environment misconfiguration).
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("babi: cannot determine home directory: " + err.Error())
	}
	return filepath.Join(home, ".babi", "sync_config.json")
}

// Load reads config from path, creating a default if it doesn't exist.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Version: 1, Entries: []SyncEntry{}}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes config to path atomically.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
