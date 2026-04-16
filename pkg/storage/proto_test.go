package storage

import "testing"

func TestEntryProtoRoundTripWithSeedingPolicy(t *testing.T) {
	ratio := 1.25
	minutes := 90
	entry := &Entry{
		InfoHash: "hash",
		Name:     "name",
		SeedingPolicy: &SeedingPolicy{
			StopOnRatio:      &ratio,
			StopAfterMinutes: &minutes,
			LastStopError:    "temporary error",
		},
	}

	roundTrip := ProtoToEntry(EntryToProto(entry))
	if roundTrip.SeedingPolicy == nil {
		t.Fatal("expected seeding policy after round-trip")
	}
	if roundTrip.SeedingPolicy.StopOnRatio == nil || *roundTrip.SeedingPolicy.StopOnRatio != ratio {
		t.Fatalf("unexpected ratio: %#v", roundTrip.SeedingPolicy.StopOnRatio)
	}
	if roundTrip.SeedingPolicy.StopAfterMinutes == nil || *roundTrip.SeedingPolicy.StopAfterMinutes != minutes {
		t.Fatalf("unexpected seeding time: %#v", roundTrip.SeedingPolicy.StopAfterMinutes)
	}
	if roundTrip.SeedingPolicy.LastStopError != "temporary error" {
		t.Fatalf("unexpected last stop error: %q", roundTrip.SeedingPolicy.LastStopError)
	}
}

func TestEntryProtoRoundTripWithoutSeedingPolicy(t *testing.T) {
	entry := &Entry{
		InfoHash: "hash",
		Name:     "name",
	}

	roundTrip := ProtoToEntry(EntryToProto(entry))
	if roundTrip.SeedingPolicy != nil {
		t.Fatalf("expected nil seeding policy, got %#v", roundTrip.SeedingPolicy)
	}
}

func TestEntryProtoRoundTripWithProviderRatio(t *testing.T) {
	entry := &Entry{
		InfoHash: "hash",
		Name:     "name",
		Providers: map[string]*ProviderEntry{
			"torbox-main": {
				Provider: "torbox-main",
				ID:       "torrent-id",
				Ratio:    1.75,
				Files:    map[string]*ProviderFile{},
			},
		},
	}

	roundTrip := ProtoToEntry(EntryToProto(entry))
	provider := roundTrip.Providers["torbox-main"]
	if provider == nil {
		t.Fatal("expected provider after round-trip")
	}
	if provider.Ratio != 1.75 {
		t.Fatalf("unexpected provider ratio: %v", provider.Ratio)
	}
}
