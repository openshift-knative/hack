package dockerfilegen

import "testing"

func TestBuilderImageForGoVersion(t *testing.T) {
	t.Parallel()
	tcs := []builderImageForGoVersionTestCase{{
		name: "empty",
		want: "registry.ci.openshift.org/openshift/release:rhel-8-release-golang--openshift-4.20",
	}, {
		name: "go1.21",
		goVersion: "1.21",
		want: "registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.21-openshift-4.16",
	}, {
		name: "go1.22",
		goVersion: "1.22",
		want: "registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.24-openshift-4.20",
	}, {
		name: "go1.23",
		goVersion: "1.23",
		want: "registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.24-openshift-4.20",
	}, {
		name: "go1.24",
		goVersion: "1.24",
		want: "registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.24-openshift-4.20",
	}, {
		name: "go1.25",
		goVersion: "1.25",
		want: "registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.25-openshift-4.20",
	}}
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, tc.run)
	}
}

type builderImageForGoVersionTestCase struct {
	name string
	goVersion string
	rhelVersion string
	want      string
}

func (tc builderImageForGoVersionTestCase) run(t *testing.T) {
	t.Parallel()
	got := builderImageForGoVersion(tc.goVersion, tc.rhelVersion)
	if got != tc.want {
		t.Errorf("builderImageForGoVersion(%q, %q) = %q; want %q", tc.goVersion, tc.rhelVersion, got, tc.want)
	}
}
