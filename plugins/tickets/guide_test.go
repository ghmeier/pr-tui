package tickets

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sourceLabel is the line that introduces a code block in the guide: the path of the file the
// block is quoted from, in backticks, relative to the repository root.
var sourceLabel = regexp.MustCompile("^`([^`]+\\.go)`:$")

// guideBlock is a fenced Go block of the guide together with the file it claims to quote.
type guideBlock struct {
	path string
	text string
}

func guideBlocks(t *testing.T, guide string) []guideBlock {
	t.Helper()
	var blocks []guideBlock
	var current *guideBlock
	var pendingPath string
	for _, line := range strings.Split(guide, "\n") {
		switch {
		case current != nil && line == "```":
			blocks = append(blocks, *current)
			current = nil
		case current != nil:
			current.text += line + "\n"
		case line == "```go" && pendingPath != "":
			current = &guideBlock{path: pendingPath}
			pendingPath = ""
		case sourceLabel.MatchString(line):
			pendingPath = sourceLabel.FindStringSubmatch(line)[1]
		}
	}
	return blocks
}

func TestGuideBlocksMatchTheFilesTheyQuote(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	guide, err := os.ReadFile(filepath.Join(repositoryRoot, "docs", "writing-a-plugin.md"))
	if err != nil {
		t.Fatal(err)
	}

	blocks := guideBlocks(t, string(guide))

	if len(blocks) == 0 {
		t.Fatal("the guide has no code block introduced by the path of its source file")
	}
	for _, block := range blocks {
		source, err := os.ReadFile(filepath.Join(repositoryRoot, block.path))
		if err != nil {
			t.Fatalf("the guide quotes %s: %v", block.path, err)
		}
		if !strings.Contains(string(source), block.text) {
			t.Errorf("the guide's block for %s is not in the file:\n%s", block.path, block.text)
		}
	}
}
