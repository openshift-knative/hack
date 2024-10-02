package soversion

import (
	"fmt"
	"testing"

	"github.com/coreos/go-semver/semver"
)

func TestFromUpstreamVersion(t *testing.T) {
	tests := []struct {
		name     string
		upstream string
		want     string
	}{
		{
			upstream: "1.11.0",
			want:     "1.32.0",
		}, {
			upstream: "1.12.0",
			want:     "1.33.0",
		}, {
			upstream: "1.12.1",
			want:     "1.33.0",
		}, {
			upstream: "1.13.0",
			want:     "1.33.0",
		}, {
			upstream: "1.13.1",
			want:     "1.33.0",
		}, {
			upstream: "1.14.0",
			want:     "1.34.0",
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q -> %q", tt.upstream, tt.want), func(t *testing.T) {
			if got := FromUpstreamVersion(tt.upstream); got.String() != semver.New(tt.want).String() {
				t.Errorf("FromUpstreamVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToUpstreamVersion(t *testing.T) {
	tests := []struct {
		soVersion string
		want      string
	}{
		{
			soVersion: "1.32.0",
			want:      "1.11.0",
		}, {
			soVersion: "1.33.0",
			want:      "1.12.0",
		}, {
			soVersion: "1.33.1",
			want:      "1.12.0",
		}, {
			soVersion: "1.34.0",
			want:      "1.14.0",
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q -> %q", tt.soVersion, tt.want), func(t *testing.T) {
			if got := ToUpstreamVersion(tt.soVersion); got.String() != semver.New(tt.want).String() {
				t.Errorf("ToUpstreamVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIncrementBranchName(t *testing.T) {
	tests := []struct {
		soVersion string
		want      string
	}{
		{
			soVersion: "release-1.34",
			want:      "release-1.35",
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q -> %q", tt.soVersion, tt.want), func(t *testing.T) {
			if got := IncrementBranchName(tt.soVersion); got != tt.want {
				t.Errorf("ToUpstreamVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
