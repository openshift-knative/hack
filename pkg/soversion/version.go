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

	if upstreamVersion.Compare(*semver.New("1.21.0")) >= 0 {
		// 1.18-1.20 were skipped, so 1.21+ only subtracts the 3 skipped versions
		// 1.21 -> 1.38
		// 1.22 -> 1.39
		soVersion.Minor -= 3
	} else if upstreamVersion.Compare(*semver.New("1.18.0")) >= 0 {
		// 1.18-1.20 map to 1.37 (same as 1.17)
		// 1.17 -> 1.37
		// 1.18 -> 1.37 (subtract 1)
		// 1.19 -> 1.37 (subtract 2)
		// 1.20 -> 1.37 (subtract 3)
		soVersion.Minor -= upstreamVersion.Minor - semver.New("1.17.0").Minor
	}

	return soVersion
}

func ToUpstreamVersion(soversion string) *semver.Version {
	soversion = strings.Replace(soversion, "serverless-v", "", 1)
	soversion = strings.Replace(soversion, "serverless-", "", 1)
	soversion = strings.Replace(soversion, "v", "", 1)

	dotParts := strings.SplitN(soversion, ".", 3)
	if len(dotParts) == 2 {
		soversion = soversion + ".0"
	}
	upstreamVersion := semver.New(soversion)
	upstreamVersion.Patch = 0
	for i := 0; i < 21; i++ { // Example 1.32 -> 1.11
		upstreamVersion.Minor--
	}

	soVersion := semver.New(soversion)
	if soVersion.Compare(*semver.New("1.34.0")) >= 0 { //so >= xy
		// As 1.12 was actually 1.13
		// 1.33 -> 1.12
		// 1.34 -> 1.14
		upstreamVersion.Minor++
	}

	if soVersion.Compare(*semver.New("1.38.0")) >= 0 {
		// Add back the 3 skipped versions (1.18-1.20)
		// 1.38 -> 1.21
		// 1.39 -> 1.22
		upstreamVersion.Minor += 3
	}

	return upstreamVersion
}

func BranchName(soVersion *semver.Version) string {
	return fmt.Sprintf("release-%d.%d", soVersion.Major, soVersion.Minor)
}

func IncrementBranchName(branch string) string {
	var major, minor int
	n, err := fmt.Sscanf(branch, "release-%d.%d", &major, &minor)
	if err != nil && n != 2 {
		panic(fmt.Errorf("failed to parse branch name %q: err %v or unexpected format", branch, err))
	}
	return fmt.Sprintf("release-%d.%d", major, minor+1)
}
