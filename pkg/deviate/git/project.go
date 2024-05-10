package git

import (
	"errors"
	"fmt"

	gitv5 "github.com/go-git/go-git/v5"
	"github.com/openshift-knative/hack/pkg/deviate/config"
	"github.com/openshift-knative/hack/pkg/deviate/state"
)

var (
	// ErrNotGitRepo when target isn't a git repository.
	ErrNotGitRepo = errors.New("not a git repository")

	// ErrRemoteOperationFailed when remote git repository operation failed.
	ErrRemoteOperationFailed = errors.New("remote git operation failed")

	// ErrLocalOperationFailed when local git repository operation failed.
	ErrLocalOperationFailed = errors.New("local git operation failed")
)

// NewProject creates a new Project from regular config.Project.
func NewProject(project config.Project, state state.State) (Project, error) {
	r, err := gitv5.PlainOpen(project.Path)
	if err != nil {
		return Project{}, fmt.Errorf("%s - %w: %w",
			project.Path, ErrNotGitRepo, err)
	}
	return Project{
		Project: project,
		repo:    r,
		state:   state,
	}, nil
}

// Project is a project with Git information attached.
type Project struct {
	config.Project
	state state.State
	repo  *gitv5.Repository
}

// Repository returns a Git repository implementation.
func (p Project) Repository() *Repository {
	return &Repository{
		Context:    p.state.Context,
		Project:    p.Project,
		Repository: p.repo,
	}
}
