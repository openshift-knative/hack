package sync

type step func() error

func runSteps(steps []step) error {
	for _, st := range steps {
		if err := st(); err != nil {
			return err
		}
	}
	return nil
}
