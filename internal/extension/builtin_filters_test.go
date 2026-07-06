package extension

import (
	"strings"
	"testing"
)

// loadBuiltinFilter parses an embedded builtin pack and returns the named
// filter, so behavior tests exercise the shipped .bsh source, not a copy.
func loadBuiltinFilter(t *testing.T, file, name string) FilterDef {
	t.Helper()
	builtins, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	src, ok := builtins[file]
	if !ok {
		t.Fatalf("no embedded builtin %q", file)
	}
	l := NewLoader()
	if err := l.LoadSource(file, src); err != nil {
		t.Fatal(err)
	}
	for _, f := range l.Filters() {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("filter %q not found in %s", name, file)
	return FilterDef{}
}

func TestBuiltinPacksRegister(t *testing.T) {
	builtins, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	l := NewLoader()
	for name, src := range builtins {
		if err := l.LoadSource(name, src); err != nil {
			t.Errorf("load %s: %v", name, err)
		}
	}
	names := map[string]bool{}
	for _, f := range l.Filters() {
		names[f.Name] = true
		if f.Match == "" {
			t.Errorf("builtin filter %q has empty match", f.Name)
		}
		if f.Compact == "" && f.Ops.IsZero() {
			t.Errorf("builtin filter %q has no action", f.Name)
		}
	}
	for _, want := range []string{
		"gh-pr-list", "gh-pr-checks",
		"make", "cmake", "ninja",
		"jest", "npx-jest", "vitest", "eslint", "tsc",
		"dotnet-build", "dotnet-test", "dotnet-restore",
	} {
		if !names[want] {
			t.Errorf("builtins missing filter %q", want)
		}
	}
}

func TestMakeFilterSuccess(t *testing.T) {
	f := loadBuiltinFilter(t, "build.bsh", "make")
	raw := "make: Entering directory '/src/app'\n" +
		"cc -O2 -Wall -c -o main.o main.c\n" +
		"cc -O2 -Wall -c -o util.o util.c\n" +
		"cc -o app main.o util.o\n" +
		"make: Leaving directory '/src/app'"
	got := f.Ops.Apply(raw)
	want := "== 3 compile commands"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestMakeFilterKeepsErrors(t *testing.T) {
	f := loadBuiltinFilter(t, "build.bsh", "make")
	raw := "make: Entering directory '/src/app'\n" +
		"cc -O2 -Wall -c -o main.o main.c\n" +
		"main.c:12:5: error: expected ';' before 'return'\n" +
		"make: *** [Makefile:8: main.o] Error 1\n" +
		"make: Leaving directory '/src/app'"
	got := f.Ops.Apply(raw)
	want := "main.c:12:5: error: expected ';' before 'return'\n" +
		"make: *** [Makefile:8: main.o] Error 1\n" +
		"== 1 compile commands"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestNinjaFilter(t *testing.T) {
	f := loadBuiltinFilter(t, "build.bsh", "ninja")
	raw := "[1/3] CXX obj/main.o\n[2/3] CXX obj/util.o\n[3/3] LINK app"
	got := f.Ops.Apply(raw)
	want := "== 3 build steps"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestJestFilterKeepsFailures(t *testing.T) {
	f := loadBuiltinFilter(t, "jstest.bsh", "jest")
	raw := "PASS src/a.test.js\n" +
		"PASS src/b.test.js\n" +
		"FAIL src/c.test.js\n" +
		"  ● works › returns value\n" +
		"\n" +
		"    expect(received).toBe(expected)\n" +
		"\n" +
		"Tests:       1 failed, 12 passed, 13 total\n" +
		"Time:        2.5 s"
	got := f.Ops.Apply(raw)
	want := "FAIL src/c.test.js\n" +
		"  ● works › returns value\n" +
		"    expect(received).toBe(expected)\n" +
		"Tests:       1 failed, 12 passed, 13 total\n" +
		"Time:        2.5 s\n" +
		"== 2 suites passed"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestGHPRChecksFilter(t *testing.T) {
	f := loadBuiltinFilter(t, "gh.bsh", "gh-pr-checks")
	raw := "build\tpass\t1m2s\thttps://github.com/o/r/actions/runs/1\n" +
		"lint\tpass\t12s\thttps://github.com/o/r/actions/runs/2\n" +
		"test\tfail\t3m\thttps://github.com/o/r/actions/runs/3"
	got := f.Ops.Apply(raw)
	want := "test\tfail\t3m\thttps://github.com/o/r/actions/runs/3\n" +
		"== 2 checks pass"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDotnetBuildFilter(t *testing.T) {
	f := loadBuiltinFilter(t, "dotnet.bsh", "dotnet-build")
	raw := "  Determining projects to restore...\n" +
		"  Restored /src/App/App.csproj (in 320 ms).\n" +
		"  App -> /src/App/bin/Debug/net8.0/App.dll\n" +
		"Build succeeded.\n" +
		"    0 Warning(s)\n" +
		"    0 Error(s)\n" +
		"Time Elapsed 00:00:03.51"
	got := f.Ops.Apply(raw)
	want := "Build succeeded.\n" +
		"    0 Warning(s)\n" +
		"    0 Error(s)\n" +
		"Time Elapsed 00:00:03.51\n" +
		"== 1 projects built"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDotnetTestFilterKeepsFailures(t *testing.T) {
	f := loadBuiltinFilter(t, "dotnet.bsh", "dotnet-test")
	raw := "  Passed TestAdd [12 ms]\n" +
		"  Passed TestSub [3 ms]\n" +
		"  Failed TestMul [5 ms]\n" +
		"  Error Message:\n" +
		"   Assert.Equal() Failure\n" +
		"Failed!  - Failed:     1, Passed:     2, Skipped:     0, Total:     3"
	got := f.Ops.Apply(raw)
	want := "  Failed TestMul [5 ms]\n" +
		"  Error Message:\n" +
		"   Assert.Equal() Failure\n" +
		"Failed!  - Failed:     1, Passed:     2, Skipped:     0, Total:     3\n" +
		"== 2 tests passed"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestESLintFilterPassthrough(t *testing.T) {
	f := loadBuiltinFilter(t, "jstest.bsh", "eslint")
	raw := "/src/app/index.js\n" +
		"  1:1  error  Unexpected var  no-var\n" +
		"  2:5  warning  Unused x  no-unused-vars\n" +
		"\n" +
		"/src/app/util.js\n" +
		"  10:1  error  Missing semicolon  semi\n" +
		"\n" +
		"✖ 3 problems (2 errors, 1 warning)"
	got := f.Ops.Apply(raw)
	if got != raw {
		t.Errorf("small report should pass through unchanged:\ngot  %q\nwant %q", got, raw)
	}
}

func TestESLintFilterCapsRepeatedRule(t *testing.T) {
	f := loadBuiltinFilter(t, "jstest.bsh", "eslint")
	raw := "/src/app/index.js\n"
	for i := 1; i <= 10; i++ {
		raw += "  1:1  warning  Unexpected console statement  no-console\n"
	}
	raw = raw[:len(raw)-1]
	got := f.Ops.Apply(raw)
	lines := len(strings.Split(got, "\n"))
	// path + 8 kept + 1 overflow marker
	if lines != 10 {
		t.Errorf("want 10 lines (8 kept + overflow), got %d:\n%s", lines, got)
	}
}
