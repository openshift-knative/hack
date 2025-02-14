package rhel

import (
	"fmt"
	"regexp"

	"github.com/coreos/go-semver/semver"
	"github.com/openshift-knative/hack/config"
	"github.com/openshift-knative/hack/pkg/soversion"

	"github.com/openshift-knative/hack/pkg/prowgen"
)

func ForSOVersion(soVersion *semver.Version) (string, error) {
	soYaml, err := config.Configs.ReadFile("serverless-operator.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to load config for serverless-operator: %w", err)
	}

	soConfig, err := prowgen.UnmarshalConfig(soYaml)
	if err != nil {
		return "", fmt.Errorf("failed to parse config for serverless-operator: %w", err)
	}

	soBranchName := soversion.BranchName(soVersion)

	br, ok := soConfig.Config.Branches[soBranchName]
	if !ok {
		br, ok = soConfig.Config.Branches["main"]
		if !ok {
			return "", fmt.Errorf("main or %s branch configuration not found for serverless-operator", soBranchName)
		}
	}

	for _, img := range br.Konflux.ImageOverrides {
		if img.Name == "GO_BUILDER" || img.Name == "GO_RUNTIME" {
			v, err := extractRHELVersion(img.PullSpec)
			if err == nil {
				return v, nil
			}
			// don't abort on error. Try the next one instead...
		}
	}

	return "", fmt.Errorf("failed to find a matching rhel version from any of the image overrides for %s branch", soBranchName)
}

func extractRHELVersion(s string) (string, error) {
	re := regexp.MustCompile(`.*ubi(\d+)|rhel\D?(\d+).*`)
	match := re.FindStringSubmatch(s)

	if len(match) < 2 {
		return "", fmt.Errorf("failed to extract rhel version from %s", s)
	}

	// as we have 2 possible groups to match
	if match[1] != "" {
		return match[1], nil
	}

	return match[2], nil
}
