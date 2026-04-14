package manager

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/debrid/account"
	"github.com/sirrobot01/decypharr/pkg/debrid/types"
)

type availabilityClientStub struct {
	available map[string]bool
	supports  bool
}

func (s availabilityClientStub) SubmitMagnet(tr *types.Torrent) (*types.Torrent, error) {
	return nil, nil
}

func (s availabilityClientStub) CheckStatus(tr *types.Torrent) (*types.Torrent, error) {
	return nil, nil
}

func (s availabilityClientStub) GetDownloadLink(torrentID string, file *types.File) (types.DownloadLink, error) {
	return types.DownloadLink{}, nil
}

func (s availabilityClientStub) DeleteTorrent(torrentId string) error {
	return nil
}

func (s availabilityClientStub) IsAvailable(infohashes []string) map[string]bool {
	return s.available
}

func (s availabilityClientStub) UpdateTorrent(torrent *types.Torrent) error {
	return nil
}

func (s availabilityClientStub) GetTorrent(torrentId string) (*types.Torrent, error) {
	return nil, nil
}

func (s availabilityClientStub) GetTorrents() ([]*types.Torrent, error) {
	return nil, nil
}

func (s availabilityClientStub) Config() config.Debrid {
	return config.Debrid{}
}

func (s availabilityClientStub) Logger() zerolog.Logger {
	return zerolog.Nop()
}

func (s availabilityClientStub) RefreshDownloadLinks() error {
	return nil
}

func (s availabilityClientStub) CheckFile(ctx context.Context, infohash, fileID string) error {
	return nil
}

func (s availabilityClientStub) AccountManager() *account.Manager {
	return nil
}

func (s availabilityClientStub) GetProfile() (*types.Profile, error) {
	return nil, nil
}

func (s availabilityClientStub) GetAvailableSlots() (int, error) {
	return 0, nil
}

func (s availabilityClientStub) SyncAccounts() {}

func (s availabilityClientStub) DeleteLink(dl types.DownloadLink) error {
	return nil
}

func (s availabilityClientStub) SpeedTest(ctx context.Context) types.SpeedTestResult {
	return types.SpeedTestResult{}
}

func (s availabilityClientStub) SupportsCheck() bool {
	return false
}

func (s availabilityClientStub) SupportsAvailabilityCheck() bool {
	return s.supports
}

func TestIsHashAvailableMatchesCaseInsensitiveKeys(t *testing.T) {
	client := availabilityClientStub{
		supports: true,
		available: map[string]bool{
			"ABCDEF0123456789ABCDEF0123456789ABCDEF01": true,
		},
	}

	if !isHashAvailable(client, "abcdef0123456789abcdef0123456789abcdef01") {
		t.Fatal("expected hash to be available")
	}
}

func TestIsHashAvailableSkipsUnsupportedProviders(t *testing.T) {
	client := availabilityClientStub{
		supports: false,
		available: map[string]bool{
			"abcdef0123456789abcdef0123456789abcdef01": true,
		},
	}

	if isHashAvailable(client, "abcdef0123456789abcdef0123456789abcdef01") {
		t.Fatal("expected unsupported provider to skip availability preflight")
	}
}
