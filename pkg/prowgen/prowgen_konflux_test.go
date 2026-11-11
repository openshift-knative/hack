package prowgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetPrefetchDeps(t *testing.T) {
	tests := []struct {
		name string
		// files/dirs to create in the repo directory (relative paths)
		create    []string
		wantGomod bool
		wantRPM   bool
	}{
		{
			name:      "unvendored Go repo gets gomod prefetch",
			create:    []string{"go.mod"},
			wantGomod: true,
		},
		{
			// Hermeto supports vendored repos: it validates the vendor dir and
			// reports dependencies with hermeto:found_by annotations, which the
			// sbom_spdx.hermeto_attribution_required policy rule requires.
			name:      "vendored Go repo gets gomod prefetch",
			create:    []string{"go.mod", "vendor/modules.txt"},
			wantGomod: true,
		},
		{
			name:    "repo with rpms.lock.yaml gets RPM prefetch",
			create:  []string{"rpms.lock.yaml"},
			wantRPM: true,
		},
		{
			name:      "repo with both gets gomod and RPM prefetch",
			create:    []string{"go.mod", "vendor/modules.txt", "rpms.lock.yaml"},
			wantGomod: true,
			wantRPM:   true,
		},
		{
			name:   "non-Go repo gets no prefetch",
			create: []string{"README.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			repoDir := filepath.Join(root, "org", "repo")
			if err := os.MkdirAll(repoDir, 0o777); err != nil {
				t.Fatal(err)
			}
			for _, f := range tt.create {
				p := filepath.Join(repoDir, f)
				if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("test"), 0o666); err != nil {
					t.Fatal(err)
				}
			}

			t.Chdir(root)
			deps, err := getPrefetchDeps(Repository{Org: "org", Repo: "repo"})
			if err != nil {
				t.Fatalf("getPrefetchDeps() error: %v", err)
			}

			if got := strings.Contains(deps.PrefetchInput, `"gomod"`); got != tt.wantGomod {
				t.Errorf("gomod prefetch = %v, want %v (PrefetchInput: %s)", got, tt.wantGomod, deps.PrefetchInput)
			}
			if got := strings.Contains(deps.PrefetchInput, `"rpm"`); got != tt.wantRPM {
				t.Errorf("rpm prefetch = %v, want %v (PrefetchInput: %s)", got, tt.wantRPM, deps.PrefetchInput)
			}
		})
	}
}
