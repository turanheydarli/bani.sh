// Package history reads the Claude Code session transcripts under
// ~/.claude/projects and recovers the Bash commands the agent ran, including
// whether each command failed. It is the shared data source for banish discover
// (find frequent commands with no filter) and banish learn (find command
// mistakes the agent corrected).
package history

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Command is one Bash command the agent ran, recovered from the transcripts.
type Command struct {
	Raw       string    // the command as the agent issued it
	Base      string    // the base command name (e.g. "git"), paths and wrappers stripped
	IsError   bool      // the command's tool result was an error
	Timestamp time.Time // when the agent issued it
	Session   string    // transcript session id
}

// projectsDir returns ~/.claude/projects, or "" if home is unknown.
func projectsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

// Commands reads every transcript under ~/.claude/projects and returns the Bash
// commands the agent ran, sorted by time, with failure status resolved from the
// matching tool results.
func Commands() ([]Command, error) {
	dir := projectsDir()
	if dir == "" {
		return nil, nil
	}

	var files []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".jsonl") {
			files = append(files, p)
		}
		return nil
	})

	type rawCmd struct {
		id  string
		cmd Command
	}
	var raw []rawCmd
	errByID := make(map[string]bool)

	for _, f := range files {
		scanFile(f, func(id, command string, ts time.Time, session string) {
			raw = append(raw, rawCmd{id: id, cmd: Command{
				Raw:       command,
				Base:      BaseCommand(command),
				Timestamp: ts,
				Session:   session,
			}})
		}, func(toolUseID string, isError bool) {
			errByID[toolUseID] = isError
		})
	}

	out := make([]Command, 0, len(raw))
	for _, r := range raw {
		r.cmd.IsError = errByID[r.id]
		out = append(out, r.cmd)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	return out, nil
}

// transcriptLine is the subset of a transcript line banish reads.
type transcriptLine struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// contentBlock is one item in a message's content array.
type contentBlock struct {
	Type  string `json:"type"`
	ID    string `json:"id"`   // tool_use
	Name  string `json:"name"` // tool_use
	Input struct {
		Command string `json:"command"`
	} `json:"input"`
	ToolUseID string `json:"tool_use_id"` // tool_result
	IsError   bool   `json:"is_error"`    // tool_result
}

// scanFile reads a transcript file line by line, calling onCommand for each Bash
// tool_use and onResult for each tool_result. Malformed lines are skipped.
func scanFile(path string,
	onCommand func(id, command string, ts time.Time, session string),
	onResult func(toolUseID string, isError bool),
) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 8*1024*1024)
	for sc.Scan() {
		var line transcriptLine
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if len(line.Message.Content) == 0 {
			continue
		}
		var blocks []contentBlock
		if json.Unmarshal(line.Message.Content, &blocks) != nil {
			continue // content was a string or otherwise not a block array
		}
		for _, b := range blocks {
			switch {
			case b.Type == "tool_use" && b.Name == "Bash" && b.Input.Command != "":
				onCommand(b.ID, b.Input.Command, line.Timestamp, line.SessionID)
			case b.Type == "tool_result" && b.ToolUseID != "":
				onResult(b.ToolUseID, b.IsError)
			}
		}
	}
}

// BaseCommand returns the base command name from a raw command: it unwraps a
// banish wrapper, skips leading VAR=value assignments, and strips any path from
// the executable (so "/usr/bin/git" and "git" both return "git").
func BaseCommand(cmd string) string {
	c := unwrapBanish(strings.TrimSpace(cmd))
	fields := strings.Fields(c)
	for len(fields) > 1 && strings.Contains(fields[0], "=") && !strings.ContainsAny(fields[0], "/'\"") {
		fields = fields[1:] // skip env assignment prefix
	}
	if len(fields) == 0 {
		return ""
	}
	tok := fields[0]
	if i := strings.LastIndexByte(tok, '/'); i >= 0 {
		tok = tok[i+1:]
	}
	return tok
}

// unwrapBanish strips a banish wrapper so the underlying command is recovered.
func unwrapBanish(c string) string {
	for _, prefix := range []string{"banish run -e ", "banish "} {
		if rest, ok := strings.CutPrefix(c, prefix); ok {
			rest = strings.TrimSpace(rest)
			if len(rest) >= 2 && (rest[0] == '\'' || rest[0] == '"') && rest[len(rest)-1] == rest[0] {
				rest = rest[1 : len(rest)-1]
			}
			return strings.TrimSpace(rest)
		}
	}
	return c
}
