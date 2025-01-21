package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/openshift-knative/hack/pkg/konfluxgen"
	"github.com/spf13/pflag"

	"github.com/openshift-knative/hack/pkg/konfluxapply"
)

func main() {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	defaultAdditionalDirs := []string{
		filepath.Join(".konflux", konfluxgen.ReleasePlansDirName),
		filepath.Join(".konflux", konfluxgen.ReleasesDirName),
	}
	inputConfig := pflag.String("config", filepath.Join("config"), "Specify repositories config")
	konfluxDir := pflag.String("konflux-dir", filepath.Join(".konflux", konfluxgen.ApplicationsDirectoryName), "Konflux directory containing applications, components, etc")
	additionalDirs := pflag.StringArray("additional-dirs", defaultAdditionalDirs, "Additional directories to apply")
	pflag.Parse()

	err := konfluxapply.Apply(ctx, konfluxapply.ApplyConfig{
		InputConfigPath: *inputConfig,
		KonfluxDir:      *konfluxDir,
	})
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to apply dir %q: %v", *konfluxDir, err))
	}

	for _, dir := range *additionalDirs {
		err = konfluxapply.Apply(ctx, konfluxapply.ApplyConfig{
			InputConfigPath: *inputConfig,
			KonfluxDir:      dir,
		})
		if err != nil {
			log.Fatal(fmt.Sprintf("Failed to apply dir %q: %v", dir, err))
		}
	}
}
