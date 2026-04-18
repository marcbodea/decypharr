package manager

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/arr"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/notifications"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

func TestMarkAsCompletedPersistsFinalState(t *testing.T) {
	strg, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = strg.Close() }()

	mgr := &Manager{
		storage:       strg,
		queue:         newQueue(context.Background(), strg, 10, ""),
		arr:           arr.NewStorage(),
		logger:        zerolog.Nop(),
		Notifications: notifications.New(&config.Notifications{}, zerolog.Nop()),
	}

	addedAt := time.Now().Add(-time.Minute)
	entry := &storage.Entry{
		Protocol:         config.ProtocolTorrent,
		InfoHash:         "hash-complete",
		Name:             "example",
		OriginalFilename: "example",
		Category:         "tv-sonarr",
		SavePath:         "/downloads",
		ActiveProvider:   "torbox",
		Providers: map[string]*storage.ProviderEntry{
			"torbox": {
				Provider: "torbox",
				ID:       "1",
				Status:   debridTypes.TorrentStatusDownloaded,
				Files:    map[string]*storage.ProviderFile{},
			},
		},
		Files: map[string]*storage.File{
			"episode.mkv": {
				Name:     "episode.mkv",
				Size:     1234,
				InfoHash: "hash-complete",
				AddedOn:  addedAt,
			},
		},
		Status:    debridTypes.TorrentStatusDownloaded,
		State:     storage.EntryStateDownloading,
		Progress:  1,
		AddedOn:   addedAt,
		CreatedAt: addedAt,
		UpdatedAt: addedAt,
	}

	if err := strg.AddOrUpdate(entry); err != nil {
		t.Fatalf("failed to seed persisted entry: %v", err)
	}
	if err := mgr.queue.Add(entry); err != nil {
		t.Fatalf("failed to seed queued entry: %v", err)
	}

	d := &Downloader{manager: mgr, logger: zerolog.Nop()}
	d.markAsCompleted(entry)

	persisted, err := strg.Get(entry.InfoHash)
	if err != nil {
		t.Fatalf("failed to load persisted entry: %v", err)
	}

	if persisted.State != storage.EntryStatePausedUP {
		t.Fatalf("expected persisted state %q, got %q", storage.EntryStatePausedUP, persisted.State)
	}
	if !persisted.IsComplete {
		t.Fatal("expected persisted entry to be complete")
	}
	if persisted.CompletedAt == nil {
		t.Fatal("expected persisted entry completion time to be set")
	}
}
