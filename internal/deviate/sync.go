package deviate

import (
	"path"

	"github.com/openshift-knative/hack/pkg/deviate/cli"
	"github.com/openshift-knative/hack/pkg/deviate/config"
	"github.com/spf13/cobra"
)

type sync struct {
	*cli.Options
}

func (s sync) command() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "sync",
		Short:     "Synchronize to the upstream releases",
		ValidArgs: []string{"REPOSITORY"},
		Args:      cobra.OnlyValidArgs,
		RunE:      s.run,
	}
	return cmd
}

func (s sync) run(cmd *cobra.Command, args []string) error {
	return cli.Sync(cmd, s.project(args)) //nolint:wrapcheck
}

func (s sync) project(args []string) func() config.Project {
	return func() config.Project {
		project := config.Project{
			ConfigPath: s.ConfigPath,
			Path:       path.Dir(s.ConfigPath),
		}
		if len(args) > 0 {
			project.Path = args[0]
		}
		return project
	}
}
