package main

import (
	"flag"
	"fmt"

	"github.com/openshift-knative/hack/pkg/sobranch"
)

func main() {
	upstreamVersion := flag.String("upstream-version", "", "Upstream version")
	flag.Parse()

	soBranch := sobranch.FromUpstreamVersion(*upstreamVersion)

	fmt.Printf(soBranch)
}
