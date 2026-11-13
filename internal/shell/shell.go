// Package shell runs external commands.
package shell

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// Run runs an external command and returns its standard output. Tests replace it.
var Run = func(arguments ...string) ([]byte, error) {
	var standardError bytes.Buffer
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Stderr = &standardError
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(standardError.String())
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return output, nil
}
