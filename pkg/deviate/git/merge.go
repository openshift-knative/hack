package git

import (
	"fmt"

	gitv5 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/errors"
	pkgfiles "github.com/openshift-knative/hack/pkg/deviate/files"
	"github.com/openshift-knative/hack/pkg/deviate/sh"
)

func (r Repository) Merge(remote *git.Remote, branch string) error {
	var (
		err    error
		before *plumbing.Reference
		after  *plumbing.Reference
	)
	if remote != nil {
		err = r.Fetch(*remote)
		if err != nil {
			return errors.Wrap(err, ErrRemoteOperationFailed)
		}
	}
	before, err = r.Head()
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}

	targetBranch := branch
	if remote != nil {
		targetBranch = fmt.Sprintf("%s/%s", remote.Name, branch)
	}
	// TODO: Consider rewriting this to Go native code.
	err = pkgfiles.WithinDirectory(r.Project.Path, func() error {
		return errors.Wrap(sh.Run("git", "merge", "--commit",
			"--quiet", "--log", "-m", "Merge "+targetBranch, targetBranch),
			ErrRemoteOperationFailed)
	})
	if err != nil {
		_ = pkgfiles.WithinDirectory(r.Project.Path, func() error {
			return sh.Run("git", "merge", "--abort")
		})
		return errors.Wrap(err, ErrRemoteOperationFailed)
	}
	after, err = r.Head()
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}

	if before.Hash() == after.Hash() {
		err = gitv5.NoErrAlreadyUpToDate
	}
	return err
}
