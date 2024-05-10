package git

import (
	"context"

	gitv5 "github.com/go-git/go-git/v5"
	"github.com/openshift-knative/hack/pkg/deviate/config"
)

// Repository is an implementation of git repository using Golang library.
type Repository struct {
	*gitv5.Repository
	config.Project
	context.Context
}
