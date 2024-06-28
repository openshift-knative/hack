package github

import (
	"context"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/factory"
	ghroot "github.com/cli/cli/v2/pkg/cmd/root"
	"github.com/openshift-knative/hack/pkg/deviate/errors"
	"github.com/openshift-knative/hack/pkg/deviate/files"
	"github.com/openshift-knative/hack/pkg/deviate/metadata"
)

// ErrClientFailed when client operations has failed.
var ErrClientFailed = errors.New("client failed")

// NewClient creates new client.
func NewClient(args ...string) Client {
	return Client{Args: args}
}

// Client a client for Github CLI.
type Client struct {
	Args         []string
	DisableColor bool
	ProjectDir   string
}

// Execute a Github client CLI command.
func (c Client) Execute(ctx context.Context) ([]byte, error) {
	buildVersion := metadata.Version
	cmdFactory := factory.New(buildVersion)
	cmd, err := ghroot.NewCmdRoot(cmdFactory, buildVersion, "-")
	if err != nil {
		return nil, errors.Wrap(err, ErrClientFailed)
	}
	cmd.SetArgs(c.Args)
	tmpf, terr := os.CreateTemp("", "gh-")
	if terr != nil {
		return nil, errors.Wrap(terr, ErrClientFailed)
	}
	defer os.Remove(tmpf.Name())
	cmdFactory.IOStreams.Out = tmpf
	cmdFactory.IOStreams.ErrOut = os.Stderr
	if c.DisableColor {
		cmdFactory.IOStreams.SetColorEnabled(false)
	}
	runner := func() error {
		return errors.Wrap(cmd.ExecuteContext(ctx), ErrClientFailed)
	}
	if c.ProjectDir != "" {
		previousRunner := runner
		runner = func() error {
			return errors.Wrap(files.WithinDirectory(c.ProjectDir, previousRunner),
				ErrClientFailed)
		}
	}
	err = runner()
	bytes, ferr := os.ReadFile(tmpf.Name())
	if ferr != nil {
		return nil, errors.Wrap(ferr, ErrClientFailed)
	}
	return bytes, errors.Wrap(err, ErrClientFailed)
}
