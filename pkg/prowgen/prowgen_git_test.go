package prowgen

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCmpBranches(t *testing.T) {
	tests := []struct {
		versions       []string
		expectedSorted []string
	}{
		{
			versions:       []string{"release-v1.15", "release-v1.14", "release-v1.16"},
			expectedSorted: []string{"release-v1.14", "release-v1.15", "release-v1.16"},
		},
		{
			versions:       []string{"release-v1.15", "release-v1.16", "serverless-1.35"},
			expectedSorted: []string{"release-v1.15", "serverless-1.35", "release-v1.16"},
		},
		{
			versions:       []string{"release-v1.15", "release-v1.16", "serverless-1.34"},
			expectedSorted: []string{"serverless-1.34", "release-v1.15", "release-v1.16"},
		},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			sorted := tt.versions
			//testing indirect via stort func, as this is easier to read
			slices.SortFunc(sorted, CmpBranches)
			if diff := cmp.Diff(tt.expectedSorted, sorted); diff != "" {
				t.Error("CmpBranches() (-want, +got):", diff)
			}
		})
	}
}
