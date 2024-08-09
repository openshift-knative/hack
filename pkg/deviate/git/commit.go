package git

import (
	gitv5 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/openshift-knative/hack/pkg/deviate/errors"
)

func (r Repository) CommitChanges(message string) (*object.Commit, error) {
	wt, err := r.Repository.Worktree()
	if err != nil {
		return nil, errors.Wrap(err, ErrLocalOperationFailed)
	}
	var st gitv5.Status
	st, err = wt.Status()
	if err != nil {
		return nil, errors.Wrap(err, ErrLocalOperationFailed)
	}
	if st.IsClean() {
		return nil, gitv5.NoErrAlreadyUpToDate
	}
	err = wt.AddWithOptions(&gitv5.AddOptions{
		All:  true,
		Path: ".",
	})
	if err != nil {
		return nil, errors.Wrap(err, ErrLocalOperationFailed)
	}
	var hash plumbing.Hash
	hash, err = wt.Commit(message, &gitv5.CommitOptions{
		All: true,
	})
	if err != nil {
		return nil, errors.Wrap(err, ErrLocalOperationFailed)
	}
	var commit *object.Commit
	commit, err = r.CommitObject(hash)
	return commit, errors.Wrap(err, ErrLocalOperationFailed)
}
