package sync

import (
	gitv5 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/errors"
	"github.com/openshift-knative/hack/pkg/deviate/log/color"
	"github.com/openshift-knative/hack/pkg/deviate/state"
)

// ErrSyncFailed when the sync failed.
var ErrSyncFailed = errors.New("sync failed")

// Operation performs sync - the upstream synchronization.
type Operation struct {
	state.State
}

func (o Operation) Run() error {
	err := runSteps([]step{
		o.mirrorReleases,
		o.syncTags,
		o.syncReleaseNext,
		o.triggerCI,
		o.createSyncReleaseNextPR,
	})
	_ = o.switchToMain()
	return err
}

func (o Operation) switchToMain() error {
	downstream := git.Remote{Name: "downstream", URL: o.Config.Downstream}
	err := o.Repository.Fetch(downstream)
	if err != nil {
		return errors.Wrap(err, ErrSyncFailed)
	}
	return errors.Wrap(
		o.Repository.Checkout(downstream, o.Config.Main).As(o.Config.Main),
		ErrSyncFailed,
	)
}

func (o Operation) commitChanges(message string) step {
	return func() error {
		o.Println("- Committing changes:", message)
		commit, err := o.Repository.CommitChanges(message)
		if err != nil {
			if errors.Is(err, gitv5.NoErrAlreadyUpToDate) {
				o.Println("-- No changes to commit")
				return nil
			}
			return errors.Wrap(err, ErrSyncFailed)
		}
		stats, err := commit.StatsContext(o.Context)
		if err == nil {
			o.Printf("-- Statistics:\n%s\n", stats)
		}
		return errors.Wrap(err, ErrSyncFailed)
	}
}

func (o Operation) syncTags() error {
	refName := plumbing.NewTagReferenceName(o.Config.Tags.RefSpec)
	o.Println("- Syncing tags:", color.Blue(refName))
	return publish(o.State, "tag synchronization", refName)
}
