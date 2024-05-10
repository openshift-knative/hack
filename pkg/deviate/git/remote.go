package git

import (
	gitv5 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/errors"
)

func (r Repository) ListRemote(remote git.Remote) ([]*plumbing.Reference, error) {
	rem := gitv5.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: remote.Name,
		URLs: []string{remote.URL},
	})

	auth, err := authentication(remote)
	if err != nil {
		return nil, err
	}
	opts := &gitv5.ListOptions{
		Auth: auth,
	}
	refs, err := rem.ListContext(r.Context, opts)
	if err != nil {
		return nil, errors.Wrap(err, ErrRemoteOperationFailed)
	}
	return refs, nil
}

func (r Repository) Remote(name string) (string, error) {
	remote, err := r.Repository.Remote(name)
	if err != nil {
		return "", errors.Wrap(err, ErrRemoteOperationFailed)
	}
	return remote.Config().URLs[0], nil
}

func (r Repository) ensureRemote(remote git.Remote) error {
	_, err := r.Repository.Remote(remote.Name)
	if errors.Is(err, gitv5.ErrRemoteNotFound) {
		_, err = r.Repository.CreateRemote(&config.RemoteConfig{
			Name: remote.Name,
			URLs: []string{remote.URL},
		})
		return errors.Wrap(err, ErrRemoteOperationFailed)
	}
	return errors.Wrap(err, ErrRemoteOperationFailed)
}
