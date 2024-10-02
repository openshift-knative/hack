package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/openshift-knative/hack/pkg/action"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	inputConfig := flag.String("config", filepath.Join("config"), "Specify repositories config")
	inputAction := flag.String("input", filepath.Join(".github", "workflows", "release-generate-ci-template.yaml"), "Input action (template)")
	outputAction := flag.String("output", filepath.Join(".github", "workflows", "release-generate-ci.yaml"), "Output action")
	flag.Parse()

	err := action.UpdateAction(ctx, action.Config{
		InputAction:     *inputAction,
		InputConfigPath: *inputConfig,
		OutputAction:    *outputAction,
	})
	if err != nil {
		log.Fatal(err)
	}
}
