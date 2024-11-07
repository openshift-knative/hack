package prowgen

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToName(t *testing.T) {

	openshiftVersion := "4.11"
	continuousSuffix := "-c"

	tests := []struct {
		name             string
		r                Repository
		test             *Test
		openShiftVersion string
		want             string
	}{{
		name: "max length name",
		r:    Repository{},
		test: &Test{
			Command: strings.Repeat("a", maxNameLength),
		},
		openShiftVersion: openshiftVersion,
		want:             fmt.Sprintf("%s-%s", strings.Repeat("a", maxNameLength-len(continuousSuffix)-shaLength-1) /* hex sha1 */, "32e067e"),
	}, {
		name: "max length name without continuous +1",
		r:    Repository{},
		test: &Test{
			Command: strings.Repeat("a", maxNameLength-len(continuousSuffix)+1),
		},
		openShiftVersion: openshiftVersion,
		want:             fmt.Sprintf("%s-%s", strings.Repeat("a", maxNameLength-len(continuousSuffix)-shaLength-1) /* hex sha1 */, "52cedd6"),
	}, {
		name: "max length name without continuous",
		r:    Repository{},
		test: &Test{
			Command: strings.Repeat("a", maxNameLength-len(continuousSuffix)),
		},
		openShiftVersion: openshiftVersion,
		want:             strings.Repeat("a", maxNameLength-len(continuousSuffix)),
	}, {
		name: "test-conformance name",
		r:    Repository{},
		test: &Test{
			Command: "test-conformance",
		},
		openShiftVersion: openshiftVersion,
		want:             "test-conformance",
	}, {
		name: "test-kafka-broker-filter-upstream-nightly",
		r:    Repository{},
		test: &Test{
			Command: "test-kafka-broker-filter-upstream-nightly",
		},
		openShiftVersion: openshiftVersion,
		want:             "test-kafka-broker-filter-upstrea-b7f30b5",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.want) > maxNameLength-len(continuousSuffix) {
				t.Fatalf("Test misconfiguration want cannot be longer than %d, got %d", maxNameLength-len(continuousSuffix), len(tt.want))
			}
			got := ToName(tt.r, tt.test)
			assert.Equal(t, tt.want, got)
		})
	}
}
