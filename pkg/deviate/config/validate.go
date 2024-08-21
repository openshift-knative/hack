package config

import (
	"fmt"

	valid "github.com/asaskevich/govalidator"
)

func (c Config) validate() error {
	ok, err := valid.ValidateStruct(c)
	if !ok {
		return fmt.Errorf("%w: %w", ErrConfigFileHaveInvalidFormat, err)
	}
	return nil
}
