package sobranch

import (
	"fmt"
	"testing"
)

func TestFromUpstreamVersion(t *testing.T) {
	tests := []struct {
		name     string
		upstream string
		want     string
	}{
		{
			upstream: "1.11.0",
			want:     "release-1.32",
		}, {
			upstream: "1.12.0",
			want:     "release-1.33",
		}, {
			upstream: "1.12.1",
			want:     "release-1.33",
		}, {
			upstream: "1.13.0",
			want:     "release-1.33",
		}, {
			upstream: "1.13.1",
			want:     "release-1.33",
		}, {
			upstream: "1.14.0",
			want:     "release-1.34",
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q -> %q", tt.upstream, tt.want), func(t *testing.T) {
			if got := FromUpstreamVersion(tt.upstream); got != tt.want {
				t.Errorf("FromUpstreamVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
