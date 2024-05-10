package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/log"
	"gopkg.in/yaml.v3"
)

var (
	// ErrConfigFileCantBeRead when config file cannot be read.
	ErrConfigFileCantBeRead = errors.New("config file can't be read")
	// ErrConfigFileHaveInvalidFormat when config file has invalid format.
	ErrConfigFileHaveInvalidFormat = errors.New("config file have invalid format")
)

func (c *Config) load(
	project Project,
	log log.Logger,
	informer git.RemoteURLInformer,
) error {
	bytes, err := os.ReadFile(project.ConfigPath)
	if err != nil {
		return fmt.Errorf("%s - %w: %w", project.ConfigPath,
			ErrConfigFileCantBeRead, err)
	}
	err = yaml.Unmarshal(bytes, c)
	if err != nil {
		return fmt.Errorf("%s - %w: %w", project.ConfigPath,
			ErrConfigFileHaveInvalidFormat, err)
	}

	return c.loadFromGit(log, informer)
}
