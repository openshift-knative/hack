package prowgen

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func GitCheckout(ctx context.Context, r Repository, branch string) error {
	_, err := run(ctx, r, "git", "checkout", branch)
	return err
}

func GitClone(ctx context.Context, r Repository) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if _, err := os.Stat(r.RepositoryDirectory()); !errors.Is(err, os.ErrNotExist) {
		log.Println("Repository", r.RepositoryDirectory(), "already cloned")
		return nil
	}

	remoteRepo := fmt.Sprintf("https://github.com/%s/%s.git", r.Org, r.Repo)
	localRepo := filepath.Join(r.RepositoryDirectory(), ".git")

	if err := os.RemoveAll(r.RepositoryDirectory()); err != nil {
		return fmt.Errorf("[%s] failed to delete directory: %w", r.RepositoryDirectory(), err)
	}

	if err := os.MkdirAll(filepath.Dir(r.RepositoryDirectory()), os.ModePerm); err != nil {
		return fmt.Errorf("[%s] failed to create directory: %w", r.RepositoryDirectory(), err)
	}

	if _, err := runNoRepo(ctx, "git", "clone", "--mirror", remoteRepo, localRepo); err != nil {
		return fmt.Errorf("[%s] failed to clone repository: %w", r.RepositoryDirectory(), err)
	}

	if _, err := run(ctx, r, "git", "config", "--bool", "core.bare", "false"); err != nil {
		return fmt.Errorf("[%s] failed to set config for repository: %w", r.RepositoryDirectory(), err)
	}

	return nil
}

func GitMerge(ctx context.Context, r Repository, sha string) error {
	_, err := run(ctx, r, "git", "merge", sha, "--no-ff", "-m", "Merge " + sha)
	return err
}

func GitFetch(ctx context.Context, r Repository, sha string) error {
	remoteRepo := fmt.Sprintf("https://github.com/%s/%s.git", r.Org, r.Repo)
	_, err := runNoRepo(ctx, "git", "fetch", remoteRepo, sha)
	return err
}

func GitDiffNameOnly(ctx context.Context, r Repository, sha string) ([]string, error) {
	out, err := run(ctx, r, "git", "diff", "--name-only", sha)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}
