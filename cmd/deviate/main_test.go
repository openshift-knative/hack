package main_test

import (
	"bytes"
	"fmt"
	"testing"

	deviatemain "github.com/openshift-knative/hack/cmd/deviate"
	"github.com/openshift-knative/hack/internal/deviate"
	"github.com/openshift-knative/hack/pkg/deviate/metadata"
	"github.com/spf13/cobra"
	"gotest.tools/v3/assert"
)

const notSetRetCode = -1 * (2 ^ 63)

func TestMainFunc(t *testing.T) {
	o := output{}
	a := args{[]string{"--version"}}
	code := withCapturedRetCode(func() {
		withOptions(func() {
			deviatemain.Main()
		}, o.configure, a.configure)
	})

	assert.Equal(t, code, 0)
	assert.Equal(t, o.String(), fmt.Sprintf("%s version %s\n",
		metadata.Name, metadata.Version))
}

type args struct {
	of []string
}

func (a args) configure(root *cobra.Command) {
	root.SetArgs(a.of)
}

type output struct {
	*bytes.Buffer
}

func (o *output) configure(root *cobra.Command) {
	root.SetOut(o.buff())
	root.SetErr(o.buff())
}

func (o *output) buff() *bytes.Buffer {
	if o.Buffer == nil {
		o.Buffer = new(bytes.Buffer)
	}
	return o.Buffer
}

func withCapturedRetCode(fn func()) int {
	retcode := notSetRetCode
	old := deviatemain.ExitFunc
	deviatemain.ExitFunc = func(code int) {
		retcode = code
	}
	defer func() {
		deviatemain.ExitFunc = old
	}()
	fn()
	return retcode
}

func withOptions(fn func(), newOpts ...deviate.Option) {
	old := deviatemain.Opts
	deviatemain.Opts = newOpts
	defer func() {
		deviatemain.Opts = old
	}()
	fn()
}
