package sync

import (
	"fmt"

	gitv5 "github.com/go-git/go-git/v5"
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/errors"
	"github.com/openshift-knative/hack/pkg/deviate/log/color"
)

func (o Operation) resyncReleases(excluded []release) error {
	if !o.Config.ResyncReleases.Enabled {
		return nil
	}
	releases, err := o.listReleases(true)
	if err != nil {
		return errors.Wrap(err, ErrSyncFailed)
	}
	releases = filterOutExcluded(releases, excluded)
	idx := len(releases) - o.Config.ResyncReleases.NumberOf
	if idx > 0 {
		releases = releases[idx:]
	}

	if len(releases) > 0 {
		o.Printf("Re-syncing releases: %s\n",
			color.Blue(fmt.Sprintf("%+q", releases)))
		for _, rel := range releases {
			err = o.resyncRelease(rel)
			if err != nil {
				return err
			}
		}
	} else {
		o.Println("No releases to re-sync")
	}
	return nil
}

func (o Operation) resyncRelease(rel release) error {
	rr := resyncRelease{o, rel}
	return rr.run()
}

type resyncRelease struct {
	Operation
	rel release
}

func (r resyncRelease) run() error {
	upstreamBranch, err := r.rel.Name(r.Config.ReleaseTemplates.Upstream)
	if err != nil {
		return errors.Wrap(err, ErrSyncFailed)
	}
	downstreamBranch, err := r.rel.Name(r.Config.ReleaseTemplates.Downstream)
	if err != nil {
		return errors.Wrap(err, ErrSyncFailed)
	}
	syncBranch := r.Config.Branches.SynchCI + downstreamBranch
	r.Printf("Re-syncing release: %s\n", color.Blue(r.rel.String()))
	downstreamRemote := git.Remote{
		Name: "downstream",
		URL:  r.Config.Downstream,
	}
	upstreamRemote := git.Remote{
		Name: "upstream",
		URL:  r.Config.Upstream,
	}
	return runSteps([]step{
		r.checkoutAs(downstreamRemote, downstreamBranch, syncBranch),
		r.mergeUpstream(upstreamBranch, syncBranch, []step{
			r.checkoutAs(upstreamRemote, upstreamBranch, syncBranch),
			r.pushBranch(syncBranch),
			r.createSyncReleasePR(downstreamBranch, upstreamBranch, syncBranch),
		}),
	})
}

func (r resyncRelease) checkoutAs(remote git.Remote, targetBranch, branch string) step {
	return func() error {
		err := r.Repository.Checkout(remote, targetBranch).As(branch)
		return errors.Wrap(err, ErrSyncFailed)
	}
}

func (r resyncRelease) mergeUpstream(upstreamBranch, syncBranch string, onChanges []step) step {
	upstream := git.Remote{
		Name: "upstream",
		URL:  r.Config.Upstream,
	}
	return func() error {
		err := r.Repository.Merge(&upstream, upstreamBranch)
		defer func() {
			_ = r.deleteBranch(syncBranch)
		}()
		if errors.Is(err, gitv5.NoErrAlreadyUpToDate) {
			r.Println("- no changes detected")
			return nil
		}
		r.Println("- changes detected")
		return runSteps(onChanges)
	}
}

func (r resyncRelease) createSyncReleasePR(downstreamBranch, upstreamBranch, syncBranch string) step {
	return func() error {
		title := fmt.Sprintf(
			r.Config.Messages.TriggerCI,
			downstreamBranch, upstreamBranch)
		body := fmt.Sprintf(
			r.Config.Messages.TriggerCIBody,
			downstreamBranch, upstreamBranch)
		return r.createPR(title, body, downstreamBranch, syncBranch)
	}
}

func (r resyncRelease) deleteBranch(branch string) error {
	err := r.switchToMain()
	if err != nil {
		return errors.Wrap(err, ErrSyncFailed)
	}
	err = r.Repository.DeleteBranch(branch)
	return errors.Wrap(err, ErrSyncFailed)
}

func filterOutExcluded(releases []release, excluded []release) []release {
	output := make([]release, 0, len(releases))
	for _, rel := range releases {
		found := false
		for _, exclude := range excluded {
			if rel == exclude {
				found = true
				break
			}
		}
		if !found {
			output = append(output, rel)
		}
	}
	return output
}
