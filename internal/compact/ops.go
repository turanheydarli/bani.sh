package compact

import (
	"fmt"
	"regexp"
	"strings"
)

// FilterOps are declarative output transformations applied in-process,
// replacing shell pipes like "grep -v ... | head -N". Zero values mean
// "not set". Order of application: drop/keep, group-by caps, line-length
// clamp, total line cap. Truncation always emits the overflow marker so
// the agent knows data was omitted.
type FilterOps struct {
	Drop       string      // regex: matching lines are removed
	Keep       string      // regex: only matching lines survive (applied after Drop)
	Sub        []SubRule   // per-line regex replacements, applied in order
	Tally      []TallyRule // count lines matching a regex, append a summary line
	GroupBy    string      // regex whose first capture group keys line grouping
	PerGroup   int         // max lines kept per group key
	MaxLines   int         // max total output lines
	MaxLineLen int         // max characters per line
	Overflow   string      // marker template, "{n}" = omitted count; default "+{n} more"
}

// SubRule is a per-line regex replacement (!sub "pattern" "replacement").
type SubRule struct {
	Pattern string
	Replace string
}

// TallyRule counts input lines matching Pattern (before any dropping) and
// appends Template with "{n}" substituted when the count is positive
// (!tally "pattern" "template").
type TallyRule struct {
	Pattern  string
	Template string
}

// IsZero reports whether no op is configured.
func (o FilterOps) IsZero() bool {
	return o.Drop == "" && o.Keep == "" && o.GroupBy == "" &&
		len(o.Sub) == 0 && len(o.Tally) == 0 &&
		o.PerGroup == 0 && o.MaxLines == 0 && o.MaxLineLen == 0
}

// Apply runs the configured ops over text. Invalid regexes disable their op
// rather than failing the command -- filters must never lose output.
func (o FilterOps) Apply(text string) string {
	out, _ := o.ApplyDetail(text, "", false)
	return out
}

// ApplyDetail runs the configured ops over text and accounts for every line
// they remove, one DroppedGroup per op stage labeled "<label>.<op>". When
// trace is set, each contiguous run dropped by !drop/!keep is replaced in
// place by an annotation line instead of vanishing silently (group-by and
// max-lines truncation already emit overflow markers inline).
func (o FilterOps) ApplyDetail(text, label string, trace bool) (string, []DroppedGroup) {
	if o.IsZero() || text == "" {
		return text, nil
	}
	lines := strings.Split(text, "\n")
	var groups []DroppedGroup

	tallies := o.countTallies(lines)
	lines, dropped := o.applyDropKeep(lines, label, trace)
	if dropped > 0 {
		groups = append(groups, DroppedGroup{Filter: label + ".drop", Lines: dropped})
	}
	lines = o.applySubs(lines)
	lines, dropped = o.applyGroupBy(lines)
	if dropped > 0 {
		groups = append(groups, DroppedGroup{Filter: label + ".per-group", Lines: dropped})
	}

	if o.MaxLineLen > 0 {
		for i, l := range lines {
			if len(l) > o.MaxLineLen {
				lines[i] = clampLine(l, o.MaxLineLen)
			}
		}
	}

	if o.MaxLines > 0 && len(lines) > o.MaxLines {
		omitted := len(lines) - o.MaxLines
		lines = append(lines[:o.MaxLines], o.overflowMarker(omitted))
		groups = append(groups, DroppedGroup{Filter: label + ".max-lines", Lines: omitted})
	}

	lines = append(lines, tallies...)
	return strings.Join(lines, "\n"), groups
}

// countTallies evaluates tally rules against the unfiltered input.
func (o FilterOps) countTallies(lines []string) []string {
	var out []string
	for _, t := range o.Tally {
		re, err := regexp.Compile(t.Pattern)
		if err != nil {
			continue
		}
		n := 0
		for _, l := range lines {
			if re.MatchString(l) {
				n++
			}
		}
		if n > 0 {
			out = append(out, strings.ReplaceAll(t.Template, "{n}", fmt.Sprintf("%d", n)))
		}
	}
	return out
}

// applySubs runs per-line regex replacements in rule order.
func (o FilterOps) applySubs(lines []string) []string {
	for _, s := range o.Sub {
		re, err := regexp.Compile(s.Pattern)
		if err != nil {
			continue
		}
		for i, l := range lines {
			lines[i] = re.ReplaceAllString(l, s.Replace)
		}
	}
	return lines
}

// applyDropKeep removes lines per the Drop/Keep regexes and returns the
// dropped count. In trace mode each contiguous dropped run is replaced by
// one annotation line so filter authors can see where content went.
func (o FilterOps) applyDropKeep(lines []string, label string, trace bool) ([]string, int) {
	var dropRe, keepRe *regexp.Regexp
	if o.Drop != "" {
		dropRe, _ = regexp.Compile(o.Drop)
	}
	if o.Keep != "" {
		keepRe, _ = regexp.Compile(o.Keep)
	}
	if dropRe == nil && keepRe == nil {
		return lines, 0
	}
	out := make([]string, 0, len(lines))
	dropped, run := 0, 0
	flush := func() {
		if run > 0 && trace {
			out = append(out, traceAnnotation(run, label+".drop"))
		}
		run = 0
	}
	for _, l := range lines {
		if (dropRe != nil && dropRe.MatchString(l)) || (keepRe != nil && !keepRe.MatchString(l)) {
			dropped++
			run++
			continue
		}
		flush()
		out = append(out, l)
	}
	flush()
	return out, dropped
}

// applyGroupBy keeps the first PerGroup lines per group key and returns the
// truncated line count. Lines that do not match the group regex are always
// kept. One overflow marker is emitted per truncated group, at the point of
// truncation.
func (o FilterOps) applyGroupBy(lines []string) ([]string, int) {
	if o.GroupBy == "" || o.PerGroup <= 0 {
		return lines, 0
	}
	re, err := regexp.Compile(o.GroupBy)
	if err != nil {
		return lines, 0
	}
	keyOf := func(l string) (string, bool) {
		m := re.FindStringSubmatch(l)
		if m == nil {
			return "", false
		}
		if len(m) > 1 {
			return m[1], true
		}
		return m[0], true
	}

	totals := make(map[string]int)
	for _, l := range lines {
		if key, ok := keyOf(l); ok {
			totals[key]++
		}
	}

	kept := make(map[string]int)
	out := make([]string, 0, len(lines))
	dropped := 0
	for _, l := range lines {
		key, ok := keyOf(l)
		if !ok {
			out = append(out, l)
			continue
		}
		kept[key]++
		switch {
		case kept[key] <= o.PerGroup:
			out = append(out, l)
		case kept[key] == o.PerGroup+1:
			out = append(out, o.overflowMarker(totals[key]-o.PerGroup))
			dropped++
		default:
			dropped++
		}
	}
	return out, dropped
}

func (o FilterOps) overflowMarker(n int) string {
	tpl := o.Overflow
	if tpl == "" {
		tpl = "+{n} more"
	}
	return strings.ReplaceAll(tpl, "{n}", fmt.Sprintf("%d", n))
}

// clampLine shortens a line to max characters with a truncation marker.
func clampLine(l string, max int) string {
	if max <= 3 {
		return l[:max]
	}
	return l[:max-3] + "..."
}
