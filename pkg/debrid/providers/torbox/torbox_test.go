package torbox

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
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

func TestGetTorboxStatus(t *testing.T) {
	tb := &Torbox{}

	tests := []struct {
		name     string
		status   string
		finished bool
		want     debridTypes.TorrentStatus
	}{
		{
			name:   "checking stays active",
			status: "checking",
			want:   debridTypes.TorrentStatusDownloading,
		},
		{
			name:   "queued stays active",
			status: "queued",
			want:   debridTypes.TorrentStatusDownloading,
		},
		{
			name:   "processing with suffix stays active",
			status: "processing (metadata)",
			want:   debridTypes.TorrentStatusDownloading,
		},
		{
			name:   "cached is downloaded",
			status: "cached",
			want:   debridTypes.TorrentStatusDownloaded,
		},
		{
			name:   "failed processing is error",
			status: "failed processing",
			want:   debridTypes.TorrentStatusError,
		},
		{
			name:   "incomplete is error",
			status: "incomplete",
			want:   debridTypes.TorrentStatusError,
		},
		{
			name:     "finished overrides state",
			status:   "checking",
			finished: true,
			want:     debridTypes.TorrentStatusDownloaded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tb.getTorboxStatus(tt.status, tt.finished)
			if got != tt.want {
				t.Fatalf("getTorboxStatus(%q, %t) = %q, want %q", tt.status, tt.finished, got, tt.want)
			}
		})
	}
}

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
				"progress": 1,
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

	if torrent.Progress != 1 {
		t.Fatalf("unexpected progress: got %v want 1", torrent.Progress)
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

func TestDeleteTorrent(t *testing.T) {
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

	if err := tb.DeleteTorrent("123"); err != nil {
		t.Fatalf("DeleteTorrent returned error: %v", err)
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
	if gotPayload["operation"] != "delete" {
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

func TestDeleteTorrentReturnsAPIError(t *testing.T) {
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

	if err := tb.DeleteTorrent("123"); err == nil {
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

func TestSubmitMagnetUploadsTorrentFileWhenAvailable(t *testing.T) {
	var (
		gotForm         map[string][]string
		gotFileContents []byte
		gotFileName     string
		gotContentType  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")

		mediaType, params, err := mime.ParseMediaType(gotContentType)
		if err != nil {
			t.Fatalf("failed to parse content type: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("unexpected media type: %s", mediaType)
		}

		reader := multipart.NewReader(r.Body, params["boundary"])
		form, err := reader.ReadForm(1 << 20)
		if err != nil {
			t.Fatalf("failed to read multipart form: %v", err)
		}

		gotForm = form.Value

		files := form.File["file"]
		if len(files) != 1 {
			t.Fatalf("expected one file upload, got %d", len(files))
		}
		gotFileName = files[0].Filename

		file, err := files[0].Open()
		if err != nil {
			t.Fatalf("failed to open uploaded file: %v", err)
		}
		defer file.Close()

		gotFileContents, err = io.ReadAll(file)
		if err != nil {
			t.Fatalf("failed to read uploaded file: %v", err)
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

	const torrentBytes = "dummy torrent bytes"
	torrent := &debridTypes.Torrent{
		InfoHash:         "abc",
		Name:             "Example Release",
		OriginalFilename: "example-release",
		Magnet: &utils.Magnet{
			Link: "magnet:?xt=urn:btih:abc",
			File: []byte(torrentBytes),
		},
		RequestSeeding: true,
	}

	if _, err := tb.SubmitMagnet(torrent); err != nil {
		t.Fatalf("SubmitMagnet returned error: %v", err)
	}

	if gotForm["seed"][0] != "2" {
		t.Fatalf("unexpected seed form value: %q", gotForm["seed"][0])
	}
	if gotForm["name"][0] != "Example Release" {
		t.Fatalf("unexpected name form value: %q", gotForm["name"][0])
	}
	if _, ok := gotForm["magnet"]; ok {
		t.Fatal("expected multipart torrent submission to omit magnet field")
	}
	if string(gotFileContents) != torrentBytes {
		t.Fatalf("unexpected uploaded file contents: %q", string(gotFileContents))
	}
	if gotFileName != "example-release.torrent" {
		t.Fatalf("unexpected uploaded filename: %q", gotFileName)
	}
}
