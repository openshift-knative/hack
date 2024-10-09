package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/openshift-knative/hack/pkg/konfluxgen"

	"github.com/openshift-knative/hack/pkg/util"
	"github.com/spf13/pflag"

	"github.com/openshift-knative/hack/pkg/konfluxapply"
)

func main() {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	var (
		inputConfig string
		konfluxDir  string
		excludes    []string
	)

	defaultExcludes := []string{
		fmt.Sprintf(".*%s.*", konfluxgen.ReleasePlanAdmissionsDirectoryName),
	}

	pflag.StringVar(&inputConfig, "config", filepath.Join("config"), "Specify repositories config")
	pflag.StringVar(&konfluxDir, "konflux-dir", ".konflux", "Konflux directory containing applications, components, etc")
	pflag.StringArrayVar(&excludes, "exclude", defaultExcludes, "Regex patterns of files or directories to exclude from apply")

	pflag.Parse()

	excludeRegex, err := util.ToRegexp(excludes)
	if err != nil {
		log.Fatal("failed to parse excludes regex: ", err)
	}

	err = konfluxapply.Apply(ctx, konfluxapply.ApplyConfig{
		InputConfigPath: inputConfig,
		KonfluxDir:      konfluxDir,
		ExcludePatterns: excludeRegex,
	})
	if err != nil {
		log.Fatal(err)
	}
}
