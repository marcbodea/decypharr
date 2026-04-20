package qbit

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/debrid/account"
	"github.com/sirrobot01/decypharr/pkg/debrid/common"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/manager"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

func TestParseSeedingPolicy(t *testing.T) {
	tests := []struct {
		name        string
		values      url.Values
		wantRatio   *float64
		wantMinutes *int
		wantErr     bool
	}{
		{
			name:        "unset values return nil",
			values:      url.Values{},
			wantRatio:   nil,
			wantMinutes: nil,
		},
		{
			name: "explicit values are parsed",
			values: url.Values{
				"ratioLimit":       []string{"1.5"},
				"seedingTimeLimit": []string{"60"},
			},
			wantRatio:   ptrFloat64(1.5),
			wantMinutes: ptrInt(60),
		},
		{
			name: "special negative values are ignored",
			values: url.Values{
				"ratioLimit":       []string{"-2"},
				"seedingTimeLimit": []string{"-1"},
			},
			wantRatio:   nil,
			wantMinutes: nil,
		},
		{
			name: "other negative ratio returns error",
			values: url.Values{
				"ratioLimit": []string{"-3"},
			},
			wantErr: true,
		},
		{
			name: "other negative seeding time returns error",
			values: url.Values{
				"seedingTimeLimit": []string{"-3"},
			},
			wantErr: true,
		},
		{
			name: "invalid ratio returns error",
			values: url.Values{
				"ratioLimit": []string{"abc"},
			},
			wantErr: true,
		},
		{
			name: "invalid seeding time returns error",
			values: url.Values{
				"seedingTimeLimit": []string{"abc"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v2/torrents/add", nil)
			req.Form = tt.values

			policy, err := parseSeedingPolicy(req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSeedingPolicy returned error: %v", err)
			}

			if tt.wantRatio == nil && tt.wantMinutes == nil {
				if policy != nil {
					t.Fatalf("expected nil policy, got %#v", policy)
				}
				return
			}
			if policy == nil {
				t.Fatal("expected policy")
			}
			if !floatPtrEqual(policy.StopOnRatio, tt.wantRatio) {
				t.Fatalf("unexpected ratio limit: got %v want %v", policy.StopOnRatio, tt.wantRatio)
			}
			if !intPtrEqual(policy.StopAfterMinutes, tt.wantMinutes) {
				t.Fatalf("unexpected seeding time limit: got %v want %v", policy.StopAfterMinutes, tt.wantMinutes)
			}
		})
	}
}

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt(v int) *int             { return &v }

func floatPtrEqual(got, want *float64) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func intPtrEqual(got, want *int) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func TestSplitHashes(t *testing.T) {
	hashes := splitHashes("abc|def,ghi\njkl")
	if len(hashes) != 4 {
		t.Fatalf("unexpected hash count: got %d want 4", len(hashes))
	}
	want := []string{"abc", "def", "ghi", "jkl"}
	for i := range want {
		if hashes[i] != want[i] {
			t.Fatalf("unexpected hash at %d: got %q want %q", i, hashes[i], want[i])
		}
	}
}

func TestHandleWebAPIVersion(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/app/webapiVersion", nil)
	rec := httptest.NewRecorder()

	var q QBit
	q.handleWebAPIVersion(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", res.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(body) != qbitWebAPIVersion {
		t.Fatalf("unexpected web api version: got %q want %q", string(body), qbitWebAPIVersion)
	}
}

func TestConvertToQBitTorrentTorrentSeedingTime(t *testing.T) {
	now := time.Now()
	completedAt := now.Add(-2 * time.Minute)
	entry := &storage.Entry{
		InfoHash:    "abc123",
		Name:        "example",
		Size:        1024,
		Progress:    1,
		State:       storage.EntryStatePausedUP,
		CreatedAt:   now.Add(-10 * time.Minute),
		CompletedAt: &completedAt,
	}

	torrent := convertToQBitTorrentTorrent(entry)

	if torrent.SeedingTime < 119 || torrent.SeedingTime > 121 {
		t.Fatalf("unexpected seeding time: got %d want about 120", torrent.SeedingTime)
	}
}

type deleteClientStub struct {
	cfg         config.Debrid
	deleteCalls []string
}

func (s *deleteClientStub) SubmitMagnet(tr *debridTypes.Torrent) (*debridTypes.Torrent, error) {
	return nil, nil
}

func (s *deleteClientStub) CheckStatus(tr *debridTypes.Torrent) (*debridTypes.Torrent, error) {
	return nil, nil
}

func (s *deleteClientStub) GetDownloadLink(torrentID string, file *debridTypes.File) (debridTypes.DownloadLink, error) {
	return debridTypes.DownloadLink{}, nil
}

func (s *deleteClientStub) DeleteTorrent(torrentId string) error {
	s.deleteCalls = append(s.deleteCalls, torrentId)
	return nil
}

func (s *deleteClientStub) StopSeeding(torrentId string) error { return nil }
func (s *deleteClientStub) IsAvailable(infohashes []string) map[string]bool {
	return nil
}
func (s *deleteClientStub) UpdateTorrent(torrent *debridTypes.Torrent) error { return nil }
func (s *deleteClientStub) GetTorrent(torrentId string) (*debridTypes.Torrent, error) {
	return nil, nil
}
func (s *deleteClientStub) GetTorrents() ([]*debridTypes.Torrent, error) { return nil, nil }
func (s *deleteClientStub) Config() config.Debrid                        { return s.cfg }
func (s *deleteClientStub) Logger() zerolog.Logger                       { return zerolog.Nop() }
func (s *deleteClientStub) RefreshDownloadLinks() error                  { return nil }
func (s *deleteClientStub) CheckFile(ctx context.Context, infohash, fileID string) error {
	return nil
}
func (s *deleteClientStub) AccountManager() *account.Manager             { return nil }
func (s *deleteClientStub) GetProfile() (*debridTypes.Profile, error)    { return nil, nil }
func (s *deleteClientStub) GetAvailableSlots() (int, error)              { return 0, nil }
func (s *deleteClientStub) SyncAccounts()                                {}
func (s *deleteClientStub) DeleteLink(dl debridTypes.DownloadLink) error { return nil }
func (s *deleteClientStub) SpeedTest(ctx context.Context) debridTypes.SpeedTestResult {
	return debridTypes.SpeedTestResult{}
}
func (s *deleteClientStub) SupportsCheck() bool             { return false }
func (s *deleteClientStub) SupportsAvailabilityCheck() bool { return false }

var _ common.Client = (*deleteClientStub)(nil)

func TestHandleTorrentsDeleteRemovesProviderPlacements(t *testing.T) {
	config.Reset()
	config.SetConfigPath(t.TempDir())
	t.Cleanup(func() {
		config.Reset()
		config.SetConfigPath("")
	})

	mgr := manager.New()
	t.Cleanup(func() {
		_ = mgr.Storage().Close()
	})

	client := &deleteClientStub{
		cfg: config.Debrid{
			Name:     "torbox-main",
			Provider: "torbox",
		},
	}
	mgr.Clients().Store("torbox-main", client)

	entry := &storage.Entry{
		Protocol:         config.ProtocolTorrent,
		InfoHash:         "delete-hash",
		Name:             "delete-hash",
		OriginalFilename: "delete-hash",
		Category:         "tv-sonarr",
		Size:             1,
		Bytes:            1,
		ActiveProvider:   "torbox-main",
		Providers: map[string]*storage.ProviderEntry{
			"torbox-main": {
				Provider: "torbox-main",
				ID:       "torrent-id",
				Status:   debridTypes.TorrentStatusDownloaded,
				Files:    map[string]*storage.ProviderFile{},
			},
		},
		Files:     map[string]*storage.File{},
		Status:    debridTypes.TorrentStatusDownloaded,
		AddedOn:   time.Now().Add(-time.Hour),
		CreatedAt: time.Now().Add(-time.Hour),
	}
	if err := mgr.Storage().AddOrUpdate(entry); err != nil {
		t.Fatalf("failed to seed storage entry: %v", err)
	}

	q := New(mgr)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/delete", nil)
	req = req.WithContext(context.WithValue(req.Context(), hashesKey, []string{entry.InfoHash}))
	rec := httptest.NewRecorder()

	q.handleTorrentsDelete(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
	}
	if len(client.deleteCalls) != 1 || client.deleteCalls[0] != "torrent-id" {
		t.Fatalf("unexpected delete calls: %#v", client.deleteCalls)
	}
	if _, err := mgr.Storage().Get(entry.InfoHash); err == nil {
		t.Fatal("expected storage entry to be deleted")
	}
}

func TestConvertToQBitTorrentTorrentDefersCompletionUntilLocalReady(t *testing.T) {
	entry := &storage.Entry{
		InfoHash:         "abc123",
		Name:             "example",
		OriginalFilename: "example",
		Size:             1024,
		Progress:         1,
		State:            storage.EntryStatePausedUP,
		Action:           config.DownloadActionSymlink,
		IsComplete:       false,
		CreatedAt:        time.Now().Add(-time.Minute),
	}

	torrent := convertToQBitTorrentTorrent(entry)

	if torrent.State != storage.EntryStateDownloading {
		t.Fatalf("expected qbit state %q, got %q", storage.EntryStateDownloading, torrent.State)
	}
	if torrent.Progress >= 1 {
		t.Fatalf("expected qbit progress to stay below 1 until local ready, got %v", torrent.Progress)
	}
	if torrent.CompletionOn != 0 {
		t.Fatalf("expected no completion time until local ready, got %d", torrent.CompletionOn)
	}
}
