package config

import (
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/log"
)

// New creates a new default configuration.
func New(
	project Project,
	log log.Logger,
	informer git.RemoteURLInformer,
) (Config, error) {
	c := newDefaults()
	err := c.load(project, log, informer)
	if err != nil {
		return Config{}, err
	}
	err = c.overrides()
	if err != nil {
		return Config{}, err
	}
	err = c.validate()
	if err != nil {
		return Config{}, err
	}
	return c, nil
}
