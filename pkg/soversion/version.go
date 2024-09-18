package soversion

import (
	"fmt"
	"strings"

	"github.com/coreos/go-semver/semver"
)

func FromUpstreamVersion(upstream string) *semver.Version {
	upstream = strings.Replace(upstream, "release-v", "", 1)
	upstream = strings.Replace(upstream, "release-", "", 1)
	upstream = strings.Replace(upstream, "v", "", 1)

	dotParts := strings.SplitN(upstream, ".", 3)
	if len(dotParts) == 2 {
		upstream = upstream + ".0"
	}
	soVersion := semver.New(upstream)
	for i := 0; i < 21; i++ { // Example 1.11 -> 1.32
		soVersion.BumpMinor()
	}

	upstreamVersion := semver.New(upstream)
	if upstreamVersion.Compare(*semver.New("1.13.0")) >= 0 {
		// As 1.12 was actually 1.13
		// 1.12 -> 1.33
		// 1.14 -> 1.34
		soVersion.Minor -= 1
	}

	return soVersion
}

func BranchName(soVersion *semver.Version) string {
	return fmt.Sprintf("release-%d.%d", soVersion.Major, soVersion.Minor)
}
