package link

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	debridtypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
)

func TestValidateLinkTorbox400IsRefetchable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusBadRequest)
	}))
	defer srv.Close()

	svc := &Service{
		httpClient: srv.Client(),
	}

	err := svc.validateLink(context.Background(), &debridtypes.DownloadLink{
		Debrid:       "torbox",
		Filename:     "movie.mkv",
		Link:         "torbox://123/0",
		DownloadLink: srv.URL,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	linkErr := GetLinkError(err)
	if linkErr == nil {
		t.Fatalf("expected link error, got %T", err)
	}
	if !linkErr.ShouldRefetch() {
		t.Fatalf("expected refetchable error, got category %s", linkErr.Category.String())
	}
}

func TestValidateLinkGeneric400RemainsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	svc := &Service{
		httpClient: srv.Client(),
	}

	err := svc.validateLink(context.Background(), &debridtypes.DownloadLink{
		Debrid:       "realdebrid",
		Filename:     "movie.mkv",
		Link:         "rd://123/0",
		DownloadLink: srv.URL,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	linkErr := GetLinkError(err)
	if linkErr == nil {
		t.Fatalf("expected link error, got %T", err)
	}
	if !linkErr.IsPermanent() {
		t.Fatalf("expected permanent error, got category %s", linkErr.Category.String())
	}
}
