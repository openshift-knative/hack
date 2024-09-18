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
