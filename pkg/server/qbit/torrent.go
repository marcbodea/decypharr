package qbit

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"strings"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	"github.com/sirrobot01/decypharr/pkg/arr"
	"github.com/sirrobot01/decypharr/pkg/manager"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

func (q *QBit) shouldDelayImport(importReq *manager.ImportRequest) bool {
	if importReq == nil || importReq.Type != manager.ImportTypeQBit || importReq.SeedingPolicy != nil || importReq.Magnet == nil {
		return false
	}
	return q.isTorboxTarget(importReq)
}

func (q *QBit) submitImport(ctx context.Context, importReq *manager.ImportRequest) error {
	err := q.manager.AddNewTorrent(ctx, importReq)
	if err == nil {
		return nil
	}
	return err
}

func (q *QBit) submitImportAsync(importReq *manager.ImportRequest, reason string) {
	go func() {
		q.logger.Debug().
			Str("hash", importReq.Magnet.InfoHash).
			Str("name", importReq.Magnet.Name).
			Str("reason", reason).
			Bool("has_seeding_policy", importReq.SeedingPolicy != nil).
			Msg("Submitting delayed qBittorrent torrent import")
		if err := q.submitImport(context.Background(), importReq); err != nil {
			q.logger.Error().
				Err(err).
				Str("hash", importReq.Magnet.InfoHash).
				Str("name", importReq.Magnet.Name).
				Str("reason", reason).
				Msg("Failed to submit delayed qBittorrent torrent import")
		}
	}()
}

func (q *QBit) queuePendingImport(importReq *manager.ImportRequest) error {
	hash := strings.ToLower(importReq.Magnet.InfoHash)
	pending := &pendingTorrentImport{
		request:   importReq,
		createdAt: time.Now(),
	}

	pending.timer = time.AfterFunc(pendingQBitShareLimitWindow, func() {
		q.releasePendingImport(hash, nil, "timeout")
	})

	q.pendingImportMu.Lock()
	if existing, ok := q.pendingImports[hash]; ok {
		if existing.timer != nil {
			existing.timer.Stop()
		}
	}
	q.pendingImports[hash] = pending
	q.pendingImportMu.Unlock()

	q.logger.Debug().
		Str("hash", importReq.Magnet.InfoHash).
		Str("name", importReq.Magnet.Name).
		Dur("delay", pendingQBitShareLimitWindow).
		Msg("Delaying qBittorrent torrent import while waiting for share limits")
	return nil
}

func (q *QBit) releasePendingImport(infoHash string, policy *storage.SeedingPolicy, reason string) bool {
	hash := strings.ToLower(infoHash)

	q.pendingImportMu.Lock()
	pending, ok := q.pendingImports[hash]
	if !ok {
		q.pendingImportMu.Unlock()
		return false
	}
	delete(q.pendingImports, hash)
	if pending.timer != nil {
		pending.timer.Stop()
	}
	if policy != nil {
		pending.request.SeedingPolicy = cloneQBitSeedingPolicy(policy)
	}
	request := pending.request
	age := time.Since(pending.createdAt)
	q.pendingImportMu.Unlock()

	q.logger.Debug().
		Str("hash", request.Magnet.InfoHash).
		Str("name", request.Magnet.Name).
		Str("reason", reason).
		Dur("pending_for", age).
		Bool("has_seeding_policy", request.SeedingPolicy != nil).
		Msg("Releasing delayed qBittorrent torrent import")
	q.submitImportAsync(request, reason)
	return true
}

func (q *QBit) submitOrDelayImport(ctx context.Context, importReq *manager.ImportRequest) error {
	if !q.shouldDelayImport(importReq) {
		return q.submitImport(ctx, importReq)
	}
	return q.queuePendingImport(importReq)
}

// All torrent-related helpers goes here
func (q *QBit) addMagnet(ctx context.Context, url string, arr *arr.Arr, debrid string, action config.DownloadAction, callbackURL string, rmTrackerUrls, skipMultiSeason bool, seedingPolicy *storage.SeedingPolicy) error {
	magnet, err := utils.GetMagnetFromUrl(url, rmTrackerUrls)
	if err != nil {
		return fmt.Errorf("error parsing magnet link: %w", err)
	}

	importReq := manager.NewTorrentRequest(debrid, q.downloadFolder, magnet, arr, action, arr.DownloadUncached, callbackURL, manager.ImportTypeQBit, skipMultiSeason, seedingPolicy)

	err = q.submitOrDelayImport(ctx, importReq)
	if err != nil {
		return fmt.Errorf("failed to process torrent: %w", err)
	}
	return nil
}

func (q *QBit) addTorrent(ctx context.Context, fileHeader *multipart.FileHeader, arr *arr.Arr, debrid string, action config.DownloadAction, callbackURL string, rmTrackerUrls, skipMultiSeason bool, seedingPolicy *storage.SeedingPolicy) error {
	file, _ := fileHeader.Open()
	defer file.Close()
	var reader io.Reader = file
	magnet, err := utils.GetMagnetFromFile(reader, fileHeader.Filename, rmTrackerUrls)
	if err != nil {
		return fmt.Errorf("error reading file: %s \n %w", fileHeader.Filename, err)
	}
	importReq := manager.NewTorrentRequest(debrid, q.downloadFolder, magnet, arr, action, arr.DownloadUncached, callbackURL, manager.ImportTypeQBit, skipMultiSeason, seedingPolicy)
	err = q.submitOrDelayImport(ctx, importReq)
	if err != nil {
		return fmt.Errorf("failed to process torrent: %w", err)
	}
	return nil
}

func (q *QBit) ResumeTorrent(t *storage.Entry) bool {
	return true
}

func (q *QBit) PauseTorrent(t *storage.Entry) bool {
	return true
}

func (q *QBit) RefreshTorrent(t *storage.Entry) bool {
	return true
}

func (q *QBit) GetTorrentProperties(t *storage.Entry) *TorrentProperties {
	return &TorrentProperties{
		AdditionDate:       t.AddedOn.Unix(),
		Comment:            "Provider Blackhole <https://github.com/sirrobot01/decypharr>",
		CreatedBy:          "Provider Blackhole <https://github.com/sirrobot01/decypharr>",
		CreationDate:       t.AddedOn.Unix(),
		DlLimit:            -1,
		UpLimit:            -1,
		DlSpeed:            t.Speed,
		UpSpeed:            t.Speed,
		TotalSize:          t.Size,
		TotalUploaded:      t.Bytes,
		TotalDownloaded:    t.Bytes,
		LastSeen:           time.Now().Unix(),
		NbConnectionsLimit: 100,
		Peers:              0,
		PeersTotal:         2,
		SeedingTime:        1,
		Seeds:              100,
		ShareRatio:         100,
	}
}

func (q *QBit) setTorrentTags(t *storage.Entry, tags []string) {
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		if !utils.Contains(t.Tags, tag) {
			t.Tags = append(t.Tags, tag)
		}
		if !utils.Contains(q.Tags, tag) {
			q.Tags = append(q.Tags, tag)
		}
	}
	_ = q.manager.Queue().Update(t)
}

func (q *QBit) removeTorrentTags(t *storage.Entry, tags []string) bool {
	newTorrentTags := utils.RemoveItem(t.Tags, tags...)
	q.Tags = utils.RemoveItem(q.Tags, tags...)
	t.Tags = newTorrentTags
	_ = q.manager.Queue().Update(t)
	return true
}

func (q *QBit) addTags(tags []string) bool {
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		if !utils.Contains(q.Tags, tag) {
			q.Tags = append(q.Tags, tag)
		}
	}
	return true
}
