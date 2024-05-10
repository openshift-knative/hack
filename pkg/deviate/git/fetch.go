package git

import (
	gitv5 "github.com/go-git/go-git/v5"
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/errors"
)

func (r Repository) Fetch(remote git.Remote) error {
	if err := r.ensureRemote(remote); err != nil {
		return err
	}
	auth, err := authentication(remote)
	if err != nil {
		return err
	}
	if err = r.Repository.FetchContext(r.Context, &gitv5.FetchOptions{
		RemoteName: remote.Name,
		Auth:       auth,
	}); !errors.Is(err, gitv5.NoErrAlreadyUpToDate) {
		return errors.Wrap(err, ErrRemoteOperationFailed)
	}

	return nil
}
