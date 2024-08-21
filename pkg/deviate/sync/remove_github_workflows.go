package sync

import (
	"os"
	"path"
	"path/filepath"

	"github.com/openshift-knative/hack/pkg/deviate/errors"
)

func (o Operation) removeGithubWorkflows() error {
	o.Println("- Remove upstream Github workflows")
	workflows := path.Join(o.State.Project.Path, ".github", "workflows")

	dir, err := os.ReadDir(workflows)
	if err != nil {
		return errors.Wrap(err, ErrSyncFailed)
	}
	for _, d := range dir {
		fp := path.Join(workflows, d.Name())
		if ok, _ := filepath.Match(o.GithubWorkflowsRemovalGlob, path.Base(fp)); ok {
			err = os.RemoveAll(fp)
			if err != nil {
				return errors.Wrap(err, ErrSyncFailed)
			}
		}
	}
	return nil
}
