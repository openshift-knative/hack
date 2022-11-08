package prowgen

import (
	"fmt"
	"strings"
	"testing"
)

func TestToName(t *testing.T) {

	openshiftVersion := "4.11"
	suffix := "-aws-ocp-411"
	continuousSuffix := "-continuous"

	tests := []struct {
		name             string
		r                Repository
		test             *Test
		openShiftVersion string
		want             string
	}{
		{
			name: fmt.Sprintf("%d length name", maxNameLength),
			r:    Repository{},
			test: &Test{
				Command: strings.Repeat("a", maxNameLength),
			},
			openShiftVersion: openshiftVersion,
			want:             fmt.Sprintf("%s-%s%s", strings.Repeat("a", maxNameLength-len(suffix)-len(continuousSuffix)-shaLength-1) /* hex sha1 */, "6161616", suffix),
		},
		{
			name: fmt.Sprintf("%d length name", maxNameLength-len(suffix)-len(continuousSuffix)+1),
			r:    Repository{},
			test: &Test{
				Command: strings.Repeat("a", maxNameLength-len(suffix)-len(continuousSuffix)+1),
			},
			openShiftVersion: openshiftVersion,
			want:             fmt.Sprintf("%s-%s%s", strings.Repeat("a", maxNameLength-len(suffix)-len(continuousSuffix)-shaLength-1) /* hex sha1 */, "6161616", suffix),
		},
		{
			name: fmt.Sprintf("%d length name", maxNameLength-len(suffix)-len(continuousSuffix)),
			r:    Repository{},
			test: &Test{
				Command: strings.Repeat("a", maxNameLength-len(suffix)-len(continuousSuffix)),
			},
			openShiftVersion: openshiftVersion,
			want:             fmt.Sprintf("%s%s", strings.Repeat("a", maxNameLength-len(suffix)-len(continuousSuffix)), suffix),
		},
		{
			name: "test-conformance name",
			r:    Repository{},
			test: &Test{
				Command: "test-conformance",
			},
			openShiftVersion: openshiftVersion,
			want:             fmt.Sprintf("%s%s", "test-conformance", suffix),
		},
		{
			name: "test-kafka-broker-upstream-nightly",
			r:    Repository{},
			test: &Test{
				Command: "test-kafka-broker-upstream-nightly",
			},
			openShiftVersion: openshiftVersion,
			want:             fmt.Sprintf("%s%s", "test-kafka--7465737", suffix),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.want) > maxNameLength-len(continuousSuffix) {
				t.Fatalf("Test misconfiguration want cannot be longer than %d, got %d", maxNameLength-len(continuousSuffix), len(tt.want))
			}
			got := ToName(tt.r, tt.test, tt.openShiftVersion)
			if got != tt.want {
				t.Errorf("ToName() = %v (length %d), want %v (length %d)", got, len(got), tt.want, len(tt.want))
			}
			t.Logf("ToName() = %v (length %d), want %v (length %d)", got, len(got), tt.want, len(tt.want))
		})
	}
}
