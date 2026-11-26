package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFavoritesStorage(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "favorites.json")

	store, err := NewFavoritesStorage(filePath)
	if err != nil {
		t.Fatalf("Failed to create favorites storage: %v", err)
	}

	mac := "18:31:BF:52:31:38"

	// 1. Initial check
	if store.IsFavorite(mac) {
		t.Error("Expected device not to be favorite initially")
	}

	// 2. Add
	if err := store.Add(mac, "MAX | PC"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if !store.IsFavorite(mac) {
		t.Error("Expected device to be favorite after Add")
	}
	// Case-insensitivity check
	if !store.IsFavorite("18:31:bf:52:31:38") {
		t.Error("Expected lowercase MAC to be recognized as favorite")
	}

	// 3. Persistence reload test
	store2, err := NewFavoritesStorage(filePath)
	if err != nil {
		t.Fatalf("Failed to reload favorites storage: %v", err)
	}
	if !store2.IsFavorite(mac) {
		t.Error("Expected device to persist across reloads")
	}

	// 4. Toggle
	isFav, err := store2.Toggle(mac, "MAX | PC")
	if err != nil {
		t.Fatalf("Toggle failed: %v", err)
	}
	if isFav {
		t.Error("Expected toggle to remove favorite")
	}
	if store2.IsFavorite(mac) {
		t.Error("Expected device not to be favorite after toggle")
	}

	// 5. Toggle back
	isFav, err = store2.Toggle(mac, "MAX | PC")
	if err != nil {
		t.Fatalf("Toggle back failed: %v", err)
	}
	if !isFav {
		t.Error("Expected toggle to add favorite")
	}
	if !store2.IsFavorite(mac) {
		t.Error("Expected device to be favorite after toggle back")
	}

	// 6. Remove
	if err := store2.Remove(mac); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if store2.IsFavorite(mac) {
		t.Error("Expected device not to be favorite after Remove")
	}

	// Check file content is valid JSON
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	t.Logf("Favorites JSON file content:\n%s", string(data))
}
