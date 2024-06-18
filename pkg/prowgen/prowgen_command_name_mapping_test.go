package prowgen

import (
	"fmt"
	"strings"
	"testing"
)

func TestToName(t *testing.T) {
	continuousSuffix := "-c"

	tests := []struct {
		name string
		r    Repository
		test *Test
		want string
	}{
		{
			name: fmt.Sprintf("%d length name", maxNameLength),
			r:    Repository{},
			test: &Test{
				Command: strings.Repeat("a", maxNameLength),
			},
			want: fmt.Sprintf("%s-%s", strings.Repeat("a", maxNameLength-len(continuousSuffix)-shaLength-1) /* hex sha1 */, "32e067e"),
		},
		{
			name: fmt.Sprintf("%d length name", maxNameLength-len(continuousSuffix)+1),
			r:    Repository{},
			test: &Test{
				Command: strings.Repeat("a", maxNameLength-len(continuousSuffix)+1),
			},
			want: fmt.Sprintf("%s-%s", strings.Repeat("a", maxNameLength-len(continuousSuffix)-shaLength-1) /* hex sha1 */, "52cedd6"),
		},
		{
			name: fmt.Sprintf("%d length name", maxNameLength-len(continuousSuffix)),
			r:    Repository{},
			test: &Test{
				Command: strings.Repeat("a", maxNameLength-len(continuousSuffix)),
			},
			want: strings.Repeat("a", maxNameLength-len(continuousSuffix)),
		},
		{
			name: "test-conformance name",
			r:    Repository{},
			test: &Test{
				Command: "test-conformance",
			},
			want: "test-conformance",
		},
		{
			name: "test-kafka-broker-conformance-upstream-nightly",
			r:    Repository{},
			test: &Test{
				Command: "test-kafka-broker-conformance-upstream-nightly",
			},
			want: "test-kafka-broker-conformance-up-9c189d7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.want) > maxNameLength-len(continuousSuffix) {
				t.Fatalf("Test misconfiguration want cannot be longer than %d, got %d", maxNameLength-len(continuousSuffix), len(tt.want))
			}
			got := ToName(tt.r, tt.test)
			if got != tt.want {
				t.Errorf("ToName() = %v (length %d), want %v (length %d)", got, len(got), tt.want, len(tt.want))
			}
			t.Logf("ToName() = %v (length %d), want %v (length %d)", got, len(got), tt.want, len(tt.want))
		})
	}
}
