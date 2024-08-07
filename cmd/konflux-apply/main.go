package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/openshift-knative/hack/pkg/konfluxapply"
)

func main() {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	inputConfig := flag.String("config", filepath.Join("config"), "Specify repositories config")
	konfluxDir := flag.String("konflux-dir", ".konflux", "Konflux directory containing applications, components, etc")
	flag.Parse()

	err := konfluxapply.Apply(ctx, konfluxapply.ApplyConfig{
		InputConfigPath: *inputConfig,
		KonfluxDir:      *konfluxDir,
	})
	if err != nil {
		log.Fatal(err)
	}
}
