// Package github is the workflow provider for GitHub Actions. It is the only place in the
// workflows plugin that runs gh or knows the shape of GitHub's responses.
package github

import (
	"path"
	"strings"
	"sync"

	"github.com/ghmeier/pr-tui/plugins/workflows"
)

func init() {
	workflows.RegisterProvider("github", func() (workflows.Provider, error) { return newProvider(), nil })
}

type provider struct {
	// workflowsByRepository keeps each repository's workflows for the life of the process,
	// because their files rarely change and listing them takes two requests. Loads of
	// several repositories run at the same time, so the map is guarded.
	mutex                 sync.Mutex
	workflowsByRepository map[workflows.Repository][]workflows.Workflow
}

func newProvider() *provider {
	return &provider{workflowsByRepository: map[workflows.Repository][]workflows.Workflow{}}
}

// baseName is the last element of a workflow's path. A run of a pull request names the
// version of the file it ran with after an @, which is not part of the name.
func baseName(workflowPath string) string {
	file, _, _ := strings.Cut(workflowPath, "@")
	return path.Base(file)
}
