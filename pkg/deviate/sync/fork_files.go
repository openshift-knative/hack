package sync

import (
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/errors"
)

func (o Operation) addForkFiles() error {
	return runSteps([]step{
		o.removeGithubWorkflows,
		func() error {
			o.Println("- Add fork's files")
			upstream := git.Remote{Name: "upstream", URL: o.Config.Upstream}
			err := o.Repository.Checkout(upstream, o.Config.Branches.Main).
				OntoWorkspace()
			return errors.Wrap(err, ErrSyncFailed)
		},
		o.commitChanges(o.Config.Messages.ApplyForkFiles),
	})
}
