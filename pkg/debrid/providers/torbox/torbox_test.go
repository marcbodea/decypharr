package torbox

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/request"
	"github.com/sirrobot01/decypharr/internal/utils"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
)

func TestGetTorrentPreservesProviderProgressScale(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"error": null,
			"detail": "ok",
			"data": {
				"id": 123,
				"hash": "abc123",
				"name": "example.mkv",
				"size": 1024,
				"download_state": "downloading",
				"download_finished": false,
				"progress": 100,
				"download_speed": 0,
				"seeds": 5,
				"ratio": 0,
				"created_at": "2026-01-01T00:00:00Z",
				"files": []
			}
		}`))
	}))
	defer server.Close()

	tb := &Torbox{
		Host:   server.URL,
		client: request.New(),
		logger: zerolog.Nop(),
		config: config.Debrid{Name: "torbox", Provider: "torbox"},
	}

	torrent, err := tb.GetTorrent("123")
	if err != nil {
		t.Fatalf("GetTorrent returned error: %v", err)
	}

	if torrent.Progress != 100 {
		t.Fatalf("unexpected progress: got %v want 100", torrent.Progress)
	}
}

func TestStopSeeding(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("failed to decode request payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"error":null,"detail":"ok","data":null}`))
	}))
	defer server.Close()

	tb := &Torbox{
		Host:   server.URL,
		client: request.New(),
		logger: zerolog.Nop(),
		config: config.Debrid{Name: "torbox", Provider: "torbox"},
	}

	if err := tb.StopSeeding("123"); err != nil {
		t.Fatalf("StopSeeding returned error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("unexpected method: %s", gotMethod)
	}
	if gotPath != "/api/torrents/controltorrent" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotPayload["torrent_id"] != "123" {
		t.Fatalf("unexpected torrent_id payload: %#v", gotPayload)
	}
	if gotPayload["operation"] != "stop_seeding" {
		t.Fatalf("unexpected operation payload: %#v", gotPayload)
	}
}

func TestStopSeedingReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"error":"INVALID_OPTION","detail":"bad"}`))
	}))
	defer server.Close()

	tb := &Torbox{
		Host:   server.URL,
		client: request.New(),
		logger: zerolog.Nop(),
		config: config.Debrid{Name: "torbox", Provider: "torbox"},
	}

	if err := tb.StopSeeding("123"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSubmitMagnetRequestsSeedingWhenPolicyIsSet(t *testing.T) {
	var gotForm url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		gotForm, err = url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("failed to parse form body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"error":null,"detail":"ok","data":{"torrent_id":123,"hash":"abc"}}`))
	}))
	defer server.Close()

	tb := &Torbox{
		Host:   server.URL,
		client: request.New(),
		logger: zerolog.Nop(),
		config: config.Debrid{Name: "torbox", Provider: "torbox"},
	}

	torrent := &debridTypes.Torrent{
		Magnet:         &utils.Magnet{Link: "magnet:?xt=urn:btih:abc"},
		RequestSeeding: true,
	}

	if _, err := tb.SubmitMagnet(torrent); err != nil {
		t.Fatalf("SubmitMagnet returned error: %v", err)
	}

	if gotForm.Get("seed") != "2" {
		t.Fatalf("unexpected seed form value: %q", gotForm.Get("seed"))
	}
}
