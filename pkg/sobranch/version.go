package sobranch

import (
	"fmt"
	"github.com/coreos/go-semver/semver"
)

func FromUpstreamVersion(upstream string) string {
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

	return fmt.Sprintf("release-%d.%d", soVersion.Major, soVersion.Minor)
}
