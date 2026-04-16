package manager

import (
	"context"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

const (
	seedingPolicyBatchSize = 100
	seedingPolicyWorkers   = 4
)

func (m *Manager) processSeedingPolicies(ctx context.Context) {
	candidates := make([]string, 0, seedingPolicyBatchSize)

	err := m.storage.ForEachBatch(seedingPolicyBatchSize, func(batch []*storage.Entry) error {
		for _, entry := range batch {
			if entry.Protocol != config.ProtocolTorrent || entry.SeedingPolicy == nil {
				continue
			}
			if entry.SeedingPolicy.StopCompletedAt != nil || entry.ActiveProvider == "" {
				continue
			}
			candidates = append(candidates, entry.InfoHash)
		}
		return nil
	})
	if err != nil {
		m.logger.Error().Err(err).Msg("Failed to scan entries for seeding policies")
		return
	}
	if len(candidates) == 0 {
		return
	}

	workCh := make(chan string, len(candidates))
	workers := min(seedingPolicyWorkers, len(candidates))
	done := make(chan struct{}, workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for infohash := range workCh {
				select {
				case <-ctx.Done():
					return
				default:
					m.processSeedingPolicyEntry(infohash)
				}
			}
		}()
	}

	for _, infohash := range candidates {
		workCh <- infohash
	}
	close(workCh)

	for i := 0; i < workers; i++ {
		<-done
	}
}

func (m *Manager) processSeedingPolicyEntry(infohash string) {
	entry, err := m.storage.Get(infohash)
	if err != nil || entry == nil || entry.SeedingPolicy == nil || entry.SeedingPolicy.StopCompletedAt != nil {
		return
	}

	client := m.ProviderClient(entry.ActiveProvider)
	if client == nil || client.Config().Provider != "torbox" {
		return
	}

	placement := entry.GetActiveProvider()
	if placement == nil || placement.ID == "" {
		return
	}

	downloadedAt := placement.DownloadedAt
	if downloadedAt == nil {
		downloadedAt = entry.CompletedAt
	}
	if downloadedAt == nil {
		return
	}

	now := time.Now()
	timeDue := false
	var deadline *time.Time
	if entry.SeedingPolicy.StopAfterMinutes != nil {
		d := downloadedAt.Add(time.Duration(*entry.SeedingPolicy.StopAfterMinutes) * time.Minute)
		deadline = &d
		timeDue = !now.Before(d)
	}

	ratioDue := false
	var currentRatio *float64
	if entry.SeedingPolicy.StopOnRatio != nil && !timeDue {
		remote, err := client.GetTorrent(placement.ID)
		if err != nil {
			entry.SeedingPolicy.LastStopError = err.Error()
			if saveErr := m.storage.AddOrUpdate(entry); saveErr != nil {
				m.logger.Error().Err(saveErr).Str("infohash", infohash).Msg("Failed to persist seeding policy fetch error")
			}
			m.logger.Error().
				Err(err).
				Str("infohash", infohash).
				Str("provider", entry.ActiveProvider).
				Str("provider_id", placement.ID).
				Msg("Failed to fetch torrent ratio for seeding policy")
			return
		}
		ratio := remote.Ratio
		currentRatio = &ratio
		ratioDue = ratio >= *entry.SeedingPolicy.StopOnRatio
		if entry.SeedingPolicy.LastStopError != "" {
			entry.SeedingPolicy.LastStopError = ""
			if saveErr := m.storage.AddOrUpdate(entry); saveErr != nil {
				m.logger.Error().Err(saveErr).Str("infohash", infohash).Msg("Failed to clear seeding policy fetch error")
			}
		}
	}

	if !timeDue && !ratioDue {
		return
	}

	if entry.SeedingPolicy.StopRequestedAt == nil {
		requestedAt := now
		entry.SeedingPolicy.StopRequestedAt = &requestedAt
		if err := m.storage.AddOrUpdate(entry); err != nil {
			m.logger.Error().Err(err).Str("infohash", infohash).Msg("Failed to persist seeding stop request timestamp")
			return
		}
	}

	logEvent := m.logger.Info().
		Str("infohash", infohash).
		Str("provider", entry.ActiveProvider).
		Str("provider_id", placement.ID).
		Time("downloaded_at", *downloadedAt)

	if entry.SeedingPolicy.StopOnRatio != nil {
		logEvent = logEvent.Float64("ratio_limit", *entry.SeedingPolicy.StopOnRatio)
	}
	if entry.SeedingPolicy.StopAfterMinutes != nil {
		logEvent = logEvent.Int("seeding_time_limit_minutes", *entry.SeedingPolicy.StopAfterMinutes)
	}
	if deadline != nil {
		logEvent = logEvent.Time("deadline", *deadline)
	}
	if currentRatio != nil {
		logEvent = logEvent.Float64("current_ratio", *currentRatio)
	}
	logEvent.Msg("Stopping TorBox seeding for entry")

	if err := client.StopSeeding(placement.ID); err != nil {
		entry.SeedingPolicy.LastStopError = err.Error()
		if saveErr := m.storage.AddOrUpdate(entry); saveErr != nil {
			m.logger.Error().Err(saveErr).Str("infohash", infohash).Msg("Failed to persist seeding stop error")
		}
		m.logger.Error().
			Err(err).
			Str("infohash", infohash).
			Str("provider", entry.ActiveProvider).
			Str("provider_id", placement.ID).
			Msg("Failed to stop TorBox seeding")
		return
	}

	completedAt := time.Now()
	entry.SeedingPolicy.StopCompletedAt = &completedAt
	entry.SeedingPolicy.LastStopError = ""
	if err := m.storage.AddOrUpdate(entry); err != nil {
		m.logger.Error().Err(err).Str("infohash", infohash).Msg("Failed to persist seeding stop success")
		return
	}

	m.logger.Info().
		Str("infohash", infohash).
		Str("provider", entry.ActiveProvider).
		Str("provider_id", placement.ID).
		Time("stopped_at", completedAt).
		Msg("Stopped TorBox seeding for entry")
}
