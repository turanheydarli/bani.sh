package interpreter

import (
	"encoding/json"
	"fmt"
)

// Result holds the output of a command execution.
type Result struct {
	Data any               // the result value
	Hint *Hint             // optional _hint for shorter alternative
	Meta map[string]any    // additional response metadata
}

// Hint suggests a shorter banish alternative for a bash command.
type Hint struct {
	Shorter string `json:"shorter"`
	Saved   int    `json:"saved"`
	Why     string `json:"why,omitempty"`
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

	if r.Hint != nil {
		out["_hint"] = r.Hint
	}
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
