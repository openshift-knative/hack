package prowgen

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func runNoRepo(ctx context.Context, name string, args ...string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	cmd := exec.Command(name, args...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run %s %v: %w", name, args, err)
	}
	return nil
}

func run(ctx context.Context, r Repository, name string, args ...string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	cmd := exec.Command(name, args...)

	cmd.Dir = r.RepositoryDirectory()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("[%s] failed to run %s %v: %w", r.RepositoryDirectory(), name, args, err)
	}
	return nil
}
