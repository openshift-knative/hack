package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/openshift-knative/hack/pkg/konfluxgen"

	"github.com/openshift-knative/hack/pkg/konfluxapply"
)

func main() {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	inputConfig := flag.String("config", filepath.Join("config"), "Specify repositories config")
	konfluxDir := flag.String("konflux-dir", filepath.Join(".konflux", konfluxgen.ApplicationsDirectoryName), "Konflux directory containing applications, components, etc")
	releasePlansDir := flag.String("release-plans-dir", filepath.Join(".konflux", "releaseplans"), "Release plans directory")
	flag.Parse()

	err := konfluxapply.Apply(ctx, konfluxapply.ApplyConfig{
		InputConfigPath: *inputConfig,
		KonfluxDir:      *konfluxDir,
	})
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to apply dir %q: %v", *konfluxDir, err))
	}

	err = konfluxapply.Apply(ctx, konfluxapply.ApplyConfig{
		InputConfigPath: *inputConfig,
		KonfluxDir:      *releasePlansDir,
	})
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to apply dir %q: %v", *releasePlansDir, err))
	}
}
