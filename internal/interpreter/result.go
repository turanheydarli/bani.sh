package interpreter

import (
	"encoding/json"
	"fmt"
)

// Result holds the output of a command execution.
type Result struct {
	Data any            // the result value
	Meta map[string]any // additional response metadata

	// Internal tracking -- NOT serialized to JSON output.
	RawTokens int64  // tokens before compaction
	OutTokens int64  // tokens after compaction
	SavedPct  int    // savings percentage
	Rewritten string // name of the rewrite rule applied pre-exec, if any
}

// NewResult creates a result with the given data.
func NewResult(data any) *Result {
	return &Result{Data: data}
}

// JSON returns the compact JSON representation of the result.
func (r *Result) JSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}

	out := make(map[string]any)
	out["result"] = r.Data

	for k, v := range r.Meta {
		out[k] = v
	}

	return json.Marshal(out)
}

// String returns a human-readable representation of the result.
func (r *Result) String() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%v", r.Data)
}
