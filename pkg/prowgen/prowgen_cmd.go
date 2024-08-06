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
