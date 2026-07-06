package lockfile

import (
	"strings"
	"testing"
)

// hunk parses a raw diff string literal (leading '+', '-', or ' ' per line)
// into the slice form parseXxx expects. Blank lines and lines without a diff
// sigil are dropped.
func hunk(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l == "" {
			continue
		}
		if l[0] != '+' && l[0] != '-' && l[0] != ' ' {
			continue
		}
		out = append(out, l)
	}
	return out
}

func TestMatch(t *testing.T) {
	cases := map[string]string{
		"package-lock.json":                       "package-lock",
		"foo/package-lock.json":                   "package-lock",
		"packages/app/npm-shrinkwrap.json":        "package-lock",
		"yarn.lock":                               "yarn",
		"apps/web/yarn.lock":                      "yarn",
		"Cargo.lock":                              "cargo",
		"go.sum":                                  "go-sum",
		"README.md":                               "",
		"package.json":                            "",
		"go.mod":                                  "",
		"internal/compact/lockfile/lockfile.go":   "",
	}
	for path, want := range cases {
		if got := Match(path); got != want {
			t.Errorf("Match(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestRenderRespectsEscapeHatch(t *testing.T) {
	t.Setenv("BANISH_LOCKFILE_FULL", "1")
	if _, ok := Render("package-lock.json", []string{`+  "version": "1.0"`}); ok {
		t.Error("BANISH_LOCKFILE_FULL=1 should disable rendering")
	}
}

func TestPackageLockV3Upgrade(t *testing.T) {
	// Real-world shape: only the version line flips; the package key stays
	// as context so both sides attribute to the same package.
	h := hunk(`
     "node_modules/express": {
-      "version": "4.19.0",
+      "version": "4.19.2",
       "resolved": "https://...",
`)
	d, ok := parsePackageLock(h)
	if !ok {
		t.Fatal("parsePackageLock ok=false")
	}
	if len(d.Upgraded) != 1 || d.Upgraded[0].Name != "express" ||
		d.Upgraded[0].From != "4.19.0" || d.Upgraded[0].To != "4.19.2" {
		t.Fatalf("Upgraded = %+v", d.Upgraded)
	}
	if len(d.Added)+len(d.Removed) != 0 {
		t.Errorf("unexpected added/removed: %+v %+v", d.Added, d.Removed)
	}
}

func TestPackageLockAddAndRemove(t *testing.T) {
	h := hunk(`
+    "node_modules/cors": {
+      "version": "2.8.5",
+      "resolved": "https://..."
+    },
-    "node_modules/lodash": {
-      "version": "4.17.21",
-      "resolved": "https://..."
-    }
`)
	d, ok := parsePackageLock(h)
	if !ok {
		t.Fatal("ok=false")
	}
	if len(d.Added) != 1 || d.Added[0].Name != "cors" || d.Added[0].Version != "2.8.5" {
		t.Fatalf("Added = %+v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0].Name != "lodash" || d.Removed[0].Version != "4.17.21" {
		t.Fatalf("Removed = %+v", d.Removed)
	}
	if len(d.Upgraded) != 0 {
		t.Errorf("unexpected upgraded: %+v", d.Upgraded)
	}
}

func TestPackageLockScopedPackage(t *testing.T) {
	// @scope/name must survive the last-segment reduction of the v2/v3 key.
	h := hunk(`
     "node_modules/@types/node": {
-      "version": "20.0.0",
+      "version": "20.11.0",
`)
	d, ok := parsePackageLock(h)
	if !ok {
		t.Fatal("ok=false")
	}
	if len(d.Upgraded) != 1 || d.Upgraded[0].Name != "@types/node" {
		t.Fatalf("Upgraded = %+v", d.Upgraded)
	}
}

func TestPackageLockIgnoresMetaKeysV1(t *testing.T) {
	// Under a v1 "dependencies": { block, "requires" and similar keys open
	// a JSON object but are not packages.
	h := hunk(`
     "express": {
-      "version": "4.19.0",
+      "version": "4.19.2",
       "requires": {
-        "body-parser": "1.20.0"
+        "body-parser": "1.20.2"
       }
     }
`)
	d, ok := parsePackageLock(h)
	if !ok {
		t.Fatal("ok=false")
	}
	// Only express's version bump should surface; the "requires" scalar
	// swap is not a version field and stays out.
	if len(d.Upgraded) != 1 || d.Upgraded[0].Name != "express" {
		t.Fatalf("Upgraded = %+v", d.Upgraded)
	}
}

func TestYarnLockUpgrade(t *testing.T) {
	h := hunk(`
-express@^4.19.0:
-  version "4.19.0"
+express@^4.19.0:
+  version "4.19.2"
   resolved "https://..."
`)
	d, ok := parseYarnLock(h)
	if !ok {
		t.Fatal("ok=false")
	}
	if len(d.Upgraded) != 1 || d.Upgraded[0].Name != "express" ||
		d.Upgraded[0].From != "4.19.0" || d.Upgraded[0].To != "4.19.2" {
		t.Fatalf("Upgraded = %+v", d.Upgraded)
	}
}

func TestYarnLockScoped(t *testing.T) {
	h := hunk(`
-"@types/node@^20.0.0":
-  version "20.0.0"
+"@types/node@^20.0.0":
+  version "20.11.0"
`)
	d, ok := parseYarnLock(h)
	if !ok {
		t.Fatal("ok=false")
	}
	if len(d.Upgraded) != 1 || d.Upgraded[0].Name != "@types/node" {
		t.Fatalf("Upgraded = %+v", d.Upgraded)
	}
}

func TestCargoLockUpgrade(t *testing.T) {
	// Version bump inside an otherwise-unchanged block: name is context.
	h := hunk(`
 [[package]]
 name = "serde"
-version = "1.0.150"
+version = "1.0.160"
 source = "registry+https://..."
`)
	d, ok := parseCargoLock(h)
	if !ok {
		t.Fatal("ok=false")
	}
	if len(d.Upgraded) != 1 || d.Upgraded[0].Name != "serde" ||
		d.Upgraded[0].From != "1.0.150" || d.Upgraded[0].To != "1.0.160" {
		t.Fatalf("Upgraded = %+v", d.Upgraded)
	}
}

func TestCargoLockAddedBlock(t *testing.T) {
	h := hunk(`
+[[package]]
+name = "tokio"
+version = "1.35.0"
+source = "registry+https://..."
`)
	d, ok := parseCargoLock(h)
	if !ok {
		t.Fatal("ok=false")
	}
	if len(d.Added) != 1 || d.Added[0].Name != "tokio" || d.Added[0].Version != "1.35.0" {
		t.Fatalf("Added = %+v", d.Added)
	}
}

func TestCargoLockHeaderResetsCurrent(t *testing.T) {
	// If we did not reset curPlus on a new [[package]] header, the version
	// under the second block would be attributed to the first block's name.
	h := hunk(`
+[[package]]
+name = "alpha"
+version = "0.1.0"
+
+[[package]]
+name = "beta"
+version = "0.2.0"
`)
	d, ok := parseCargoLock(h)
	if !ok {
		t.Fatal("ok=false")
	}
	if len(d.Added) != 2 {
		t.Fatalf("Added = %+v", d.Added)
	}
	got := map[string]string{}
	for _, nv := range d.Added {
		got[nv.Name] = nv.Version
	}
	if got["alpha"] != "0.1.0" || got["beta"] != "0.2.0" {
		t.Errorf("wrong version attribution: %+v", got)
	}
}

func TestGoSumUpgrade(t *testing.T) {
	h := hunk(`
-github.com/foo/bar v1.2.3 h1:abc=
-github.com/foo/bar v1.2.3/go.mod h1:xyz=
+github.com/foo/bar v1.2.4 h1:def=
+github.com/foo/bar v1.2.4/go.mod h1:uvw=
`)
	d, ok := parseGoSum(h)
	if !ok {
		t.Fatal("ok=false")
	}
	if len(d.Upgraded) != 1 || d.Upgraded[0].Name != "github.com/foo/bar" ||
		d.Upgraded[0].From != "v1.2.3" || d.Upgraded[0].To != "v1.2.4" {
		t.Fatalf("Upgraded = %+v", d.Upgraded)
	}
}

func TestGoSumAddAndRemove(t *testing.T) {
	h := hunk(`
+github.com/new/dep v0.1.0 h1:aaa=
+github.com/new/dep v0.1.0/go.mod h1:bbb=
-github.com/old/dep v0.9.0 h1:ccc=
-github.com/old/dep v0.9.0/go.mod h1:ddd=
`)
	d, ok := parseGoSum(h)
	if !ok {
		t.Fatal("ok=false")
	}
	if len(d.Added) != 1 || d.Added[0].Name != "github.com/new/dep" {
		t.Fatalf("Added = %+v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0].Name != "github.com/old/dep" {
		t.Fatalf("Removed = %+v", d.Removed)
	}
}

func TestFormatDeterministic(t *testing.T) {
	// Adds/removes/upgrades must sort by name so goldens do not churn on
	// map iteration order.
	d := PackageDiff{
		Added:    []NameVer{{"z-pkg", "1.0"}, {"a-pkg", "2.0"}},
		Removed:  []NameVer{{"foo", "0.1"}},
		Upgraded: []Upgrade{{"delta", "1", "2"}, {"alpha", "3", "4"}},
	}
	out := d.Format()
	if !strings.Contains(out, "a-pkg@2.0, z-pkg@1.0") {
		t.Errorf("Added not sorted:\n%s", out)
	}
	if !strings.Contains(out, "alpha 3 → 4, delta 1 → 2") {
		t.Errorf("Upgraded not sorted:\n%s", out)
	}
}

func TestFormatEmptyDiff(t *testing.T) {
	// A whitespace-only or comment-only lockfile diff parses to an empty
	// PackageDiff. The summary should still tell the agent what happened.
	out := PackageDiff{}.Format()
	if !strings.Contains(out, "no package changes detected") {
		t.Errorf("missing empty-diff notice:\n%s", out)
	}
}

func TestRenderEndToEnd(t *testing.T) {
	h := hunk(`
     "node_modules/express": {
-      "version": "4.19.0",
+      "version": "4.19.2",
`)
	out, ok := Render("app/package-lock.json", h)
	if !ok {
		t.Fatal("Render ok=false")
	}
	for _, want := range []string{"semantic diff", "express 4.19.0 → 4.19.2", "BANISH_LOCKFILE_FULL"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderRejectsUnknownPath(t *testing.T) {
	if _, ok := Render("src/main.go", []string{"+foo"}); ok {
		t.Error("non-lockfile path should return ok=false")
	}
}
