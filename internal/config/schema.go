package config

// SyncEntry represents one named source-to-targets mapping.
type SyncEntry struct {
	Name    string   `json:"name"`
	Source  string   `json:"source"`
	Targets []string `json:"targets"`
	Enabled bool     `json:"enabled"`
}

// Config is the root config.json shape.
type Config struct {
	Version int         `json:"version"`
	Entries []SyncEntry `json:"entries"`
}
