package files

import (
	"os"

	"github.com/openshift-knative/hack/pkg/deviate/errors"
)

// ErrCannotChangeDirectory when cannot change directory.
var ErrCannotChangeDirectory = errors.New("cannot change directory")

// WithinDirectory executes given function within directory.
func WithinDirectory(path string, fn func() error) error {
	current, err := os.Getwd()
	if err != nil {
		current = ""
	}
	err = os.Chdir(path)
	if err != nil {
		return errors.Wrap(err, ErrCannotChangeDirectory)
	}
	defer func() {
		if current != "" {
			_ = os.Chdir(current)
		}
	}()
	return fn()
}
