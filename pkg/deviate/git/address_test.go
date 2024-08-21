package git_test

import (
	"errors"
	"testing"

	"github.com/openshift-knative/hack/pkg/deviate/git"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestParseAddress(t *testing.T) {
	for _, iter := range parseAddressTestCases(t) {
		tc := iter
		t.Run(tc.address, func(t *testing.T) {
			got, gotErr := git.ParseAddress(tc.address)
			if tc.wantErr != nil && !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("got err: %#v, want err: %#v", gotErr, tc.wantErr)
			}
			assert.Check(t, is.DeepEqual(got, tc.want))
		})
	}
}

func parseAddressTestCases(tb testing.TB) []parseAddressTestCase {
	tb.Helper()
	return []parseAddressTestCase{{
		address: "git@github.com:cardil/kn-plugin-event-fork.git",
		want: &git.Address{
			Type: git.AddressTypeGit,
			User: "git",
			Host: "github.com",
			Path: "cardil/kn-plugin-event-fork",
			Ext:  "git",
		},
	}, {
		address: "https://ghp_ThisIsNotSecret:x-oauth-basic@github.com/cardil/" +
			"kn-plugin-event-fork.git",
		want: &git.Address{
			Type:     git.AddressTypeHTTP,
			Protocol: "https",
			User:     "ghp_ThisIsNotSecret:x-oauth-basic",
			Host:     "github.com",
			Path:     "cardil/kn-plugin-event-fork",
			Ext:      "git",
		},
	}, {
		address: ":gibberish?sdss.$",
		wantErr: git.ErrInvalidAddress,
	}}
}

type parseAddressTestCase struct {
	address string
	want    *git.Address
	wantErr error
}
