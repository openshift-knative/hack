package sync

func (o Operation) syncReleaseNext() error {
	return runSteps([]step{
		o.resetReleaseNext,
		o.addForkFiles,
		o.applyPatches,
		o.pushBranch(o.Config.Branches.ReleaseNext),
	})
}

func (o Operation) pushBranch(branch string) step {
	return func() error {
		p := push{
			State:  o.State,
			branch: branch,
		}
		return runSteps(p.steps())
	}
}
