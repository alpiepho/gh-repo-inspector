package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// CloneRecord tracks a completed clone operation.
type CloneRecord struct {
	Name         string     `json:"name"`
	Path         string     `json:"path"`
	ClonedAt     time.Time  `json:"clonedAt"`
	AllBranches  bool       `json:"allBranches"`
	LastPulledAt *time.Time `json:"lastPulledAt,omitempty"`
}

// Config is the persistent application state.
type Config struct {
	LastClonePath string        `json:"lastClonePath"`
	CloneHistory  []CloneRecord `json:"cloneHistory"`
	GitLabURL     string        `json:"gitLabURL,omitempty"`
	GitLabToken   string        `json:"gitLabToken,omitempty"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gh-repo-inspector", "state.json"), nil
}

// Load reads the config from disk. Returns an empty config if the file doesn't exist.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return &Config{}, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return &Config{}, nil
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return &Config{}, nil
	}
	return &c, nil
}

// Save writes the config to disk.
func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// RecordClone adds or updates a clone record and saves the config.
func (c *Config) RecordClone(name, path string, allBranches bool) {
	for i, r := range c.CloneHistory {
		if r.Name == name {
			c.CloneHistory[i] = CloneRecord{Name: name, Path: path, ClonedAt: time.Now(), AllBranches: allBranches}
			return
		}
	}
	c.CloneHistory = append(c.CloneHistory, CloneRecord{
		Name: name, Path: path, ClonedAt: time.Now(), AllBranches: allBranches,
	})
}

// FindClone returns the most recent clone record for a repo, if any.
func (c *Config) FindClone(name string) (CloneRecord, bool) {
	for i := len(c.CloneHistory) - 1; i >= 0; i-- {
		if c.CloneHistory[i].Name == name {
			return c.CloneHistory[i], true
		}
	}
	return CloneRecord{}, false
}

// RecordPull updates the LastPulledAt timestamp for a named repo.
func (c *Config) RecordPull(name string) {
	now := time.Now()
	for i, r := range c.CloneHistory {
		if r.Name == name {
			c.CloneHistory[i].LastPulledAt = &now
			return
		}
	}
}
