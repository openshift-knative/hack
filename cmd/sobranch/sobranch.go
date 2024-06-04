package main

import (
	"flag"
	"fmt"
	"github.com/openshift-knative/hack/pkg/sobranch"
	"strings"
)

func main() {
	upstreamVersion := flag.String("upstream-version", "", "Upstream version")
	flag.Parse()

	*upstreamVersion = strings.Replace(*upstreamVersion, "release-v", "", 1)
	*upstreamVersion = strings.Replace(*upstreamVersion, "release-", "", 1)
	*upstreamVersion = strings.Replace(*upstreamVersion, "v", "", 1)

	dotParts := strings.SplitN(*upstreamVersion, ".", 3)
	if len(dotParts) == 2 {
		*upstreamVersion = *upstreamVersion + ".0"
	}

	soBranch := sobranch.FromUpstreamVersion(*upstreamVersion)

	fmt.Printf(soBranch)
}
