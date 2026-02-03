package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// FetchMode determines how repo status is fetched
type FetchMode int

const (
	FetchAll       FetchMode = iota // Fetch all repos (default)
	FetchOnDemand                   // Only fetch visible repos
	FetchFavorites                  // Only fetch favorites
)

// Config holds application configuration
type Config struct {
	GitDir            string    `json:"gitDir"`
	SetupComplete     bool      `json:"setupComplete"`
	FetchMode         FetchMode `json:"fetchMode"`
	BinaryPath        string    `json:"binaryPath,omitempty"`
	ShowPullResults   *bool     `json:"showPullResults,omitempty"`   // nil = true (default)
	MaxCommitsPerRepo int       `json:"maxCommitsPerRepo,omitempty"` // 0 = 5 (default)
	FetchDelayMs      int       `json:"fetchDelayMs,omitempty"`      // 0 = 50 (default), delay between fetch/pull operations
}

func (c Config) GetShowPullResults() bool {
	if c.ShowPullResults == nil {
		return true // default
	}
	return *c.ShowPullResults
}

func (c Config) GetMaxCommitsPerRepo() int {
	if c.MaxCommitsPerRepo <= 0 {
		return 5 // default
	}
	return c.MaxCommitsPerRepo
}

func (c Config) GetFetchDelayMs() int {
	if c.FetchDelayMs <= 0 {
		return 50 // default 50ms
	}
	return c.FetchDelayMs
}

// GroupsFile represents the groups storage format
type GroupsFile struct {
	Groups []Group `json:"groups"`
}

func getConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "guppi")
}

func getFavoritesPath() string {
	return filepath.Join(getConfigDir(), "favorites.json")
}

func getConfigPath() string {
	return filepath.Join(getConfigDir(), "config.json")
}

func getGroupsPath() string {
	return filepath.Join(getConfigDir(), "groups.json")
}

func getGotoFilePath() string {
	return filepath.Join(getConfigDir(), ".goto")
}

func loadConfig() Config {
	var config Config

	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		return config
	}

	json.Unmarshal(data, &config)
	return config
}

func saveConfigFull(config Config) {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return
	}

	os.MkdirAll(getConfigDir(), 0755)
	os.WriteFile(getConfigPath(), data, 0644)
}

func saveConfig(gitDir string) {
	config := loadConfig()
	config.GitDir = gitDir
	saveConfigFull(config)
}

func loadFavorites() map[string]bool {
	favorites := make(map[string]bool)

	data, err := os.ReadFile(getFavoritesPath())
	if err != nil {
		return favorites
	}

	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return favorites
	}

	for _, path := range paths {
		favorites[path] = true
	}
	return favorites
}

func saveFavorites(favorites map[string]bool) {
	var paths []string
	for path, isFav := range favorites {
		if isFav {
			paths = append(paths, path)
		}
	}

	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return
	}

	os.MkdirAll(getConfigDir(), 0755)
	os.WriteFile(getFavoritesPath(), data, 0644)
}

func loadGroups() []Group {
	var groupsFile GroupsFile

	data, err := os.ReadFile(getGroupsPath())
	if err != nil {
		return []Group{}
	}

	if err := json.Unmarshal(data, &groupsFile); err != nil {
		return []Group{}
	}

	return groupsFile.Groups
}

func saveGroups(groups []Group) {
	// Filter out built-in groups (Favorites) from saving
	var toSave []Group
	for _, g := range groups {
		if !g.IsBuiltIn {
			toSave = append(toSave, g)
		}
	}

	groupsFile := GroupsFile{Groups: toSave}
	data, err := json.MarshalIndent(groupsFile, "", "  ")
	if err != nil {
		return
	}

	os.MkdirAll(getConfigDir(), 0755)
	os.WriteFile(getGroupsPath(), data, 0644)
}

func buildGroupsMap(groups []Group) map[string]*Group {
	m := make(map[string]*Group)
	for i := range groups {
		m[groups[i].Name] = &groups[i]
	}
	return m
}
