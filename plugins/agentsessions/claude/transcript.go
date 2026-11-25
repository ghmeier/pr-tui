package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/ghmeier/pr-tui/plugins/agentsessions"
)

// transcriptTail is how much of the end of a transcript is read. A long session's transcript can
// be many megabytes, and what it mentioned last matters most.
const transcriptTail = 2 << 20

var (
	pullRequestPattern = regexp.MustCompile(`github\.com/([A-Za-z0-9][A-Za-z0-9._-]*)/([A-Za-z0-9._-]+)/pull/(\d+)`)
	documentPattern    = regexp.MustCompile(`\.apl/([A-Za-z0-9_-]+)/([A-Za-z0-9._-]+\.md)`)
)

// transcript is what the end of a session's transcript says about the session.
type transcript struct {
	directory, branch string
	lastActive        time.Time
	pullRequests      []agentsessions.PullRequestReference
	documents         []string
}

// transcriptEntry is the part of a transcript line that the scan reads.
type transcriptEntry struct {
	Directory string `json:"cwd"`
	Branch    string `json:"gitBranch"`
	Timestamp string `json:"timestamp"`
}

// findTranscript locates the file by its session id, because the name of the directory it is in
// is derived from the session's directory by a rule that is not documented.
func findTranscript(sessionID string) (string, error) {
	home, err := homeDirectory()
	if err != nil {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", errors.New("no transcript")
	}
	return matches[0], nil
}

func scanTranscript(sessionID string) (transcript, error) {
	path, err := findTranscript(sessionID)
	if err != nil {
		return transcript{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return transcript{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return transcript{}, err
	}
	reader := bufio.NewReader(file)
	if info.Size() > transcriptTail {
		if _, err := file.Seek(info.Size()-transcriptTail, io.SeekStart); err != nil {
			return transcript{}, err
		}
		reader.Reset(file)
		// The tail starts inside a line, which cannot be read as an entry.
		if _, err := reader.ReadBytes('\n'); err != nil {
			return transcript{}, nil
		}
	}

	var scanned transcript
	var pullRequests []agentsessions.PullRequestReference
	var documents []string
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			scanned.read(line)
			pullRequests = append(pullRequests, pullRequestsIn(line)...)
			documents = append(documents, documentsIn(line)...)
		}
		if err != nil {
			break
		}
	}
	scanned.pullRequests = latestFirst(pullRequests)
	scanned.documents = latestFirst(documents)
	return scanned, nil
}

// read takes what one entry says about the session's directory, branch, and activity. Entries
// are in the order they happened, so the last one that has a directory is the newest.
func (scanned *transcript) read(line []byte) {
	var entry transcriptEntry
	if json.Unmarshal(line, &entry) != nil {
		return
	}
	if entry.Directory != "" {
		scanned.directory, scanned.branch = entry.Directory, entry.Branch
	}
	if stamp, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil && stamp.After(scanned.lastActive) {
		scanned.lastActive = stamp
	}
}

func pullRequestsIn(line []byte) []agentsessions.PullRequestReference {
	var found []agentsessions.PullRequestReference
	for _, match := range pullRequestPattern.FindAllSubmatch(line, -1) {
		if number, err := strconv.Atoi(string(match[3])); err == nil {
			found = append(found, agentsessions.PullRequestReference{Repository: string(match[1]) + "/" + string(match[2]), Number: number})
		}
	}
	return found
}

func documentsIn(line []byte) []string {
	var found []string
	for _, match := range documentPattern.FindAllSubmatch(line, -1) {
		found = append(found, string(match[1])+"/"+string(match[2]))
	}
	return found
}

// latestFirst reverses mentions that are in the order they were made and keeps the latest
// mention of each.
func latestFirst[Mention comparable](mentions []Mention) []Mention {
	seen := map[Mention]bool{}
	var latest []Mention
	for index := len(mentions) - 1; index >= 0; index-- {
		if !seen[mentions[index]] {
			seen[mentions[index]] = true
			latest = append(latest, mentions[index])
		}
	}
	return latest
}
