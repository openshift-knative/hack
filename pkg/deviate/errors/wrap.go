package errors

import (
	"errors"
	"fmt"
)

// Wrap an error into a wrapper. If nil passed, nil will be returned.
func Wrap(err error, wrapper error) error {
	if err != nil {
		if !errors.Is(err, wrapper) {
			return fmt.Errorf("%w: %w", wrapper, err)
		}
		return err
	}
	return nil
}

// Is reports whether any error in err's chain matches target.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// New returns an error that formats as the given text.
func New(text string) error {
	return errors.New(text) //nolint:goerr113
}
