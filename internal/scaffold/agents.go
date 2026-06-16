package scaffold

import (
	"embed"
	"fmt"
)

// agentFS holds the per-agent integration assets (hook scripts, awareness and
// rules notes) that banish init deploys. Each supported agent has its own
// folder under agents/; adding an agent is a folder plus a banish hook --host
// output format.
//
//go:embed agents/claude-code/* agents/cursor/*
var agentFS embed.FS

// agentAsset returns an embedded asset for an agent, e.g.
// agentAsset("cursor", "hook.sh").
func agentAsset(agent, file string) ([]byte, error) {
	data, err := agentFS.ReadFile("agents/" + agent + "/" + file)
	if err != nil {
		return nil, fmt.Errorf("scaffold: missing asset %s/%s: %w", agent, file, err)
	}
	return data, nil
}
