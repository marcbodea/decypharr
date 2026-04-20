package manager

import (
	"errors"
	"testing"
	"time"
)

func TestDeleteEntryRemovesProviderPlacements(t *testing.T) {
	manager, client, cleanup := newSeedingTestManager(t)
	defer cleanup()

	entry := buildSeedingEntry("delete-hash", time.Now().Add(-time.Hour), nil)
	if err := manager.storage.AddOrUpdate(entry); err != nil {
		t.Fatalf("failed to store entry: %v", err)
	}

	if err := manager.DeleteEntry(entry.InfoHash, true); err != nil {
		t.Fatalf("DeleteEntry returned error: %v", err)
	}

	if len(client.deleteCalls) != 1 || client.deleteCalls[0] != "torrent-id" {
		t.Fatalf("unexpected delete calls: %#v", client.deleteCalls)
	}

	if _, err := manager.storage.Get(entry.InfoHash); err == nil {
		t.Fatal("expected entry to be removed from storage")
	}
}

func TestDeleteEntryKeepsStorageWhenProviderDeleteFails(t *testing.T) {
	manager, client, cleanup := newSeedingTestManager(t)
	defer cleanup()

	entry := buildSeedingEntry("delete-error-hash", time.Now().Add(-time.Hour), nil)
	if err := manager.storage.AddOrUpdate(entry); err != nil {
		t.Fatalf("failed to store entry: %v", err)
	}

	client.deleteErr = errors.New("provider delete failed")

	if err := manager.DeleteEntry(entry.InfoHash, true); err == nil {
		t.Fatal("expected DeleteEntry to return an error")
	}

	if len(client.deleteCalls) != 1 || client.deleteCalls[0] != "torrent-id" {
		t.Fatalf("unexpected delete calls: %#v", client.deleteCalls)
	}

	if _, err := manager.storage.Get(entry.InfoHash); err != nil {
		t.Fatalf("expected entry to remain in storage, got error: %v", err)
	}
}
