package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/coreos/go-semver/semver"
	"github.com/spf13/pflag"

	"github.com/openshift-knative/hack/pkg/rhel"
)

func main() {
	var soVersion string

	pflag.StringVar(&soVersion, "so-version", "", "Serverless Operator Version (e.g. 1.35)")
	pflag.Parse()

	dotParts := strings.SplitN(soVersion, ".", 3)
	if len(dotParts) == 2 {
		soVersion = soVersion + ".0"
	}

	soSemVer, err := semver.NewVersion(soVersion)
	if err != nil {
		log.Fatal(err)
	}

	v, err := rhel.ForSOVersion(soSemVer)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(v)
}
