package manager

import (
	"context"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	debrid "github.com/sirrobot01/decypharr/pkg/debrid/common"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

func TestProcessSyncTorrentNormalizesEntryProgressForActiveProvider(t *testing.T) {
	strg, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = strg.Close() }()

	mgr := &Manager{
		storage: strg,
		clients: xsync.NewMap[string, debrid.Client](),
		logger:  zerolog.Nop(),
	}
	mgr.clients.Store("torbox", availabilityClientStub{})

	addedAt := time.Now().Add(-time.Hour)
	entry := &storage.Entry{
		Protocol:         config.ProtocolTorrent,
		InfoHash:         "hash-a",
		Name:             "example",
		OriginalFilename: "example",
		Size:             100,
		Bytes:            100,
		ActiveProvider:   "torbox",
		Providers: map[string]*storage.ProviderEntry{
			"torbox": {
				Provider: "torbox",
				ID:       "torrent-id",
				Status:   debridTypes.TorrentStatusDownloaded,
				Progress: 100,
				Files: map[string]*storage.ProviderFile{
					"example.mkv": {Id: "1", Link: "torbox://torrent-id/1", Path: "example.mkv"},
				},
			},
		},
		Files: map[string]*storage.File{
			"example.mkv": {Name: "example.mkv", Size: 100, AddedOn: addedAt},
		},
		Status:    debridTypes.TorrentStatusDownloaded,
		Progress:  100,
		AddedOn:   addedAt,
		CreatedAt: addedAt,
		UpdatedAt: addedAt,
	}
	if err := strg.AddOrUpdate(entry); err != nil {
		t.Fatalf("failed to seed storage entry: %v", err)
	}

	remote := &debridTypes.Torrent{
		Id:               "torrent-id",
		InfoHash:         "hash-a",
		Name:             "example",
		OriginalFilename: "example",
		Size:             100,
		Bytes:            100,
		Status:           debridTypes.TorrentStatusDownloaded,
		Progress:         100,
		Speed:            0,
		Seeders:          5,
		Debrid:           "torbox",
		Added:            addedAt,
		Files: map[string]debridTypes.File{
			"example.mkv": {Id: "1", Name: "example.mkv", Size: 100, Path: "example.mkv", Link: "torbox://torrent-id/1"},
		},
	}

	updated, err := mgr.processSyncTorrent(remote)
	if err != nil {
		t.Fatalf("processSyncTorrent returned error: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated entry")
	}
	if updated.Progress != 1 {
		t.Fatalf("unexpected normalized progress: got %v want 1", updated.Progress)
	}
	if updated.Seeders != 5 {
		t.Fatalf("unexpected seeders: got %d want 5", updated.Seeders)
	}
	if updated.State != storage.EntryStatePausedUP {
		t.Fatalf("expected completed torrent state %q, got %q", storage.EntryStatePausedUP, updated.State)
	}
	if updated.CompletedAt == nil {
		t.Fatal("expected completed torrent to have a completion time")
	}
}

func TestDetectTorrentChangesReprocessesStaleTopLevelProgress(t *testing.T) {
	strg, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = strg.Close() }()

	mgr := &Manager{
		storage: strg,
		clients: xsync.NewMap[string, debrid.Client](),
		queue:   newQueue(context.Background(), strg, 10, ""),
		logger:  zerolog.Nop(),
	}
	mgr.clients.Store("torbox", availabilityClientStub{})

	addedAt := time.Now().Add(-time.Hour)
	entry := &storage.Entry{
		Protocol:         config.ProtocolTorrent,
		InfoHash:         "hash-a",
		Name:             "example",
		OriginalFilename: "example",
		ActiveProvider:   "torbox",
		Providers: map[string]*storage.ProviderEntry{
			"torbox": {
				Provider: "torbox",
				ID:       "torrent-id",
				Status:   debridTypes.TorrentStatusDownloaded,
				Progress: 100,
				Files: map[string]*storage.ProviderFile{
					"example.mkv": {Id: "1", Link: "torbox://torrent-id/1", Path: "example.mkv"},
				},
			},
		},
		Files: map[string]*storage.File{
			"example.mkv": {Name: "example.mkv", Size: 100, AddedOn: addedAt},
		},
		Status:    debridTypes.TorrentStatusDownloaded,
		Progress:  100,
		AddedOn:   addedAt,
		CreatedAt: addedAt,
		UpdatedAt: addedAt,
	}
	if err := strg.AddOrUpdate(entry); err != nil {
		t.Fatalf("failed to seed storage entry: %v", err)
	}

	remote := map[string]*debridTypes.Torrent{
		"hash-a": {
			Id:               "torrent-id",
			InfoHash:         "hash-a",
			Name:             "example",
			OriginalFilename: "example",
			Status:           debridTypes.TorrentStatusDownloaded,
			Progress:         100,
			Debrid:           "torbox",
			Added:            addedAt,
			Files: map[string]debridTypes.File{
				"example.mkv": {Id: "1", Name: "example.mkv", Size: 100, Path: "example.mkv", Link: "torbox://torrent-id/1"},
			},
		},
	}

	newTorrents, torrentsToUpdate, torrentsToDelete, err := mgr.detectTorrentChanges("torbox", remote)
	if err != nil {
		t.Fatalf("detectTorrentChanges returned error: %v", err)
	}
	if len(torrentsToUpdate) != 0 {
		t.Fatalf("expected no direct updates, got %d", len(torrentsToUpdate))
	}
	if len(torrentsToDelete) != 0 {
		t.Fatalf("expected no deletions, got %d", len(torrentsToDelete))
	}
	if len(newTorrents) != 1 {
		t.Fatalf("expected stale entry to be reprocessed, got %d entries", len(newTorrents))
	}
}
