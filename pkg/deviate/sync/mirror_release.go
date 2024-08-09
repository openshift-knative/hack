package sync

import (
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/errors"
	"github.com/openshift-knative/hack/pkg/deviate/log/color"
	"github.com/openshift-knative/hack/pkg/deviate/state"
)

func (o Operation) mirrorRelease(rel release) error {
	return runSteps([]step{
		o.createNewRelease(rel),
		o.addForkFiles,
		o.applyPatches,
		o.switchToMain,
		o.pushRelease(rel),
	})
}

func (o Operation) createNewRelease(rel release) step {
	o.Printf("- Creating new release: %s\n", color.Blue(rel.String()))
	upstream := git.Remote{Name: "upstream", URL: o.Config.Upstream}
	cnr := createNewRelease{State: o.State, rel: rel, remote: upstream}
	return cnr.step
}

func (o Operation) pushRelease(rel release) step {
	return func() error {
		o.Printf("- Publishing release: %s\n", color.Blue(rel.String()))
		branch, err := rel.Name(o.Config.ReleaseTemplates.Downstream)
		if err != nil {
			return err
		}
		pr := push{State: o.State, branch: branch}
		return runSteps(pr.steps())
	}
}

type createNewRelease struct {
	state.State
	rel    release
	remote git.Remote
}

func (r createNewRelease) step() error {
	upstreamBranch, err := r.rel.Name(r.Config.ReleaseTemplates.Upstream)
	if err != nil {
		return err
	}
	downstreamBranch, err := r.rel.Name(r.Config.ReleaseTemplates.Downstream)
	if err != nil {
		return err
	}
	return runSteps([]step{
		r.fetch,
		r.checkoutAsNewRelease(upstreamBranch, downstreamBranch),
	})
}

func (r createNewRelease) fetch() error {
	return errors.Wrap(r.Repository.Fetch(r.remote), ErrSyncFailed)
}

func (r createNewRelease) checkoutAsNewRelease(upstreamBranch, downstreamBranch string) step {
	return func() error {
		return errors.Wrap(
			r.Repository.Checkout(r.remote, upstreamBranch).As(downstreamBranch),
			ErrSyncFailed)
	}
}

type push struct {
	state.State
	branch string
}

func (p push) steps() []step {
	return []step{
		p.push,
		p.delete,
	}
}

func (p push) push() error {
	refName := plumbing.NewBranchReferenceName(p.branch)
	return publish(p.State, "release push", refName)
}

func (p push) delete() error {
	return errors.Wrap(p.Repository.DeleteBranch(p.branch), ErrSyncFailed)
}

func publish(state state.State, title string, refName plumbing.ReferenceName) error {
	if state.Config.DryRun {
		state.Logger.Println(color.Yellow(fmt.Sprintf(
			"- Skipping %s, because of dry run", title)))
		return nil
	}
	remote := git.Remote{
		Name: "downstream",
		URL:  state.Config.Downstream,
	}
	return errors.Wrap(state.Repository.Push(remote, refName), ErrSyncFailed)
}
