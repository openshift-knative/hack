package soversion

import (
	"fmt"
	"strings"

	"github.com/coreos/go-semver/semver"
	"github.com/pkg/errors"
)

func FromUpstreamVersion(upstream string) *semver.Version {
	ver, err := SoFromUpstreamVersion(upstream)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	return ver
}

func SoFromUpstreamVersion(upstream string) (*semver.Version, error) {
	upstream = strings.Replace(upstream, "release-v", "", 1)
	upstream = strings.Replace(upstream, "release-", "", 1)
	upstream = strings.Replace(upstream, "v", "", 1)

	dotParts := strings.SplitN(upstream, ".", 3)
	if len(dotParts) == 2 {
		upstream = upstream + ".0"
	}
	soVersion, err := semver.NewVersion(upstream)
	if err != nil {
		return nil, errors.WithStack(err)
	}
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

	return soVersion, nil
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
