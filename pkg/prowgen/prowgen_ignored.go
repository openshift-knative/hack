package prowgen

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"sigs.k8s.io/yaml"
)

// IgnoredConfigPath is the path to the ignored branches configuration, shared
// between branch discovery (cmd/discover) and Konflux generation (prowgen).
const IgnoredConfigPath = "pkg/discover/ignored.yaml"

// Ignored describes a branch to be ignored, optionally scoped to a set of
// repositories. An empty Repos list means the ignore applies to all repositories.
type Ignored struct {
	Branch string   `json:"branch"`
	Repos  []string `json:"repos"`
}

// IgnoredBranches reads the ignored branches configuration.
func IgnoredBranches() ([]Ignored, error) {
	y, err := os.ReadFile(IgnoredConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", IgnoredConfigPath, err)
	}

	// Going directly from YAML raw input produces unexpected configs (due to missing YAML tags),
	// so we convert YAML to JSON and unmarshal the struct from the JSON object.
	j, err := yaml.YAMLToJSON(y)
	if err != nil {
		return nil, fmt.Errorf("failed to convert %q to JSON: %w", IgnoredConfigPath, err)
	}

	ignored := make([]Ignored, 0)
	if err := json.Unmarshal(j, &ignored); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config %q: %w", IgnoredConfigPath, err)
	}

	return ignored, nil
}

// IsBranchIgnored returns true if the branch is ignored for the given repository.
// Ignores without a repository restriction apply to all repositories.
func IsBranchIgnored(ignored []Ignored, branch, repo string) bool {
	for _, ignore := range ignored {
		if ignore.Branch != branch {
			continue
		}
		if len(ignore.Repos) == 0 || slices.Contains(ignore.Repos, repo) {
			return true
		}
	}
	return false
}
