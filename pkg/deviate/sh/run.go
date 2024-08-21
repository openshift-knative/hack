package sh

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Run runs the given command with the given arguments.
func Run(cmd string, args ...string) error {
	_, err := doExec(nil, os.Stdout, os.Stderr, cmd, args...)
	return err
}

// doExec executes the command, piping its stdout and stderr to the given
// writers. If the command fails, it will return an error that, if returned
// from a target or mg.Deps call, will cause mage to exit with the same code as
// the command failed with. Env is a list of environment variables to set when
// running the command, these override the current environment variables set
// (which are also passed to the command). cmd and args may include references
// to environment variables in $FOO format, in which case these will be
// expanded before the command is run.
//
// Ran reports if the command ran (rather than was not found or not executable).
// Code reports the exit code the command returned if it ran. If err == nil, ran
// is always true and code is always 0.
func doExec(env map[string]string, stdout, stderr io.Writer, cmd string, args ...string) (bool, error) {
	expand := func(s string) string {
		s2, ok := env[s]
		if ok {
			return s2
		}
		return os.Getenv(s)
	}
	cmd = os.Expand(cmd, expand)
	for i := range args {
		args[i] = os.Expand(args[i], expand)
	}
	ran, code, err := run(env, stdout, stderr, cmd, args...)
	if err == nil {
		return true, nil
	}
	if ran {
		return ran, commandError{
			code: code,
			cmd:  cmd,
			args: args,
		}
	}
	return ran, fmt.Errorf(`failed to run "%s %s: %w"`,
		cmd, strings.Join(args, " "), err)
}

func run(env map[string]string, stdout, stderr io.Writer, cmd string, args ...string) (bool, int, error) {
	c := exec.Command(cmd, args...)
	c.Env = os.Environ()
	for k, v := range env {
		c.Env = append(c.Env, k+"="+v)
	}
	c.Stderr = stderr
	c.Stdout = stdout
	c.Stdin = os.Stdin

	err := c.Run()
	return cmdRan(err), exitStatus(err), err
}

// cmdRan examines the error to determine if it was generated as a result of a
// command running via os/exec.Command.  If the error is nil, or the command ran
// (even if it exited with a non-zero exit code), cmdRan reports true.  If the
// error is an unrecognized type, or it is an error from exec.Command that says
// the command failed to run (usually due to the command not existing or not
// being executable), it reports false.
func cmdRan(err error) bool {
	if err == nil {
		return true
	}
	var ee *exec.ExitError
	ok := errors.As(err, &ee)
	if ok {
		return ee.Exited()
	}
	return false
}

// exitStatus returns the exit status of the error if it is an exec.ExitError
// or if it implements exitStatus() int.
// 0 if it is nil or 1 if it is a different error.
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := err.(withExitStatus); ok {
		return e.ExitStatus()
	}
	var e *exec.ExitError
	if errors.As(err, &e) {
		if ex, ok := e.Sys().(withExitStatus); ok {
			return ex.ExitStatus()
		}
	}
	return 1
}

type withExitStatus interface {
	ExitStatus() int
}

type commandError struct {
	code int
	cmd  string
	args []string
}

func (f commandError) Error() string {
	return fmt.Sprintf(`running "%s %s" failed with exit code %d`,
		f.cmd, strings.Join(f.args, " "), f.code)
}

func (f commandError) ExitStatus() int {
	return f.code
}
