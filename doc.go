// Package banish is token-optimized middleware for LLM coding agents. It compacts
// shell command output before it reaches the model, cutting 30 to 90 percent of
// the tokens spent on the commands an agent runs all day.
//
// banish runs the real command - git status, npm install, cargo build, kubectl
// get - strips the noise from its output, and returns a compact version. A dirty
// git status can drop from 176 tokens to 12. Run "banish gain" at any time to see
// a running total of the tokens saved.
//
// # Install
//
//	go install go.banish.sh/banish/cmd/banish@latest
//
// Then wire banish into an agent with a single command:
//
//	banish init claude-code   // or: banish init cursor, banish init mcp
//
// # What it does
//
//   - Bash proxy: run any command through banish and get compact output back.
//   - MCP server: "banish serve" exposes every verb as a tool over stdio.
//   - .bsh scripting: a small language for defining verbs and output filters,
//     loaded at runtime without a recompile.
//   - Token accounting: "banish gain" reports cumulative savings from freq.json.
//
// banish ships with filters for git, docker, kubectl, npm, yarn, pnpm, cargo,
// maven, gradle, pytest, terraform, and aws. It only auto-approves commands your
// existing agent permission rules already allow; state-changing commands still
// prompt.
//
// The executable lives in the cmd/banish package. Full documentation is at
// https://banish.sh.
package banish
