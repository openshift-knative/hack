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

// TestCmpBranchesIsValidComparator asserts the contract slices.SortFunc relies
// on. Without it the sort order is unspecified, which previously let
// discover.go pick the wrong "latest" branch and silently skip discovery.
func TestCmpBranchesIsValidComparator(t *testing.T) {
	// Deliberately includes the cases that used to violate the contract:
	// ignored branches ("1.13"), unparseable names, and duplicates.
	branches := []string{
		"main", "release-next", "release-v1.13", "release-1.13",
		"release-v1.14", "release-v1.15", "release-v1.16",
		"serverless-1.34", "serverless-1.35", "garbage", "alsogarbage",
		"release-v1.15",
	}

	for _, a := range branches {
		if got := CmpBranches(a, a); got != 0 {
			t.Errorf("reflexivity: CmpBranches(%q, %q) = %d, want 0", a, a, got)
		}
		for _, b := range branches {
			ab, ba := CmpBranches(a, b), CmpBranches(b, a)
			if ab != -ba {
				t.Errorf("antisymmetry: CmpBranches(%q, %q) = %d but CmpBranches(%q, %q) = %d",
					a, b, ab, b, a, ba)
			}
		}
	}

	for _, a := range branches {
		for _, b := range branches {
			for _, c := range branches {
				if CmpBranches(a, b) < 0 && CmpBranches(b, c) < 0 && CmpBranches(a, c) >= 0 {
					t.Errorf("transitivity: %q < %q < %q but CmpBranches(%q, %q) = %d",
						a, b, c, a, c, CmpBranches(a, c))
				}
			}
		}
	}
}
