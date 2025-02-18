package main

import (
	"log"
	"os"

	"github.com/openshift-knative/hack/pkg/dockerfilegen"
	"github.com/openshift-knative/hack/pkg/util/errors"
	"github.com/spf13/pflag"
)

func main() {
	if err := GenerateMain(os.Args[1:]); err != nil {
		log.Fatalf("🔥 Error: %+v\n", errors.Rewrap(err))
	}
}

func GenerateMain(args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	params := dockerfilegen.DefaultParams(wd)
	var fset *pflag.FlagSet
	if fset, err = params.ConfigureFlags(); err != nil {
		return err
	}
	if err = fset.Parse(args); err != nil {
		return err
	}

	if err = dockerfilegen.GenerateDockerfiles(params); err != nil {
		return err
	}

	return nil
}
