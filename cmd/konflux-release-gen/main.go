package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/coreos/go-semver/semver"
	gyaml "github.com/ghodss/yaml"
	"github.com/openshift-knative/hack/pkg/konfluxgen"
	"github.com/openshift-knative/hack/pkg/project"
	"github.com/openshift-knative/hack/pkg/prowgen"
	"github.com/openshift-knative/hack/pkg/soversion"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.TODO()

	var environment, soRevision, overrideSnapshotDir, output, releaseType string
	pflag.StringVar(&environment, "environment", "prod", "Environment to use. Available values: [stage, prod]")
	pflag.StringVar(&soRevision, "so-revision", "main", "SO revision to get snapshots from")
	pflag.StringVar(&releaseType, "type", "component", "Type of the release. Available values: [component, fbc]")
	pflag.StringVar(&overrideSnapshotDir, "so-snapshot-directory", ".konflux-release", "The directory containing Serverless Operator override snapshots")
	pflag.StringVar(&output, "output", ".konflux", "Path to output directory")
	pflag.Parse()

	// clone SO repo to get metadata & snapshots for given revision
	soRepo := prowgen.Repository{Org: "openshift-knative", Repo: "serverless-operator"}
	if err := prowgen.GitClone(ctx, soRepo); err != nil {
		return fmt.Errorf("could not clone Git repository: %w", err)
	}

	if err := prowgen.GitCheckout(ctx, soRepo, soRevision); err != nil {
		return fmt.Errorf("could not checkout Git revision %s of Serverless Operator: %w", soRevision, err)
	}

	soProjectYamlPath := filepath.Join(soRepo.RepositoryDirectory(), "olm-catalog", "serverless-operator", "project.yaml")
	soMetadata, err := project.ReadMetadataFile(soProjectYamlPath)
	if err != nil {
		return fmt.Errorf("could not read project.yaml: %w", err)
	}

	soVersion := semver.New(soMetadata.Project.Version)
	soReleaseBranch := soversion.BranchName(soVersion)

	overrideSnapshotsPath := filepath.Join(soRepo.RepositoryDirectory(), overrideSnapshotDir)

	// clone hack repo so we can commit the changes
	hackRepo := prowgen.Repository{Org: "openshift-knative", Repo: "hack"}
	outputDir := filepath.Join(hackRepo.RepositoryDirectory(), output)

	if err := prowgen.GitMirror(ctx, hackRepo); err != nil {
		return fmt.Errorf("could not clone Git repository: %w", err)
	}

	if err := prowgen.GitCheckout(ctx, hackRepo, "main"); err != nil {
		return fmt.Errorf("could not checkout main branch of hack repo: %w", err)
	}

	releaseType = strings.ToLower(releaseType)
	if releaseType == "component" {
		snapshot, err := componentSnapshotName(overrideSnapshotsPath)
		if err != nil {
			return fmt.Errorf("could not get snapshot name: %w", err)
		}
		appName := konfluxgen.AppName(soReleaseBranch)
		releasePlan := konfluxgen.ReleasePlanAdmissionName(appName, soMetadata.Project.Version, environment) // releasePlanName == releasePlanAdmissionName

		cfg := konfluxgen.ReleaseConfig{
			Snapshot:            snapshot,
			ReleasePlan:         releasePlan,
			Environment:         environment,
			ResourcesOutputPath: outputDir,
		}

		if err := konfluxgen.GenerateRelease(cfg); err != nil {
			return fmt.Errorf("could not generate release: %w", err)
		}
	} else if releaseType == "fbc" {
		for _, ocpVersion := range soMetadata.Requirements.OcpVersion.List {
			ocpVersionFlat := strings.ReplaceAll(ocpVersion, ".", "")

			snapshot, err := fbcSnapshotName(overrideSnapshotsPath, ocpVersionFlat)
			if err != nil {
				return fmt.Errorf("could not get snapshot name: %w", err)
			}

			appName := konfluxgen.FBCAppName(soReleaseBranch, ocpVersionFlat)
			releasePlan := konfluxgen.ReleasePlanAdmissionName(appName, soMetadata.Project.Version, environment) // releasePlanName == releasePlanAdmissionName

			cfg := konfluxgen.ReleaseConfig{
				Snapshot:            snapshot,
				ReleasePlan:         releasePlan,
				Environment:         environment,
				ResourcesOutputPath: outputDir,
			}

			if err := konfluxgen.GenerateRelease(cfg); err != nil {
				return fmt.Errorf("could not generate release: %w", err)
			}
		}
	} else {
		return fmt.Errorf("invalid releaseType: %s", releaseType)
	}

	pushBranch := fmt.Sprintf("release-crs-%s-%s-%s", soRevision, releaseType, environment)
	commitMsg := fmt.Sprintf("Add %s Release CRs from %s revision for %s", releaseType, soRevision, environment)

	if err := prowgen.PushBranch(ctx, hackRepo, nil, pushBranch, commitMsg); err != nil {
		return fmt.Errorf("could not push to branch %s: %w", pushBranch, err)
	}

	return nil
}

func fbcSnapshotName(soReleaseFolder string, ocpVersion string) (string, error) {
	filename := fmt.Sprintf("override-snapshot-fbc-%s.yaml", ocpVersion)
	return parseSnapshotName(filepath.Join(soReleaseFolder, filename))
}

func componentSnapshotName(soReleaseFolder string) (string, error) {
	return parseSnapshotName(filepath.Join(soReleaseFolder, "override-snapshot.yaml"))
}

func parseSnapshotName(snapshotFile string) (string, error) {
	snapshot := metav1.PartialObjectMetadata{}

	y, err := os.ReadFile(snapshotFile)
	if err != nil {
		return "", fmt.Errorf("failed to read snapshot file: %w", err)
	}

	j, err := gyaml.YAMLToJSON(y)
	if err != nil {
		return "", fmt.Errorf("failed to convert snapshot file to json: %w", err)
	}

	if err := json.Unmarshal(j, &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal snapshot file: %w", err)
	}

	if snapshot.Name == "" {
		return "", fmt.Errorf("snapshot.Name is empty")
	}

	return snapshot.Name, nil
}
