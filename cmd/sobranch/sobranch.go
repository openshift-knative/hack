package main

import (
	"flag"
	"fmt"

	"github.com/openshift-knative/hack/pkg/soversion"
)

func main() {
	upstreamVersion := flag.String("upstream-version", "", "Upstream version")
	flag.Parse()

	soVersion := soversion.FromUpstreamVersion(*upstreamVersion)
	soBranch := soversion.BranchName(soVersion)

	fmt.Printf(soBranch)
}
