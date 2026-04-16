package qbit

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/logger"
	debrid "github.com/sirrobot01/decypharr/pkg/debrid/common"
	"github.com/sirrobot01/decypharr/pkg/manager"
)

const pendingQBitShareLimitWindow = 10 * time.Second

type pendingTorrentImport struct {
	request   *manager.ImportRequest
	timer     *time.Timer
	createdAt time.Time
}

type QBit struct {
	downloadFolder          string
	categories              []string
	alwaysRemoveTrackerURLS bool
	logger                  zerolog.Logger
	Tags                    []string
	manager                 *manager.Manager
	pendingImportMu         sync.Mutex
	pendingImports          map[string]*pendingTorrentImport
}

func New(manager *manager.Manager) *QBit {
	cfg := config.Get()
	return &QBit{
		downloadFolder:          cfg.DownloadFolder,
		categories:              cfg.Categories,
		alwaysRemoveTrackerURLS: cfg.AlwaysRmTrackerUrls,
		manager:                 manager,
		logger:                  logger.New("qbit"),
		pendingImports:          make(map[string]*pendingTorrentImport),
	}
}

func (q *QBit) isTorboxTarget(importReq *manager.ImportRequest) bool {
	if importReq == nil {
		return false
	}

	if importReq.SelectedDebrid != "" {
		client := q.manager.ProviderClient(importReq.SelectedDebrid)
		return client != nil && client.Config().Provider == "torbox"
	}

	return q.manager.HasDebridProvider("torbox")
}

func (q *QBit) providerName(importReq *manager.ImportRequest) string {
	if importReq == nil || importReq.SelectedDebrid == "" {
		return ""
	}
	client := q.manager.ProviderClient(importReq.SelectedDebrid)
	if client == nil {
		return ""
	}
	return client.Config().Provider
}

func (q *QBit) getProvider(importReq *manager.ImportRequest) debrid.Client {
	if importReq == nil || importReq.SelectedDebrid == "" {
		return nil
	}
	return q.manager.ProviderClient(importReq.SelectedDebrid)
}
