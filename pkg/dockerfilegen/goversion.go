package dockerfilegen

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/coreos/go-semver/semver"
	"github.com/openshift-knative/hack/pkg/project"
	"github.com/openshift-knative/hack/pkg/prowgen"
	"github.com/openshift-knative/hack/pkg/soversion"
)

func componentBranchForTag(tag string) string {
	if tag == "knative-nightly" {
		return "release-next"
	}
	return fmt.Sprintf("release-%s", strings.TrimPrefix(tag, "knative-"))
}

func soBranchForMetadata(metadata *project.Metadata) string {
	if metadata.Project.Tag != "" {
		if metadata.Project.Tag == "knative-nightly" || metadata.Project.Tag == "main" {
			return "main"
		}
		upstream := strings.TrimPrefix(metadata.Project.Tag, "knative-")
		return soversion.BranchName(soversion.FromUpstreamVersion(upstream))
	}
	if metadata.Project.Version != "" {
		v, err := semver.NewVersion(metadata.Project.Version)
		if err != nil {
			return ""
		}
		return soversion.BranchName(v)
	}
	return ""
}

func hasImagePrefix(repos []prowgen.Repository, prefix string) bool {
	for _, r := range repos {
		if r.ImagePrefix == prefix {
			return true
		}
	}
	return false
}

func golangVersionFromRepoConfig(configs fs.FS, imagePrefix, branch string) (*string, error) {
	configFiles, err := fs.ReadDir(configs, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to read config directory: %w", err)
	}
	for _, configFile := range configFiles {
		if configFile.IsDir() {
			continue
		}
		content, err := fs.ReadFile(configs, configFile.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", configFile.Name(), err)
		}
		cfg, err := prowgen.UnmarshalConfig(content)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config from %s: %w", configFile.Name(), err)
		}
		if !hasImagePrefix(cfg.Repositories, imagePrefix) {
			continue
		}
		branchConfig, ok := cfg.Config.Branches[branch]
		if !ok {
			continue
		}
		if branchConfig.GolangVersion != nil && *branchConfig.GolangVersion != "" {
			return branchConfig.GolangVersion, nil
		}
	}
	return nil, nil
}

func golangVersionFromSOConfig(configs fs.FS, branch string) (*string, error) {
	soYaml, err := fs.ReadFile(configs, "serverless-operator.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load config for serverless-operator: %w", err)
	}
	soConfig, err := prowgen.UnmarshalConfig(soYaml)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config for serverless-operator: %w", err)
	}
	cfg, ok := soConfig.Config.Branches[branch]
	if !ok {
		return nil, nil
	}
	if cfg.GolangVersion != nil && *cfg.GolangVersion != "" {
		return cfg.GolangVersion, nil
	}
	return nil, nil
}

// goVersionFromConfig returns the Go version for the project. It checks the
// repo-branch config first, then falls back to the SO branch config, returning
// nil if neither defines one.
func goVersionFromConfig(configs fs.FS, metadata *project.Metadata) (*string, error) {
	if imagePrefix := metadata.Project.ImagePrefix; imagePrefix != "" {
		branch := componentBranchForTag(metadata.Project.Tag)
		v, err := golangVersionFromRepoConfig(configs, imagePrefix, branch)
		if err != nil {
			return nil, err
		}
		if v != nil {
			return v, nil
		}
	}
	soBranch := soBranchForMetadata(metadata)
	if soBranch == "" {
		return nil, nil
	}
	return golangVersionFromSOConfig(configs, soBranch)
}
