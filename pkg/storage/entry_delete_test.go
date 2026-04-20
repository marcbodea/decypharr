package storage

import (
	"errors"
	"testing"

	"github.com/sirrobot01/decypharr/internal/config"
)

func TestDeleteQueuedReturnsCleanupErrorAndKeepsEntry(t *testing.T) {
	store, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	entry := &Entry{
		Protocol: config.ProtocolTorrent,
		InfoHash: "queued-hash",
		Name:     "queued-hash",
	}
	if err := store.AddQueue(entry); err != nil {
		t.Fatalf("failed to add queued entry: %v", err)
	}

	if err := store.DeleteQueued(entry.InfoHash, func(*Entry) error {
		return errors.New("cleanup failed")
	}); err == nil {
		t.Fatal("expected DeleteQueued to return a cleanup error")
	}

	if _, err := store.GetQueued(entry.InfoHash); err != nil {
		t.Fatalf("expected queued entry to remain after cleanup error: %v", err)
	}
}
