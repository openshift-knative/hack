package git

import (
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// RemoteLister will list references of a GIT repository remote.
type RemoteLister interface {
	ListRemote(remote Remote) ([]*plumbing.Reference, error)
}

// RemoteURLInformer will return a URL of a remote or error if such remote
// do not exist.
type RemoteURLInformer interface {
	Remote(name string) (string, error)
}

// Repository contains operations on underlying GIT repo.
type Repository interface {
	RemoteLister
	RemoteURLInformer
	Fetch(remote Remote) error
	Checkout(remote Remote, branch string) Checkout
	Push(remote Remote, refname plumbing.ReferenceName) error
	DeleteBranch(branch string) error
	CommitChanges(message string) (*object.Commit, error)
	Merge(remote *Remote, branch string) error
}
