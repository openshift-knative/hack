package git

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	gitv5 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/errors"
)

func (r Repository) Checkout(remote git.Remote, branch string) git.Checkout { //nolint:ireturn
	return &onGoingCheckout{
		remote: remote,
		branch: branch,
		repo:   r,
		ctx:    r.Context,
	}
}

type onGoingCheckout struct {
	remote git.Remote
	branch string
	repo   Repository
	ctx    context.Context
}

func (o onGoingCheckout) As(branch string) error {
	repo := o.repo.Repository
	err := o.repo.Fetch(o.remote)
	if err != nil {
		return errors.Wrap(err, ErrRemoteOperationFailed)
	}
	var hash *plumbing.Hash
	hash, err = repo.ResolveRevision(o.revision())
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}
	var exist bool
	exist, err = o.branchExists(branch)
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}
	coOpts := &gitv5.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branch),
	}
	if !exist {
		coOpts.Create = true
		coOpts.Hash = *hash

		err = repo.CreateBranch(&config.Branch{
			Name:   branch,
			Remote: o.remote.Name,
			Merge:  plumbing.NewBranchReferenceName(branch),
		})
		if err != nil {
			return errors.Wrap(err, ErrLocalOperationFailed)
		}
	}
	err = wt.Checkout(coOpts)
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}

	return errors.Wrap(wt.Reset(&gitv5.ResetOptions{
		Commit: *hash,
		Mode:   gitv5.HardReset,
	}), ErrLocalOperationFailed)
}

func (o onGoingCheckout) OntoWorkspace() error {
	coOpts := &gitv5.CloneOptions{
		URL:           "file://" + o.repo.Project.Path,
		ReferenceName: plumbing.NewBranchReferenceName(o.branch),
		SingleBranch:  true,
		Depth:         1,
	}
	wt := memfs.New()
	_, err := gitv5.CloneContext(o.ctx, memory.NewStorage(), wt, coOpts)
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}
	return o.applyTree(wt, "/")
}

func (o onGoingCheckout) applyTree(fs billy.Filesystem, dir string) error {
	files, err := fs.ReadDir(dir)
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}
	for _, f := range files {
		fp := path.Join(dir, f.Name())
		if f.IsDir() {
			err = o.applyTree(fs, fp)
			if err != nil {
				return err
			}
			continue
		}
		err = o.applyFile(fs, fp, f.Mode())
		if err != nil {
			return err
		}
	}
	return nil
}

func (o onGoingCheckout) applyFile(fs billy.Filesystem, filePath string, mode fs.FileMode) error {
	fp := path.Join(o.repo.Path, filePath)
	dp := path.Dir(fp)
	const dirAllowAccessPerm = 0o755
	err := os.MkdirAll(dp, dirAllowAccessPerm)
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}
	var (
		reader io.ReadCloser
		writer io.WriteCloser
	)
	reader, err = fs.Open(filePath)
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}
	defer func() {
		_ = reader.Close()
	}()
	writer, err = createOfReplaceFile(fp, mode)
	if err != nil {
		return errors.Wrap(err, ErrLocalOperationFailed)
	}
	defer func() {
		_ = writer.Close()
	}()
	_, err = io.Copy(writer, reader)
	return errors.Wrap(err, ErrLocalOperationFailed)
}

func createOfReplaceFile(filePath string, mode fs.FileMode) (io.WriteCloser, error) {
	if _, err := os.Stat(filePath); err == nil {
		err = os.Remove(filePath)
		if err != nil {
			return nil, errors.Wrap(err, ErrLocalOperationFailed)
		}
	}
	wc, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, mode)
	return wc, errors.Wrap(err, ErrLocalOperationFailed)
}

func (o onGoingCheckout) branchExists(branch string) (bool, error) {
	repo := o.repo.Repository
	iter, err := repo.Branches()
	if err != nil {
		return false, errors.Wrap(err, ErrLocalOperationFailed)
	}
	defer iter.Close()
	var ref *plumbing.Reference
	for ref, err = iter.Next(); !errors.Is(err, io.EOF); ref, err = iter.Next() {
		name := ref.Name()
		if name.IsBranch() && name.Short() == branch {
			return true, nil
		}
	}
	return false, nil
}

func (o onGoingCheckout) revision() plumbing.Revision {
	return plumbing.Revision(fmt.Sprintf("%s/%s", o.remote.Name, o.branch))
}
