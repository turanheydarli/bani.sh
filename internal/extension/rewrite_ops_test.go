package extension

import (
	"testing"
)

const rewriteExt = `!extension test v:1.0

!rewrite git-status
!match git status
!unless --porcelain -s --format
!to git status --porcelain -b

!filter go-test
!match go test
!tally "^ok\s" "== {n} ok"
!drop "^ok\s"
!drop "^PASS$"
!group-by "^([^:]+):"
!per-group 3
!max-lines 100
!max-line-len 200
!overflow "+{n} more"

!verb gs
!expand exec git status --short
!help "status"
`

func TestParseRewriteDirectives(t *testing.T) {
	l := NewLoader()
	if err := l.LoadSource("test.bsh", rewriteExt); err != nil {
		t.Fatal(err)
	}
	rws := l.Rewrites()
	if len(rws) != 1 {
		t.Fatalf("want 1 rewrite, got %d", len(rws))
	}
	rw := rws[0]
	if rw.Name != "git-status" || rw.Match != "git status" || rw.To != "git status --porcelain -b" {
		t.Errorf("rewrite = %+v", rw)
	}
	if len(rw.Unless) != 3 || rw.Unless[0] != "--porcelain" {
		t.Errorf("unless = %v", rw.Unless)
	}
}

func TestParseFilterOps(t *testing.T) {
	l := NewLoader()
	if err := l.LoadSource("test.bsh", rewriteExt); err != nil {
		t.Fatal(err)
	}
	fs := l.Filters()
	if len(fs) != 1 {
		t.Fatalf("want 1 filter, got %d", len(fs))
	}
	ops := fs[0].Ops
	if ops.Drop != `^ok\s|^PASS$` {
		t.Errorf("drop should accumulate: %q", ops.Drop)
	}
	if len(ops.Tally) != 1 || ops.Tally[0].Pattern != `^ok\s` || ops.Tally[0].Template != "== {n} ok" {
		t.Errorf("tally = %+v", ops.Tally)
	}
	if ops.GroupBy != `^([^:]+):` || ops.PerGroup != 3 {
		t.Errorf("group-by = %q per-group = %d", ops.GroupBy, ops.PerGroup)
	}
	if ops.MaxLines != 100 || ops.MaxLineLen != 200 || ops.Overflow != "+{n} more" {
		t.Errorf("caps = %+v", ops)
	}
}

func TestRewriteDoesNotLeakIntoVerb(t *testing.T) {
	l := NewLoader()
	if err := l.LoadSource("test.bsh", rewriteExt); err != nil {
		t.Fatal(err)
	}
	exts := l.Extensions()
	if len(exts) != 1 || len(exts[0].Verbs) != 1 {
		t.Fatalf("want 1 verb")
	}
	v := exts[0].Verbs[0]
	if v.Name != "gs" || v.Expand != "exec git status --short" {
		t.Errorf("verb = %+v", v)
	}
}

func TestLoadDefaults(t *testing.T) {
	l := NewLoader()
	l.LoadDefaults()
	if len(l.Rewrites()) < 2 {
		t.Errorf("defaults should define git status/log rewrites, got %d", len(l.Rewrites()))
	}
	names := map[string]bool{}
	for _, f := range l.Filters() {
		names[f.Name] = true
		if f.Match == "" {
			t.Errorf("default filter %q has empty match", f.Name)
		}
		if f.Compact == "" && f.Ops.IsZero() {
			t.Errorf("default filter %q has no action", f.Name)
		}
	}
	for _, want := range []string{"git-status", "go-test", "grep", "ls", "find"} {
		if !names[want] {
			t.Errorf("defaults missing filter %q", want)
		}
	}
}
