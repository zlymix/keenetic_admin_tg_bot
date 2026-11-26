package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FavoriteDevice struct {
	MAC     string    `json:"mac"`
	Name    string    `json:"name,omitempty"`
	AddedAt time.Time `json:"added_at"`
}

type FavoritesStorage struct {
	filePath string
	mu       sync.RWMutex
	items    map[string]FavoriteDevice // normalized lowercase MAC -> FavoriteDevice
}

func NewFavoritesStorage(filePath string) (*FavoritesStorage, error) {
	if filePath == "" {
		filePath = "favorites.json"
	}

	s := &FavoritesStorage{
		filePath: filePath,
		items:    make(map[string]FavoriteDevice),
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load favorites: %w", err)
	}

	return s, nil
}

func (s *FavoritesStorage) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, start with empty map
			return nil
		}
		return err
	}

	if len(data) == 0 {
		return nil
	}

	var loaded map[string]FavoriteDevice
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("unmarshal favorites json: %w", err)
	}

	s.items = make(map[string]FavoriteDevice, len(loaded))
	for k, v := range loaded {
		s.items[strings.ToLower(k)] = v
	}

	return nil
}

func (s *FavoritesStorage) saveLocked() error {
	dir := filepath.Dir(s.filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create favorites dir: %w", err)
		}
	}

	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal favorites: %w", err)
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("write tmp favorites file: %w", err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return fmt.Errorf("rename tmp favorites file: %w", err)
	}

	return nil
}

// IsFavorite returns true if device with given MAC is in favorites.
func (s *FavoritesStorage) IsFavorite(mac string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	norm := strings.ToLower(strings.TrimSpace(mac))
	_, exists := s.items[norm]
	return exists
}

// Add adds device to favorites and saves to json file.
func (s *FavoritesStorage) Add(mac, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	norm := strings.ToLower(strings.TrimSpace(mac))
	s.items[norm] = FavoriteDevice{
		MAC:     norm,
		Name:    name,
		AddedAt: time.Now(),
	}

	return s.saveLocked()
}

// Remove removes device from favorites and saves to json file.
func (s *FavoritesStorage) Remove(mac string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	norm := strings.ToLower(strings.TrimSpace(mac))
	delete(s.items, norm)

	return s.saveLocked()
}

// Toggle toggles favorite status for device. Returns new favorite state.
func (s *FavoritesStorage) Toggle(mac, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	norm := strings.ToLower(strings.TrimSpace(mac))
	if _, exists := s.items[norm]; exists {
		delete(s.items, norm)
		return false, s.saveLocked()
	}

	s.items[norm] = FavoriteDevice{
		MAC:     norm,
		Name:    name,
		AddedAt: time.Now(),
	}
	return true, s.saveLocked()
}
