package manager

import (
	"context"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/debrid/account"
	"github.com/sirrobot01/decypharr/pkg/debrid/common"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

type seedingClientStub struct {
	cfg           config.Debrid
	remote        *debridTypes.Torrent
	stopCalls     []string
	stopErr       error
	getTorrentErr error
}

func (s *seedingClientStub) SubmitMagnet(tr *debridTypes.Torrent) (*debridTypes.Torrent, error) {
	return nil, nil
}

func (s *seedingClientStub) CheckStatus(tr *debridTypes.Torrent) (*debridTypes.Torrent, error) {
	return nil, nil
}

func (s *seedingClientStub) GetDownloadLink(torrentID string, file *debridTypes.File) (debridTypes.DownloadLink, error) {
	return debridTypes.DownloadLink{}, nil
}

func (s *seedingClientStub) DeleteTorrent(torrentId string) error { return nil }
func (s *seedingClientStub) StopSeeding(torrentId string) error {
	s.stopCalls = append(s.stopCalls, torrentId)
	return s.stopErr
}
func (s *seedingClientStub) IsAvailable(infohashes []string) map[string]bool  { return nil }
func (s *seedingClientStub) UpdateTorrent(torrent *debridTypes.Torrent) error { return nil }
func (s *seedingClientStub) GetTorrent(torrentId string) (*debridTypes.Torrent, error) {
	if s.getTorrentErr != nil {
		return nil, s.getTorrentErr
	}
	return s.remote, nil
}
func (s *seedingClientStub) GetTorrents() ([]*debridTypes.Torrent, error) { return nil, nil }
func (s *seedingClientStub) Config() config.Debrid                        { return s.cfg }
func (s *seedingClientStub) Logger() zerolog.Logger                       { return zerolog.Nop() }
func (s *seedingClientStub) RefreshDownloadLinks() error                  { return nil }
func (s *seedingClientStub) CheckFile(ctx context.Context, infohash, fileID string) error {
	return nil
}
func (s *seedingClientStub) AccountManager() *account.Manager             { return nil }
func (s *seedingClientStub) GetProfile() (*debridTypes.Profile, error)    { return nil, nil }
func (s *seedingClientStub) GetAvailableSlots() (int, error)              { return 0, nil }
func (s *seedingClientStub) SyncAccounts()                                {}
func (s *seedingClientStub) DeleteLink(dl debridTypes.DownloadLink) error { return nil }
func (s *seedingClientStub) SpeedTest(ctx context.Context) debridTypes.SpeedTestResult {
	return debridTypes.SpeedTestResult{}
}
func (s *seedingClientStub) SupportsCheck() bool             { return false }
func (s *seedingClientStub) SupportsAvailabilityCheck() bool { return false }

func TestProcessSeedingPoliciesStopsByTime(t *testing.T) {
	manager, client, cleanup := newSeedingTestManager(t)
	defer cleanup()

	downloadedAt := time.Now().Add(-2 * time.Hour)
	minutes := 60
	entry := buildSeedingEntry("time-hash", downloadedAt, &storage.SeedingPolicy{
		StopAfterMinutes: &minutes,
	})
	if err := manager.storage.AddOrUpdate(entry); err != nil {
		t.Fatalf("failed to store entry: %v", err)
	}

	manager.processSeedingPolicies(context.Background())

	updated, err := manager.storage.Get(entry.InfoHash)
	if err != nil {
		t.Fatalf("failed to reload entry: %v", err)
	}
	if len(client.stopCalls) != 1 || client.stopCalls[0] != "torrent-id" {
		t.Fatalf("unexpected stop calls: %#v", client.stopCalls)
	}
	if updated.SeedingPolicy == nil || updated.SeedingPolicy.StopCompletedAt == nil {
		t.Fatal("expected stop to be recorded")
	}
}

func TestProcessSeedingPoliciesStopsByRatio(t *testing.T) {
	manager, client, cleanup := newSeedingTestManager(t)
	defer cleanup()

	downloadedAt := time.Now().Add(-10 * time.Minute)
	ratio := 1.5
	client.remote = &debridTypes.Torrent{Id: "torrent-id", Ratio: 2.0}
	entry := buildSeedingEntry("ratio-hash", downloadedAt, &storage.SeedingPolicy{
		StopOnRatio: &ratio,
	})
	if err := manager.storage.AddOrUpdate(entry); err != nil {
		t.Fatalf("failed to store entry: %v", err)
	}

	manager.processSeedingPolicies(context.Background())

	updated, err := manager.storage.Get(entry.InfoHash)
	if err != nil {
		t.Fatalf("failed to reload entry: %v", err)
	}
	if len(client.stopCalls) != 1 {
		t.Fatalf("expected one stop call, got %#v", client.stopCalls)
	}
	if updated.SeedingPolicy == nil || updated.SeedingPolicy.StopCompletedAt == nil {
		t.Fatal("expected ratio stop to be recorded")
	}
}

func TestProcessSeedingPoliciesSkipsOtherProviders(t *testing.T) {
	manager, client, cleanup := newSeedingTestManager(t)
	defer cleanup()

	client.cfg.Provider = "realdebrid"

	downloadedAt := time.Now().Add(-2 * time.Hour)
	minutes := 60
	entry := buildSeedingEntry("skip-hash", downloadedAt, &storage.SeedingPolicy{
		StopAfterMinutes: &minutes,
	})
	if err := manager.storage.AddOrUpdate(entry); err != nil {
		t.Fatalf("failed to store entry: %v", err)
	}

	manager.processSeedingPolicies(context.Background())

	updated, err := manager.storage.Get(entry.InfoHash)
	if err != nil {
		t.Fatalf("failed to reload entry: %v", err)
	}
	if len(client.stopCalls) != 0 {
		t.Fatalf("expected no stop calls, got %#v", client.stopCalls)
	}
	if updated.SeedingPolicy != nil && updated.SeedingPolicy.StopCompletedAt != nil {
		t.Fatal("expected stop to be skipped")
	}
}

func newSeedingTestManager(t *testing.T) (*Manager, *seedingClientStub, func()) {
	t.Helper()

	strg, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	client := &seedingClientStub{
		cfg: config.Debrid{
			Name:     "torbox-main",
			Provider: "torbox",
		},
		remote: &debridTypes.Torrent{Id: "torrent-id", Ratio: 0},
	}

	clients := xsync.NewMap[string, common.Client]()
	clients.Store("torbox-main", client)

	manager := &Manager{
		storage: strg,
		clients: clients,
		logger:  zerolog.Nop(),
	}

	return manager, client, func() {
		_ = strg.Close()
	}
}

func buildSeedingEntry(infohash string, downloadedAt time.Time, policy *storage.SeedingPolicy) *storage.Entry {
	return &storage.Entry{
		Protocol:         config.ProtocolTorrent,
		InfoHash:         infohash,
		Name:             infohash,
		OriginalFilename: infohash,
		Size:             1,
		Bytes:            1,
		ActiveProvider:   "torbox-main",
		Providers: map[string]*storage.ProviderEntry{
			"torbox-main": {
				Provider:     "torbox-main",
				ID:           "torrent-id",
				Status:       debridTypes.TorrentStatusDownloaded,
				DownloadedAt: &downloadedAt,
				Files:        map[string]*storage.ProviderFile{},
			},
		},
		Files:         map[string]*storage.File{},
		Status:        debridTypes.TorrentStatusDownloaded,
		AddedOn:       downloadedAt,
		CreatedAt:     downloadedAt,
		UpdatedAt:     downloadedAt,
		SeedingPolicy: policy,
	}
}
