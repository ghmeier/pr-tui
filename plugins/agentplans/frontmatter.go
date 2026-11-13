package agentplans

import "strings"

const frontMatterFence = "---"

// withoutFrontMatter drops the YAML block at the top of a document. The board already shows
// what it holds. A document that has no block, or whose block is never closed, is returned
// whole, so that text is never hidden by a stray fence.
func withoutFrontMatter(body string) string {
	lines := strings.Split(body, "\n")
	if strings.TrimRight(lines[0], " \r") != frontMatterFence {
		return body
	}
	for index := 1; index < len(lines); index++ {
		if strings.TrimRight(lines[index], " \r") == frontMatterFence {
			return strings.Join(lines[index+1:], "\n")
		}
	}
	return body
}
