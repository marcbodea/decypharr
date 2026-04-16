package torbox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/request"
)

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
