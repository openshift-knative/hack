package main

import (
	"flag"
	"log"
	"path/filepath"

	"github.com/openshift-knative/hack/pkg/action"
)

func main() {

	inputConfig := flag.String("config", filepath.Join("config"), "Specify repositories config")
	inputAction := flag.String("input", filepath.Join(".github", "workflows", "release-generate-ci-template.yaml"), "Input action (template)")
	outputAction := flag.String("output", filepath.Join(".github", "workflows", "release-generate-ci.yaml"), "Output action")
	flag.Parse()

	err := action.UpdateAction(action.Config{
		InputAction:     *inputAction,
		InputConfigPath: *inputConfig,
		OutputAction:    *outputAction,
	})
	if err != nil {
		log.Fatal(err)
	}
}
