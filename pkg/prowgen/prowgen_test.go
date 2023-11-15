package prowgen

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

type semVTest struct {
	name     string
	branches map[string]Branch
	want     []string
}

func TestDeleteOldBranches(t *testing.T) {
	tests := []semVTest{
		{
			name: "no previous minor, two previous majors",
			branches: map[string]Branch{
				"release-v3.1": {},
				"release-v2.9": {},
				"release-v2.0": {},
			},
			want: []string{
				"release-v1",
				"release-v0",
			},
		}, {
			name: "minor and major",
			branches: map[string]Branch{
				"release-v3.1": {},
				"release-v2.9": {},
				"release-v2.2": {},
			},
			want: []string{
				"release-v1",
				"release-v0",
				"release-v2.1",
				"release-v2.0",
			},
		}, {
			name: "only minors",
			branches: map[string]Branch{
				"release-v0.3": {},
			},
			want: []string{
				"release-v0.2",
				"release-v0.1",
				"release-v0.0",
			},
		}, {
			name: "only major, one previous major",
			branches: map[string]Branch{
				"release-v1.0": {},
			},
			want: []string{
				"release-v0",
			},
		}, {
			name: "no branch should be considered",
			branches: map[string]Branch{
				"main":         {},
				"release-next": {},
			},
			want: nil,
		}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getOldBranchCandidatePrefixes(tt.branches, "release-v")
			diff := cmp.Diff(tt.want, got)
			if diff != "" {
				t.Errorf("Unexpected tests (-want, +got): \n%s", diff)
			}
		})
	}
}
