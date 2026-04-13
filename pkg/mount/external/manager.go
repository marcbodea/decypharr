package external

import (
	"context"
	"strings"

	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/logger"
	"github.com/sirrobot01/decypharr/internal/rclone"
	"github.com/sirrobot01/decypharr/pkg/manager"
)

type Manager struct {
	manager *manager.Manager
	client  *rclone.Client
	logger  zerolog.Logger
}

// NewManager creates a new external rclone manager
// This does nothing, just a placeholder to satisfy the interface
func NewManager(manager *manager.Manager) *Manager {
	_logger := logger.New("external")
	cfg := config.Get()
	rcloneClient := rclone.NewClient(
		cfg.Mount.ExternalRclone.RCUrl,
		cfg.Mount.ExternalRclone.RCUsername,
		cfg.Mount.ExternalRclone.RCPassword,
		_logger,
	)
	m := &Manager{
		manager: manager,
		logger:  _logger,
		client:  rcloneClient,
	}
	return m
}

func (m *Manager) Start(ctx context.Context) error {
	return nil
}

func (m *Manager) Stop() error {
	return nil
}

func (m *Manager) Refresh(dirs []string) error {
	return m.client.Refresh(context.Background(), normalizeRefreshDirs(dirs), "")
}

func (m *Manager) IsReady() bool {
	return true
}

func (m *Manager) Type() string {
	return "external"
}

func normalizeRefreshDirs(dirs []string) []string {
	if len(dirs) == 0 {
		return []string{""}
	}

	normalized := make([]string, 0, len(dirs))
	seen := make(map[string]struct{}, len(dirs))

	for _, dir := range dirs {
		dir = strings.TrimPrefix(dir, "/")
		dir = strings.TrimSpace(dir)

		switch {
		case dir == "", dir == manager.EntryAllFolder:
			dir = ""
		case strings.HasPrefix(dir, manager.EntryAllFolder+"/"):
			dir = strings.TrimPrefix(dir, manager.EntryAllFolder+"/")
		}

		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		normalized = append(normalized, dir)
	}

	if len(normalized) == 0 {
		return []string{""}
	}

	return normalized
}
