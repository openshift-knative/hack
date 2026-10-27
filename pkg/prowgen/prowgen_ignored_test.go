package prowgen

import (
	"testing"
)

func TestIsBranchIgnored(t *testing.T) {
	ignored := []Ignored{
		{Branch: "release-1.36"},
		{Branch: "release-1.19", Repos: []string{"kn-plugin-event"}},
	}

	testCases := []struct {
		branch string
		repo   string
		want   bool
	}{
		// Ignores without repos apply to all repositories
		{"release-1.36", "serverless-operator", true},
		{"release-1.36", "kn-plugin-event", true},
		// Repo-scoped ignores only apply to the listed repositories
		{"release-1.19", "kn-plugin-event", true},
		{"release-1.19", "serverless-operator", false},
		// Branches without an ignore entry are not ignored
		{"release-1.35", "serverless-operator", false},
	}

	for _, tc := range testCases {
		if got := IsBranchIgnored(ignored, tc.branch, tc.repo); got != tc.want {
			t.Errorf("IsBranchIgnored(%q, %q) = %v, want %v", tc.branch, tc.repo, got, tc.want)
		}
	}
}
