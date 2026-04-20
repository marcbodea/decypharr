package manager

import (
	"fmt"

	"github.com/sirrobot01/decypharr/pkg/storage"
)

func (m *Manager) RemoveFromProvider(providerEntry *storage.ProviderEntry) error {
	if providerEntry == nil {
		return nil
	}
	if providerEntry.Provider == "usenet" {
		if m.usenet != nil {
			return m.usenet.Delete(providerEntry.ID)
		}
		return nil
	}

	client := m.ProviderClient(providerEntry.Provider)
	if client == nil {
		return nil
	}
	return client.DeleteTorrent(providerEntry.ID)
}

func (m *Manager) RemoveTorrentPlacements(t *storage.Entry) error {
	if t == nil {
		return nil
	}

	var errs []error
	for _, placement := range t.Providers {
		if err := m.RemoveFromProvider(placement); err != nil {
			errs = append(errs, fmt.Errorf("%s (%s): %w", placement.Provider, placement.ID, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to remove provider placements: %v", errs)
	}

	return nil
}
