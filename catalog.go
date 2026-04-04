package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CatalogItem represents a furniture item with an assigned credit value
type CatalogItem struct {
	Name        string `json:"name"`         // internal name e.g. "club_sofa"
	DisplayName string `json:"display_name"` // human-friendly name e.g. "Club Sofa"
	Value       int    `json:"value"`        // credit value
}

var defaultCatalog = []CatalogItem{
	{Name: "club_sofa", DisplayName: "Club Sofa", Value: 0},
	{Name: "chair_plasty", DisplayName: "Chair Plasty", Value: 0},
}

func getCatalogFilePath() string {
	configDir, _ := os.UserConfigDir()
	configPath := filepath.Join(configDir, "Gamba-Suite")
	os.MkdirAll(configPath, 0700)
	return filepath.Join(configPath, "catalog.json")
}

// LoadCatalog reads the catalog from disk, returning defaults if the file doesn't exist.
func (a *App) LoadCatalog() []CatalogItem {
	file, err := os.Open(getCatalogFilePath())
	if err != nil {
		a.AddLogMsg("[CATALOG] file not found, using defaults")
		items := make([]CatalogItem, len(defaultCatalog))
		copy(items, defaultCatalog)
		return items
	}
	defer file.Close()

	var items []CatalogItem
	if err := json.NewDecoder(file).Decode(&items); err != nil {
		a.AddLogMsg("[CATALOG] decode error: " + err.Error())
		return nil
	}

	a.AddLogMsg("[CATALOG] loaded successfully")
	return items
}

// SaveCatalog writes the catalog to disk.
func (a *App) SaveCatalog(items []CatalogItem) {
	// Normalise names before saving
	for i := range items {
		items[i].Name = strings.TrimSpace(strings.ToLower(items[i].Name))
		if items[i].DisplayName == "" {
			items[i].DisplayName = formatTradeItemName(items[i].Name)
		}
	}

	// Keep list sorted by name for deterministic output
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	file, err := os.Create(getCatalogFilePath())
	if err != nil {
		a.AddLogMsg("[CATALOG] save error: " + err.Error())
		return
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(items); err != nil {
		a.AddLogMsg("[CATALOG] encode error: " + err.Error())
		return
	}

	a.AddLogMsg("[CATALOG] saved successfully")
}

// GetCatalogItemValue returns the credit value for a given internal item name,
// and whether the name was found in the catalog.
func (a *App) GetCatalogItemValue(name string) (int, bool) {
	items := a.LoadCatalog()
	for _, item := range items {
		if item.Name == name {
			return item.Value, true
		}
	}
	return 0, false
}
