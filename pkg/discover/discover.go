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

	gyaml "github.com/ghodss/yaml"

	"github.com/openshift-knative/hack/pkg/action"
	"github.com/openshift-knative/hack/pkg/prowgen"
)

func Main() {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	inputConfig := flag.String("config", filepath.Join("config"), "Specify repositories config")
	inputAction := flag.String("input", filepath.Join(".github", "workflows", "release-generate-ci-template.yaml"), "Input action (template)")
	outputAction := flag.String("output", filepath.Join(".github", "workflows", "release-generate-ci.yaml"), "Output action")
	flag.Parse()

	err := filepath.Walk(*inputConfig, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
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

	err = action.UpdateAction(action.Config{
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

		log.Println(r.RepositoryDirectory(), "Latest branch", latest)

		for i := 0; i < len(availableBranches); i++ {
			if latestConfigured == availableBranches[i] {
				for ; i < len(availableBranches); i++ {
					if _, ok := inConfig.Config.Branches[availableBranches[i]]; !ok {
						inConfig.Config.Branches[availableBranches[i]] = inConfig.Config.Branches[latest]
					}
				}
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
