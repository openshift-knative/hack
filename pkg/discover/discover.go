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

	"github.com/jinzhu/copier"

	gyaml "github.com/ghodss/yaml"

	"github.com/openshift-knative/hack/pkg/action"
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
	// Going directly from YAML raw input produces unexpected configs (due to missing YAML tags),
	// so we convert YAML to JSON and unmarshal the struct from the JSON object.
	y, err := os.ReadFile(path)
	if err != nil {
		log.Fatalln(err)
	}
	j, err := gyaml.YAMLToJSON(y)
	if err != nil {
		log.Fatalln(err)
	}

	inConfig := &prowgen.Config{}
	if err := json.Unmarshal(j, inConfig); err != nil {
		log.Fatalln("Unmarshal input config", err)
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

	// Going directly from struct to YAML produces unexpected configs (due to missing YAML tags),
	// so we produce JSON and then convert it to YAML.
	out, err := json.Marshal(inConfig)
	if err != nil {
		return err
	}
	out, err = gyaml.JSONToYAML(out)
	if err != nil {
		return err
	}

	return os.WriteFile(path, out, 0777)
}
