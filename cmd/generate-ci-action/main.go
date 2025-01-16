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
	outputFolder := flag.String("output", filepath.Join(".github", "workflows"), "Output folder for the actions")
	flag.Parse()

	err := action.Generate(ctx, *inputConfig, *outputFolder)
	if err != nil {
		log.Fatal(err)
	}
}
