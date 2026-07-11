package bench

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files from current pipeline output")

// TestCorpus is the regression gate: every fixture must match its golden,
// meet its savings threshold, and keep its must-keep patterns. Run
// "go test ./internal/bench -update" after intentional filter changes,
// then review the golden diffs.
func TestCorpus(t *testing.T) {
	fixtures, err := LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("empty corpus")
	}
	p, err := NewPipeline()
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			r := p.Run(f)
			goldenPath := filepath.Join("corpus", f.Name+".golden")

			if *update {
				if err := os.WriteFile(goldenPath, []byte(r.Output+"\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("missing golden (run: go test ./internal/bench -update): %v", err)
			}
			if got := r.Output; got != strings.TrimRight(string(want), "\n") {
				t.Errorf("output differs from golden %s\ngot:\n%s\nwant:\n%s",
					goldenPath, got, strings.TrimRight(string(want), "\n"))
			}

			for _, violation := range Check(r) {
				t.Error(violation)
			}
		})
	}
}

// TestCorpusAggregate guards the headline claim: overall savings across the
// corpus must stay meaningful even if individual fixtures shift.
func TestCorpusAggregate(t *testing.T) {
	results, err := RunAll()
	if err != nil {
		t.Fatal(err)
	}
	rawToks, outToks, pct := Totals(results)
	if rawToks == 0 || outToks == 0 {
		t.Fatalf("degenerate totals: raw=%d out=%d", rawToks, outToks)
	}
	if pct < 50 {
		t.Errorf("aggregate savings %.0f%%, want >= 50%%", pct)
	}
}

// TestReadmeTableGroups verifies fixtures sharing a group label collapse
// into one general row, so the README stays at the tool level.
func TestReadmeTableGroups(t *testing.T) {
	results, err := RunAll()
	if err != nil {
		t.Fatal(err)
	}
	table := ReadmeTable(results, "char-based estimate")
	if n := strings.Count(table, "git (status, diff, log)"); n != 1 {
		t.Errorf("want exactly one aggregated git row, got %d:\n%s", n, table)
	}
	if strings.Contains(table, "git status (dirty)") {
		t.Errorf("per-fixture git row leaked into the grouped table:\n%s", table)
	}
	for _, label := range []string{"`make`", "`jest`", "`dotnet build`"} {
		if !strings.Contains(table, label) {
			t.Errorf("missing group row %s:\n%s", label, table)
		}
	}
}

func TestUpdateReadme(t *testing.T) {
	results, err := RunAll()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "README.md")
	content := "# x\n\n<!-- bench:begin -->\nstale\n<!-- bench:end -->\n\ntail\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	changed, err := UpdateReadme(path, results, "char-based estimate")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("first update should report a change")
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "| Command | Raw | Compacted | Savings |") {
		t.Errorf("table header missing:\n%s", got)
	}
	if !strings.HasSuffix(string(got), "\ntail\n") {
		t.Errorf("content after end marker was not preserved:\n%s", got)
	}

	changed, err = UpdateReadme(path, results, "char-based estimate")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("second update should be a no-op")
	}
}

func TestUpdateReadmeMissingMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	os.WriteFile(path, []byte("no markers here\n"), 0644)
	if _, err := UpdateReadme(path, nil, "char-based estimate"); err == nil {
		t.Error("expected error for missing markers")
	}
}
