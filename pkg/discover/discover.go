package discover

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jinzhu/copier"

	gyaml "github.com/ghodss/yaml"

	"github.com/openshift-knative/hack/pkg/action"
	"github.com/openshift-knative/hack/pkg/konfluxgen"
	"github.com/openshift-knative/hack/pkg/prowgen"
	"github.com/openshift-knative/hack/pkg/soversion"
)

func Main() {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	inputConfig := flag.String("config", filepath.Join("config"), "Specify repositories config")
	inputAction := flag.String("input", filepath.Join(".github", "workflows", "release-generate-ci-template.yaml"), "Input action (template)")
	outputAction := flag.String("output", filepath.Join(".github", "workflows", "release-generate-ci.yaml"), "Output action")
	flag.Parse()

	err := filepath.Walk(*inputConfig, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		if err := discover(ctx, path); err != nil {
			return fmt.Errorf("failed to discover config for %s: %w", path, err)
		}

		return nil
	})
	if err != nil {
		log.Fatalln("Failed to walk path", *inputConfig, err)
	}

	err = action.UpdateAction(ctx, action.Config{
		InputAction:     *inputAction,
		InputConfigPath: *inputConfig,
		OutputAction:    *outputAction,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func discover(ctx context.Context, path string) error {
	inConfig := &prowgen.Config{}
	if err := readYaml(path, inConfig); err != nil {
		return err
	}

	inConfig, err := removeUnsupportedBranches(ctx, inConfig)
	if err != nil {
		return fmt.Errorf("failed to remove unsupported branches: %w", err)
	}

	for _, r := range inConfig.Repositories {
		if len(inConfig.Config.Branches) == 0 {
			continue // nothing to do here
		}

		configuredBranches := make([]string, 0, len(inConfig.Config.Branches))
		for branchName, _ := range inConfig.Config.Branches {
			configuredBranches = append(configuredBranches, branchName)
		}
		slices.SortFunc(configuredBranches, prowgen.CmpBranches)
		latestConfigured := configuredBranches[len(configuredBranches)-1]

		latest := latestConfigured
		if _, ok := inConfig.Config.Branches["release-next"]; ok {
			latest = "release-next"
		}

		availableBranches, err := prowgen.Branches(ctx, r)
		if err != nil {
			return err
		}
		latestAvailable := ""
		if len(availableBranches) > 0 {
			latestAvailable = availableBranches[len(availableBranches)-1]
		}

		log.Println(r.RepositoryDirectory(), "Latest branch", latest, ", latest available", latestAvailable, ", latest configured", latestConfigured)

		for i := 0; i < len(availableBranches); i++ {
			if latestConfigured == availableBranches[i] {
				for ; i < len(availableBranches); i++ {
					if _, ok := inConfig.Config.Branches[availableBranches[i]]; !ok {
						branchConfig := inConfig.Config.Branches[latest]

						other := prowgen.Branch{}
						// copy the whole branchConfig as this contains some pointers,
						// and it would otherwise update the existing branch config
						if err := copier.Copy(&other, &branchConfig); err != nil {
							return fmt.Errorf("could not copy branchconfig: %w", err)
						}

						// enable Konflux for all new branches
						if other.Konflux == nil {
							other.Konflux = &prowgen.Konflux{
								Enabled: true,
							}
						} else {
							other.Konflux.Enabled = true
						}

						inConfig.Config.Branches[availableBranches[i]] = other
					}
				}
			}
		}
		if r.IsServerlessOperator() {
			if latestAvailable == latestConfigured || latestConfigured == "main" {
				branchConfig := inConfig.Config.Branches[latest]

				other := prowgen.Branch{}
				// copy the whole branchConfig as this contains some pointers,
				// and it would otherwise update the existing branch config
				if err := copier.Copy(&other, &branchConfig); err != nil {
					return fmt.Errorf("could not copy branchconfig: %w", err)
				}

				// enable Konflux for all new branches
				if other.Konflux == nil {
					other.Konflux = &prowgen.Konflux{
						Enabled: true,
					}
				} else {
					other.Konflux.Enabled = true
				}
				if other.Prowgen == nil {
					other.Prowgen = &prowgen.Prowgen{Disabled: true}
				} else {
					other.Prowgen.Disabled = true
				}

				next := soversion.IncrementBranchName(latestAvailable)

				inConfig.Config.Branches[next] = other
			}
		}
	}

	return writeYaml(path, inConfig)
}

type Unsupported struct {
	Version string `json:"version" yaml:"version"`
	Date    string `json:"date" yaml:"date"`
}

func removeUnsupportedBranches(ctx context.Context, in *prowgen.Config) (*prowgen.Config, error) {

	const unsupportedConfig = "pkg/discover/unsupported.yaml"

	unsupportedBranches := make([]Unsupported, 0)
	if err := readYaml(unsupportedConfig, &unsupportedBranches); err != nil {
		return nil, err
	}

	futureUnsupportedBranchesMap := make(map[string]Unsupported, 2)

	for branch := range in.Config.Branches {
		for _, un := range unsupportedBranches {
			when, err := time.Parse("2006-01-02", un.Date)
			if err != nil {
				return in, fmt.Errorf("failed to parse date %q: %v", un.Date, err)
			}
			if time.Now().UTC().Before(when) {
				futureUnsupportedBranchesMap[un.Version] = un
				continue
			}
			dv := soversion.FromUpstreamVersion(branch)
			if strings.Contains(branch, un.Version) || strings.Contains(dv.String(), un.Version) {
				removeKonfluxResources(un.Version)
				removeKonfluxResources(fmt.Sprintf("%d.%d", dv.Major, dv.Minor))
				delete(in.Config.Branches, branch)
			}

		}
	}

	futureUnsupportedBranches := make([]Unsupported, 0, len(futureUnsupportedBranchesMap))
	for _, v := range futureUnsupportedBranchesMap {
		futureUnsupportedBranches = append(futureUnsupportedBranches, v)
	}
	slices.SortStableFunc(futureUnsupportedBranches, func(a, b Unsupported) int {
		return strings.Compare(a.Version, b.Version)
	})
	if err := writeYaml(unsupportedConfig, futureUnsupportedBranches); err != nil {
		return in, err
	}

	return in, nil
}

func removeKonfluxResources(version string) {
	matches, err := filepath.Glob(fmt.Sprintf(".konflux/**/serverless-operator-%s*", konfluxgen.Sanitize(version)))
	if err != nil {
		panic(err)
	}
	for _, match := range matches {
		if err := os.RemoveAll(match); err != nil {
			panic(err)
		}
	}
}

func readYaml(path string, out any) error {
	log.Println("Reading YAML from", path)
	// Going directly from YAML raw input produces unexpected configs (due to missing YAML tags),
	// so we convert YAML to JSON and unmarshal the struct from the JSON object.
	y, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %q: %w", path, err)
	}
	j, err := gyaml.YAMLToJSON(y)
	if err != nil {
		return fmt.Errorf("failed to convert %q to JSON: %w", path, err)
	}

	if err := json.Unmarshal(j, out); err != nil {
		return fmt.Errorf("failed to unmarshal config %q: %w", path, err)
	}

	return nil
}

func writeYaml(path string, out any) error {
	log.Println("Writing YAML to", path)
	// Going directly from struct to YAML produces unexpected configs (due to missing YAML tags),
	// so we produce JSON and then convert it to YAML.
	j, err := json.Marshal(out)
	if err != nil {
		return err
	}
	y, err := gyaml.JSONToYAML(j)
	if err != nil {
		return err
	}

	return os.WriteFile(path, y, 0644)
}
