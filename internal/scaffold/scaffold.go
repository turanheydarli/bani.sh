// Package scaffold implements banish init commands that set up project manifests,
// agent hooks, and MCP server configuration.
package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InitProject creates a starter BANISH file in the given directory.
func InitProject(dir string) error {
	path := filepath.Join(dir, "BANISH")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("BANISH file already exists")
	}

	projectName := filepath.Base(dir)
	projectType := detectProjectType(dir)

	content := generateBANISH(projectName, projectType)
	return os.WriteFile(path, []byte(content), 0644)
}

// InitClaudeCode sets up banish for Claude Code: MCP config + hook + CLAUDE.md.
func InitClaudeCode(dir string) error {
	if err := writeMCPConfig(filepath.Join(dir, ".mcp.json")); err != nil {
		return err
	}
	if err := writeClaudeMD(filepath.Join(dir, "CLAUDE.md")); err != nil {
		return err
	}
	return nil
}

// InitCursor sets up banish for Cursor: MCP config + .cursorrules.
func InitCursor(dir string) error {
	cursorDir := filepath.Join(dir, ".cursor")
	os.MkdirAll(cursorDir, 0755)

	if err := writeMCPConfig(filepath.Join(cursorDir, "mcp.json")); err != nil {
		return err
	}
	if err := writeCursorRules(filepath.Join(dir, ".cursorrules")); err != nil {
		return err
	}
	return nil
}

// InitMCPOnly writes just the MCP server config.
func InitMCPOnly(dir string) error {
	return writeMCPConfig(filepath.Join(dir, ".mcp.json"))
}

func writeMCPConfig(path string) error {
	banishBin := findBanishBinary()

	if _, err := os.Stat(path); err == nil {
		// Merge into existing file
		data, _ := os.ReadFile(path)
		var existing map[string]any
		json.Unmarshal(data, &existing)
		if existing == nil {
			existing = make(map[string]any)
		}
		servers, _ := existing["mcpServers"].(map[string]any)
		if servers == nil {
			servers = make(map[string]any)
		}
		if _, ok := servers["banish"]; ok {
			return nil // already configured
		}
		servers["banish"] = map[string]any{
			"command": banishBin,
			"args":    []string{"serve"},
		}
		existing["mcpServers"] = servers
		out, _ := json.MarshalIndent(existing, "", "  ")
		return os.WriteFile(path, append(out, '\n'), 0644)
	}

	config := map[string]any{
		"mcpServers": map[string]any{
			"banish": map[string]any{
				"command": banishBin,
				"args":    []string{"serve"},
			},
		},
	}
	out, _ := json.MarshalIndent(config, "", "  ")
	return os.WriteFile(path, append(out, '\n'), 0644)
}

// findBanishBinary returns the full path to the banish binary.
func findBanishBinary() string {
	exe, err := os.Executable()
	if err == nil {
		resolved, err := filepath.EvalSymlinks(exe)
		if err == nil {
			return resolved
		}
		return exe
	}
	return "banish"
}

const claudeMDContent = `## Banish

banish is installed. All bash commands routed through banish return enriched
responses with structured JSON output and optimization hints.

banish provides:
- Structured JSON output (typed fields, not text to parse)
- Token efficiency (compact keys, pagination, no banners)
- Optimization hints (_hint fields suggesting shorter alternatives)
- MCP tools for direct access (banish_run, banish_ls, etc.)

### Response metadata

- _hint: shorter alternative exists. Try the suggested form next time.
- _suggest_extension: frequent command detected. Ask user for confirmation,
  then create .bsh extension following the embedded guide.
- _more/_total: output truncated, paginate for more.

### BANISH file

If a BANISH file exists in the project root, read it for project-specific
verbs and configuration.
`

func writeClaudeMD(path string) error {
	if _, err := os.Stat(path); err == nil {
		existing, _ := os.ReadFile(path)
		if strings.Contains(string(existing), "## Banish") {
			return nil // already has banish section
		}
		content := string(existing) + "\n" + claudeMDContent
		return os.WriteFile(path, []byte(content), 0644)
	}
	return os.WriteFile(path, []byte(claudeMDContent), 0644)
}

const cursorRulesContent = `All bash commands are routed through banish. banish returns structured JSON
output with optimization hints.

banish response metadata:
- _hint: shorter alternative exists. Try the suggested form.
- _suggest_extension: frequent command detected. Ask user, then create .bsh
  extension following the embedded guide.
- _more/_total: output truncated, paginate for more.

Read the BANISH file in the project root for project-specific verbs.
`

func writeCursorRules(path string) error {
	if _, err := os.Stat(path); err == nil {
		existing, _ := os.ReadFile(path)
		if strings.Contains(string(existing), "banish") {
			return nil
		}
		content := string(existing) + "\n" + cursorRulesContent
		return os.WriteFile(path, []byte(content), 0644)
	}
	return os.WriteFile(path, []byte(cursorRulesContent), 0644)
}

func detectProjectType(dir string) string {
	checks := map[string]string{
		"go.mod":         "go",
		"package.json":   "node",
		"Cargo.toml":     "rust",
		"pyproject.toml": "python",
		"requirements.txt": "python",
		"pom.xml":        "java",
		"build.gradle":   "java",
	}
	for file, lang := range checks {
		if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
			return lang
		}
	}
	return "generic"
}

func generateBANISH(name, projectType string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# BANISH -- %s\n\n", name)

	switch projectType {
	case "go":
		b.WriteString("!verb build\n!expand exec go build ./...\n\n")
		b.WriteString("!verb test\n!expand exec go test -race ./...\n\n")
		b.WriteString("!verb lint\n!expand exec go vet ./...\n\n")
	case "node":
		b.WriteString("!verb build\n!expand exec npm run build\n\n")
		b.WriteString("!verb test\n!expand exec npm test\n\n")
		b.WriteString("!verb lint\n!expand exec npm run lint\n\n")
	case "rust":
		b.WriteString("!verb build\n!expand exec cargo build\n\n")
		b.WriteString("!verb test\n!expand exec cargo test\n\n")
		b.WriteString("!verb lint\n!expand exec cargo clippy\n\n")
	case "python":
		b.WriteString("!verb test\n!expand exec pytest\n\n")
		b.WriteString("!verb lint\n!expand exec ruff check .\n\n")
	default:
		b.WriteString("# Add project verbs:\n# !verb build\n# !expand exec make build\n\n")
	}

	b.WriteString("!config\n!timeout \"30s\"\n!output json\n")
	return b.String()
}
