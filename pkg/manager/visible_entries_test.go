package manager

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

func TestListVisibleEntriesMergesQueueAndPersisted(t *testing.T) {
	strg, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = strg.Close() }()

	mgr := &Manager{
		storage: strg,
		queue:   newQueue(context.Background(), strg, 10, ""),
		logger:  zerolog.Nop(),
	}

	storedUpdatedAt := time.Now().Add(-time.Minute)
	stored := &storage.Entry{
		Protocol:         config.ProtocolTorrent,
		InfoHash:         "hash-a",
		Name:             "stored",
		OriginalFilename: "stored",
		ActiveProvider:   "torbox",
		Providers: map[string]*storage.ProviderEntry{
			"torbox": {
				Provider: "torbox",
				ID:       "1",
				Status:   debridTypes.TorrentStatusDownloaded,
				Files:    map[string]*storage.ProviderFile{},
			},
		},
		Files:     map[string]*storage.File{},
		Status:    debridTypes.TorrentStatusDownloaded,
		State:     storage.EntryStatePausedUP,
		AddedOn:   storedUpdatedAt,
		CreatedAt: storedUpdatedAt,
		UpdatedAt: storedUpdatedAt,
	}
	if err := strg.AddOrUpdate(stored); err != nil {
		t.Fatalf("failed to add stored entry: %v", err)
	}

	queuedUpdatedAt := time.Now()
	queued := &storage.Entry{
		Protocol:         config.ProtocolTorrent,
		InfoHash:         "hash-a",
		Name:             "queued",
		OriginalFilename: "queued",
		ActiveProvider:   "torbox",
		Providers: map[string]*storage.ProviderEntry{
			"torbox": {
				Provider: "torbox",
				ID:       "1",
				Status:   debridTypes.TorrentStatusDownloading,
				Files:    map[string]*storage.ProviderFile{},
			},
		},
		Files:     map[string]*storage.File{},
		Status:    debridTypes.TorrentStatusDownloading,
		State:     storage.EntryStateDownloading,
		AddedOn:   queuedUpdatedAt,
		CreatedAt: queuedUpdatedAt,
		UpdatedAt: queuedUpdatedAt,
	}
	if err := mgr.queue.Add(queued); err != nil {
		t.Fatalf("failed to add queued entry: %v", err)
	}

	persistedOnly := &storage.Entry{
		Protocol:         config.ProtocolTorrent,
		InfoHash:         "hash-b",
		Name:             "persisted-only",
		OriginalFilename: "persisted-only",
		ActiveProvider:   "torbox",
		Providers: map[string]*storage.ProviderEntry{
			"torbox": {
				Provider: "torbox",
				ID:       "2",
				Status:   debridTypes.TorrentStatusDownloaded,
				Files:    map[string]*storage.ProviderFile{},
			},
		},
		Files:     map[string]*storage.File{},
		Status:    debridTypes.TorrentStatusDownloaded,
		State:     storage.EntryStatePausedUP,
		AddedOn:   queuedUpdatedAt,
		CreatedAt: queuedUpdatedAt,
		UpdatedAt: queuedUpdatedAt,
	}
	if err := strg.AddOrUpdate(persistedOnly); err != nil {
		t.Fatalf("failed to add persisted-only entry: %v", err)
	}

	entries, err := mgr.ListVisibleEntries("", config.ProtocolAll, "", nil)
	if err != nil {
		t.Fatalf("ListVisibleEntries returned error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("unexpected entry count: got %d want 2", len(entries))
	}

	byHash := make(map[string]*storage.Entry, len(entries))
	for _, entry := range entries {
		byHash[entry.InfoHash] = entry
	}

	if got := byHash["hash-a"]; got == nil || got.Name != "queued" || got.State != storage.EntryStateDownloading {
		t.Fatalf("expected queued entry to win for hash-a, got %#v", got)
	}
	if got := byHash["hash-b"]; got == nil || got.Name != "persisted-only" {
		t.Fatalf("expected persisted-only entry to remain visible, got %#v", got)
	}
}
