package state

import (
	"context"

	"github.com/openshift-knative/hack/pkg/deviate/config"
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/log"
)

// State represents a state of running tool.
type State struct {
	*config.Config
	*config.Project
	git.Repository
	context.Context
	log.Logger
	cancel context.CancelFunc
}
