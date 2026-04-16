package qbit

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestParseSeedingPolicy(t *testing.T) {
	tests := []struct {
		name        string
		values      url.Values
		wantRatio   *float64
		wantMinutes *int
		wantErr     bool
	}{
		{
			name:        "unset values return nil",
			values:      url.Values{},
			wantRatio:   nil,
			wantMinutes: nil,
		},
		{
			name: "explicit values are parsed",
			values: url.Values{
				"ratioLimit":       []string{"1.5"},
				"seedingTimeLimit": []string{"60"},
			},
			wantRatio:   ptrFloat64(1.5),
			wantMinutes: ptrInt(60),
		},
		{
			name: "special negative values are ignored",
			values: url.Values{
				"ratioLimit":       []string{"-2"},
				"seedingTimeLimit": []string{"-1"},
			},
			wantRatio:   nil,
			wantMinutes: nil,
		},
		{
			name: "other negative ratio returns error",
			values: url.Values{
				"ratioLimit": []string{"-3"},
			},
			wantErr: true,
		},
		{
			name: "other negative seeding time returns error",
			values: url.Values{
				"seedingTimeLimit": []string{"-3"},
			},
			wantErr: true,
		},
		{
			name: "invalid ratio returns error",
			values: url.Values{
				"ratioLimit": []string{"abc"},
			},
			wantErr: true,
		},
		{
			name: "invalid seeding time returns error",
			values: url.Values{
				"seedingTimeLimit": []string{"abc"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v2/torrents/add", nil)
			req.Form = tt.values

			policy, err := parseSeedingPolicy(req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSeedingPolicy returned error: %v", err)
			}

			if tt.wantRatio == nil && tt.wantMinutes == nil {
				if policy != nil {
					t.Fatalf("expected nil policy, got %#v", policy)
				}
				return
			}
			if policy == nil {
				t.Fatal("expected policy")
			}
			if !floatPtrEqual(policy.StopOnRatio, tt.wantRatio) {
				t.Fatalf("unexpected ratio limit: got %v want %v", policy.StopOnRatio, tt.wantRatio)
			}
			if !intPtrEqual(policy.StopAfterMinutes, tt.wantMinutes) {
				t.Fatalf("unexpected seeding time limit: got %v want %v", policy.StopAfterMinutes, tt.wantMinutes)
			}
		})
	}
}

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt(v int) *int             { return &v }

func floatPtrEqual(got, want *float64) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func intPtrEqual(got, want *int) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func TestSplitHashes(t *testing.T) {
	hashes := splitHashes("abc|def,ghi\njkl")
	if len(hashes) != 4 {
		t.Fatalf("unexpected hash count: got %d want 4", len(hashes))
	}
	want := []string{"abc", "def", "ghi", "jkl"}
	for i := range want {
		if hashes[i] != want[i] {
			t.Fatalf("unexpected hash at %d: got %q want %q", i, hashes[i], want[i])
		}
	}
}
