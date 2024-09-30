package main

import (
	"fmt"
	"log"

	"github.com/spf13/pflag"

	"github.com/openshift-knative/hack/pkg/konfluxgen"
)

const (
	openShiftReleasePathFlag = "openshift-release-path"
	applicationNameFlag      = "application-name"
	includesFlag             = "includes"
	excludesFlag             = "excludes"
	excludeImagesFlag        = "exclude-images"
	fbcBuilderImagesFlag     = "fbc-images"
	outputFlag               = "output"
	pipelineOutputFlag       = "pipeline-output"
	workflowsPathFlag        = "workflows-path"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {

	cfg := konfluxgen.Config{}

	pflag.StringVar(&cfg.OpenShiftReleasePath, openShiftReleasePathFlag, "", "openshift/release repository path")
	pflag.StringVar(&cfg.ApplicationName, applicationNameFlag, "", "Konflux application name")
	pflag.StringVar(&cfg.ResourcesOutputPath, outputFlag, "", "output path")
	pflag.StringVar(&cfg.PipelinesOutputPath, pipelineOutputFlag, ".tekton", "output path for pipelines")
	pflag.StringVar(&cfg.WorkflowsPath, workflowsPathFlag, ".github/workflows", "output path for Github workflows")
	pflag.StringArrayVar(&cfg.Includes, includesFlag, nil, "Regex to select CI config files to include")
	pflag.StringArrayVar(&cfg.Excludes, excludesFlag, nil, "Regex to select CI config files to exclude")
	pflag.StringArrayVar(&cfg.ExcludesImages, excludeImagesFlag, nil, "Regex to select CI config images to exclude")
	pflag.StringArrayVar(&cfg.FBCImages, fbcBuilderImagesFlag, nil, "Regex to select File-Based Catalog images")
	pflag.Parse()

	if cfg.OpenShiftReleasePath == "" {
		return fmt.Errorf("expected %q flag to be non empty", openShiftReleasePathFlag)
	}
	if len(cfg.Includes) == 0 {
		return fmt.Errorf("expected %q flag to be non empty", includesFlag)
	}

	return konfluxgen.Generate(cfg)
}
