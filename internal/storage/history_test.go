package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryStorage_RecordDevice(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "history.json")

	store, err := NewHistoryStorage(filePath)
	if err != nil {
		t.Fatalf("failed to create history storage: %v", err)
	}

	t0 := time.Now().Add(-10 * time.Minute)
	mac := "aa:bb:cc:dd:ee:01"

	// 1. Initial connection
	tr := store.RecordDevice(mac, "Phone", true, t0)
	if tr != "connected" {
		t.Fatalf("expected 'connected', got '%s'", tr)
	}

	item, ok := store.Get(mac)
	if !ok || !item.Active || item.Name != "Phone" {
		t.Fatalf("unexpected item state: %+v", item)
	}
	if !item.OnlineSince.Equal(t0) {
		t.Fatalf("expected OnlineSince %v, got %v", t0, item.OnlineSince)
	}

	// 2. Poll again while still active: no transition, OnlineSince preserved
	t1 := t0.Add(5 * time.Minute)
	tr = store.RecordDevice(mac, "Phone Updated", true, t1)
	if tr != "" {
		t.Fatalf("expected '', got '%s'", tr)
	}

	item, _ = store.Get(mac)
	if !item.OnlineSince.Equal(t0) {
		t.Fatalf("expected preserved OnlineSince %v, got %v", t0, item.OnlineSince)
	}
	if !item.LastSeen.Equal(t1) {
		t.Fatalf("expected LastSeen %v, got %v", t1, item.LastSeen)
	}
	if item.Name != "Phone Updated" {
		t.Fatalf("expected name to be updated, got '%s'", item.Name)
	}

	// 3. Disconnect
	t2 := t1.Add(5 * time.Minute)
	tr = store.RecordDevice(mac, "Phone Updated", false, t2)
	if tr != "disconnected" {
		t.Fatalf("expected 'disconnected', got '%s'", tr)
	}

	item, _ = store.Get(mac)
	if item.Active {
		t.Fatalf("expected active=false")
	}
	if !item.OfflineSince.Equal(t2) {
		t.Fatalf("expected OfflineSince %v, got %v", t2, item.OfflineSince)
	}

	// 4. Persistence test: save and reload
	if err := store.Save(); err != nil {
		t.Fatalf("failed to save history: %v", err)
	}

	store2, err := NewHistoryStorage(filePath)
	if err != nil {
		t.Fatalf("failed to reload history: %v", err)
	}
	item2, ok := store2.Get(mac)
	if !ok || item2.Active || !item2.OfflineSince.Equal(t2) {
		t.Fatalf("reloaded item mismatch: %+v", item2)
	}

	// 5. Test MarkMissingAsOffline
	mac2 := "aa:bb:cc:dd:ee:02"
	store2.RecordDevice(mac2, "Laptop", true, t2)
	present := map[string]bool{mac: true} // mac2 missing
	t3 := t2.Add(2 * time.Minute)
	dis := store2.MarkMissingAsOffline(present, t3)
	if len(dis) != 1 || dis[0].MAC != mac2 {
		t.Fatalf("expected mac2 to be marked offline, got %+v", dis)
	}
	itemLaptop, _ := store2.Get(mac2)
	if itemLaptop.Active || !itemLaptop.OfflineSince.Equal(t3) {
		t.Fatalf("expected Laptop active=false and OfflineSince=t3")
	}
}
