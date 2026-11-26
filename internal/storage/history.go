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

// DeviceHistoryItem represents the recorded state and timestamps of a network device.
type DeviceHistoryItem struct {
	MAC          string    `json:"mac"`
	Name         string    `json:"name"`
	Active       bool      `json:"active"`
	OnlineSince  time.Time `json:"online_since,omitempty"`
	OfflineSince time.Time `json:"offline_since,omitempty"`
	LastSeen     time.Time `json:"last_seen,omitempty"`
}

// HistoryStorage provides thread-safe persistent storage for device presence history.
type HistoryStorage struct {
	filePath string
	mu       sync.RWMutex
	items    map[string]DeviceHistoryItem // normalized lowercase MAC -> DeviceHistoryItem
}

// NewHistoryStorage initializes storage from a JSON file.
func NewHistoryStorage(filePath string) (*HistoryStorage, error) {
	if filePath == "" {
		filePath = "data/history.json"
	}

	s := &HistoryStorage{
		filePath: filePath,
		items:    make(map[string]DeviceHistoryItem),
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}

	return s, nil
}

func (s *HistoryStorage) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if len(data) == 0 {
		return nil
	}

	var loaded map[string]DeviceHistoryItem
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("unmarshal history json: %w", err)
	}

	s.items = make(map[string]DeviceHistoryItem, len(loaded))
	for k, v := range loaded {
		s.items[strings.ToLower(k)] = v
	}

	return nil
}

// Save persists the current history to disk atomically.
func (s *HistoryStorage) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal history: %w", err)
	}

	dir := filepath.Dir(s.filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory for history: %w", err)
		}
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", s.filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("write temp history: %w", err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("rename temp history: %w", err)
	}

	return nil
}

// RecordDevice updates the state of a device and returns the transition type:
// "connected" if the device transitioned from offline to online,
// "disconnected" if the device transitioned from online to offline,
// or "" if there was no state change.
func (s *HistoryStorage) RecordDevice(mac, name string, active bool, now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	normMAC := strings.ToLower(strings.TrimSpace(mac))
	if normMAC == "" {
		return ""
	}

	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		trimmedName = normMAC
	}

	item, exists := s.items[normMAC]
	transition := ""

	if !exists {
		// New device never seen before
		item = DeviceHistoryItem{
			MAC:    normMAC,
			Name:   trimmedName,
			Active: active,
		}
		if active {
			item.OnlineSince = now
			item.LastSeen = now
			transition = "connected"
		} else {
			// Device is offline and we never observed it online
			item.OfflineSince = time.Time{}
		}
	} else {
		// Update name if a better name is provided
		if trimmedName != "" && trimmedName != normMAC {
			item.Name = trimmedName
		}

		if active {
			item.LastSeen = now
			if !item.Active {
				// Transition: offline -> online
				item.Active = true
				item.OnlineSince = now
				transition = "connected"
			} else {
				// Was already online; ensure OnlineSince is not zero
				if item.OnlineSince.IsZero() {
					item.OnlineSince = now
				}
			}
		} else {
			if item.Active {
				// Transition: online -> offline
				item.Active = false
				item.OfflineSince = now
				transition = "disconnected"
			}
		}
	}

	s.items[normMAC] = item
	return transition
}

// MarkMissingAsOffline marks any previously active device that is not present in the given MAC set as offline.
// It returns a list of devices that transitioned to offline.
func (s *HistoryStorage) MarkMissingAsOffline(presentMACs map[string]bool, now time.Time) []DeviceHistoryItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	var disconnected []DeviceHistoryItem

	for mac, item := range s.items {
		if !presentMACs[mac] && item.Active {
			item.Active = false
			item.OfflineSince = now
			s.items[mac] = item
			disconnected = append(disconnected, item)
		}
	}

	return disconnected
}

// Get returns the history item for the specified MAC address.
func (s *HistoryStorage) Get(mac string) (DeviceHistoryItem, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.items[strings.ToLower(strings.TrimSpace(mac))]
	return item, ok
}

// GetAll returns a copy of all tracked device history items.
func (s *HistoryStorage) GetAll() map[string]DeviceHistoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make(map[string]DeviceHistoryItem, len(s.items))
	for k, v := range s.items {
		res[k] = v
	}
	return res
}
