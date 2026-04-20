package manager

import (
	"math"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/arr"
	"github.com/sirrobot01/decypharr/pkg/debrid/common"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

type progressClientStub struct {
	availabilityClientStub
	torrent *debridTypes.Torrent
}

func (s progressClientStub) CheckStatus(tr *debridTypes.Torrent) (*debridTypes.Torrent, error) {
	return s.torrent, nil
}

func (s progressClientStub) Config() config.Debrid {
	return config.Debrid{Name: "torbox", Provider: "torbox"}
}

func TestProcessQueuedTorrentNormalizesTorboxUnitProgress(t *testing.T) {
	strg, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = strg.Close() }()

	queue := newQueue(t.Context(), strg, 10, "")
	client := progressClientStub{
		torrent: &debridTypes.Torrent{
			Id:       "torrent-id",
			InfoHash: "hash-queued-progress",
			Name:     "example",
			Size:     100,
			Status:   debridTypes.TorrentStatusDownloading,
			Progress: 0.265791,
			Speed:    4321,
			Seeders:  9,
			Debrid:   "torbox",
		},
	}

	manager := &Manager{
		storage: strg,
		queue:   queue,
		arr:     arr.NewStorage(),
		clients: xsync.NewMap[string, common.Client](),
		logger:  zerolog.Nop(),
		config:  &config.Config{},
	}
	manager.clients.Store("torbox", client)

	entry := &storage.Entry{
		Protocol:         config.ProtocolTorrent,
		InfoHash:         "hash-queued-progress",
		Name:             "example",
		OriginalFilename: "example",
		Magnet:           "magnet:?xt=urn:btih:hash-queued-progress",
		Size:             100,
		Bytes:            100,
		State:            storage.EntryStateDownloading,
		Status:           debridTypes.TorrentStatusDownloading,
		ActiveProvider:   "torbox",
		Providers: map[string]*storage.ProviderEntry{
			"torbox": {
				Provider: "torbox",
				ID:       "torrent-id",
				AddedAt:  time.Now().Add(-time.Hour),
				Status:   debridTypes.TorrentStatusDownloading,
				Files:    map[string]*storage.ProviderFile{},
			},
		},
		Files:     map[string]*storage.File{},
		AddedOn:   time.Now().Add(-time.Hour),
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
	}

	if err := queue.Add(entry); err != nil {
		t.Fatalf("failed to add queued entry: %v", err)
	}

	manager.processQueuedTorrent(entry)

	updated, err := strg.GetQueued(entry.InfoHash)
	if err != nil {
		t.Fatalf("failed to reload queued entry: %v", err)
	}

	if math.Abs(updated.Progress-0.265791) > 0.000001 {
		t.Fatalf("unexpected queue progress: got %v want 0.265791", updated.Progress)
	}
	if updated.Speed != 4321 {
		t.Fatalf("unexpected speed: got %d want 4321", updated.Speed)
	}
	if updated.Seeders != 9 {
		t.Fatalf("unexpected seeders: got %d want 9", updated.Seeders)
	}
	if placement := updated.GetActiveProvider(); placement == nil {
		t.Fatal("expected active placement")
	} else if math.Abs(placement.Progress-0.265791) > 0.000001 {
		t.Fatalf("unexpected placement progress: got %v want 0.265791", placement.Progress)
	}
}
