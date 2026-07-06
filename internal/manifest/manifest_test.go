package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

const testBanishFile = `# BANISH -- test project

!server github
!command gh-mcp
!auth env:GITHUB_TOKEN

!verb deploy
!args env, wait?
!expand exec kubectl apply

!config
!timeout "30s"
!output json
`

func TestParseBanishFile(t *testing.T) {
	bf, err := ParseBanishFile(testBanishFile, "BANISH")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(bf.Servers) != 1 {
		t.Fatalf("servers = %d, want 1", len(bf.Servers))
	}
	if bf.Servers[0].Name != "github" {
		t.Errorf("server name = %q, want github", bf.Servers[0].Name)
	}

	if len(bf.Verbs) != 1 {
		t.Fatalf("verbs = %d, want 1", len(bf.Verbs))
	}
	if bf.Verbs[0].Name != "deploy" {
		t.Errorf("verb name = %q, want deploy", bf.Verbs[0].Name)
	}

	if bf.Config.Timeout != "30s" {
		t.Errorf("timeout = %q, want 30s", bf.Config.Timeout)
	}
	if bf.Config.Output != "json" {
		t.Errorf("output = %q, want json", bf.Config.Output)
	}
}

func TestFindBanishFile(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b", "c")
	os.MkdirAll(sub, 0755)

	// Place BANISH at root
	os.WriteFile(filepath.Join(root, "BANISH"), []byte("!config\n"), 0644)

	found := FindBanishFile(sub)
	if found == "" {
		t.Fatal("expected to find BANISH file")
	}
	if filepath.Base(filepath.Dir(found)) != filepath.Base(root) {
		t.Errorf("found in wrong dir: %s", found)
	}
}

func TestFindBanishFileStopsAtGit(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "project", "src")
	os.MkdirAll(sub, 0755)
	os.MkdirAll(filepath.Join(root, "project", ".git"), 0755)

	// BANISH above .git should not be found
	os.WriteFile(filepath.Join(root, "BANISH"), []byte("!config\n"), 0644)

	found := FindBanishFile(sub)
	if found != "" {
		t.Errorf("should not find BANISH above .git, got %s", found)
	}
}

func TestFindBanishFileMissing(t *testing.T) {
	found := FindBanishFile(t.TempDir())
	if found != "" {
		t.Errorf("expected empty for missing BANISH, got %s", found)
	}
}

func TestFindBanishFileSkipsDirectory(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "mid", "child")
	os.MkdirAll(sub, 0755)

	// A DIRECTORY named BANISH is not a manifest and must be skipped.
	os.MkdirAll(filepath.Join(root, "mid", "BANISH"), 0755)

	found := FindBanishFile(sub)
	if found != "" {
		t.Errorf("directory named BANISH should be skipped, got %s", found)
	}
}

func TestFindBanishFileWalksPastDirectory(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "mid", "child")
	os.MkdirAll(sub, 0755)

	// Directory named BANISH at mid level, real manifest file above it.
	os.MkdirAll(filepath.Join(root, "mid", "BANISH"), 0755)
	os.WriteFile(filepath.Join(root, "BANISH"), []byte("!config\n"), 0644)

	found := FindBanishFile(sub)
	if found == "" {
		t.Fatal("expected to find BANISH file above the colliding directory")
	}
	if found != filepath.Join(root, "BANISH") {
		t.Errorf("found = %s, want %s", found, filepath.Join(root, "BANISH"))
	}
}

func TestLoadBanishFileRejectsDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "BANISH")
	os.MkdirAll(dir, 0755)

	_, err := LoadBanishFile(dir)
	if err == nil {
		t.Fatal("expected error loading a directory as a manifest")
	}
}

func TestEmptyBanishFile(t *testing.T) {
	bf, err := ParseBanishFile("# empty\n", "BANISH")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(bf.Servers) != 0 || len(bf.Verbs) != 0 {
		t.Error("expected empty BANISH file to have no servers or verbs")
	}
}
