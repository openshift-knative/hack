package prowgen

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

func runNoRepo(ctx context.Context, name string, args ...string) ([]byte, error) {
	var buf bytes.Buffer

	select {
	case <-ctx.Done():
		return buf.Bytes(), ctx.Err()
	default:
	}

	cmd := exec.Command(name, args...)

	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run %s %v: %w", name, args, err)
	}

	return buf.Bytes(), nil
}

// runQuiet is like Run but captures stdout instead of teeing it to the
// process stdout. Use it for commands whose output is data rather than
// progress, such as `git show` or `git rev-parse`.
func runQuiet(ctx context.Context, r Repository, name string, args ...string) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var stdout, stderr bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Dir = r.RepositoryDirectory()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("[%s] failed to run %s %v: %w: %s",
			r.RepositoryDirectory(), name, args, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func Run(ctx context.Context, r Repository, name string, args ...string) ([]byte, error) {
	var buf bytes.Buffer

	select {
	case <-ctx.Done():
		return buf.Bytes(), ctx.Err()
	default:
	}

	cmd := exec.Command(name, args...)

	cmd.Dir = r.RepositoryDirectory()
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("[%s] failed to run %s %v: %w", r.RepositoryDirectory(), name, args, err)
	}
	return buf.Bytes(), nil
}
