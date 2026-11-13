// Package shell runs external commands.
package shell

import (
	"bytes"
	"os/exec"
	"strings"
)

// commandError reports a failed command by what the command printed to standard error, and
// keeps the original error so callers can still ask what kind of failure it was, such as
// errors.Is(err, exec.ErrNotFound) for a program that is not installed.
type commandError struct {
	message string
	cause   error
}

func (err commandError) Error() string { return err.message }

func (err commandError) Unwrap() error { return err.cause }

// Run runs an external command and returns its standard output. Tests replace it.
var Run = func(arguments ...string) ([]byte, error) {
	return run("", arguments)
}

// RunIn is Run with the command's working directory set, for a program that acts on the
// directory it is started in. Tests replace it.
var RunIn = func(directory string, arguments ...string) ([]byte, error) {
	return run(directory, arguments)
}

func run(directory string, arguments []string) ([]byte, error) {
	var standardError bytes.Buffer
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Dir = directory
	command.Stderr = &standardError
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(standardError.String())
		if message == "" {
			message = err.Error()
		}
		return nil, commandError{message: message, cause: err}
	}
	return output, nil
}
