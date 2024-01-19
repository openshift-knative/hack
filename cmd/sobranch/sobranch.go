package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/coreos/go-semver/semver"
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

	upstream := semver.New(*upstreamVersion)
	for i := 0; i < 21; i++ { // Example 1.11 -> 1.32
		upstream.BumpMinor()
	}

	fmt.Printf("release-%d.%d", upstream.Major, upstream.Minor)
}
