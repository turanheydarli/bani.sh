package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		tag, current string
		want         bool
	}{
		{"v0.5.0", "v0.4.0", true},
		{"0.5.0", "0.4.0", true},
		{"v0.4.0", "v0.4.0", false},
		{"v0.4.0", "v0.5.0", false},
		{"v0.4.1", "v0.4.0", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.10.0", "v0.9.0", true},
		{"v0.4.0", "dev", true},
		{"v0.4.0", "", true},

		// Prerelease precedence (semver): a stable release outranks its own
		// candidates, candidates order by number, and cross-channel ranking
		// is ASCII (alpha < beta < rc).
		{"v0.6.0", "v0.6.0-beta.1", true},
		{"v0.6.0-beta.1", "v0.6.0", false},
		{"v0.6.0-beta.2", "v0.6.0-beta.1", true},
		{"v0.6.0-beta.1", "v0.6.0-beta.2", false},
		{"v0.6.0-beta.10", "v0.6.0-beta.9", true},
		{"v0.6.0-rc.1", "v0.6.0-beta.3", true},
		{"v0.6.0-beta.1", "v0.6.0-alpha.2", true},
		{"v0.6.1-beta.1", "v0.6.0", true},
		{"v0.6.0-beta.1", "v0.6.0-beta.1", false},
		{"v0.6.0-beta.1.1", "v0.6.0-beta.1", true},
	}
	for _, c := range cases {
		if got := IsNewer(c.tag, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.tag, c.current, got, c.want)
		}
	}
}

func TestParseChannel(t *testing.T) {
	if ch, err := ParseChannel("beta"); err != nil || ch != ChannelBeta {
		t.Errorf("ParseChannel(beta) = %v, %v", ch, err)
	}
	if ch, err := ParseChannel("stable"); err != nil || ch != ChannelStable {
		t.Errorf("ParseChannel(stable) = %v, %v", ch, err)
	}
	if _, err := ParseChannel("nightly"); err == nil {
		t.Error("ParseChannel(nightly) succeeded, want error")
	}
}

func TestResolveChannel(t *testing.T) {
	// Isolate from any real ~/.banish/config.json.
	t.Setenv("HOME", t.TempDir())

	cases := []struct {
		flag, current string
		want          Channel
	}{
		{"beta", "0.5.0", ChannelBeta},          // explicit flag wins
		{"stable", "0.6.0-beta.1", ChannelStable}, // flag beats inference
		{"", "0.6.0-beta.1", ChannelBeta},        // prerelease build infers beta
		{"", "v0.6.0-rc.2", ChannelBeta},
		{"", "0.5.0", ChannelStable},
		{"", "dev", ChannelStable}, // local builds are not beta testers by default
		{"", "", ChannelStable},
	}
	for _, c := range cases {
		if got := ResolveChannel(c.flag, c.current); got != c.want {
			t.Errorf("ResolveChannel(%q, %q) = %v, want %v", c.flag, c.current, got, c.want)
		}
	}
}

func TestResolveChannelFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".banish"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".banish", "config.json"), []byte(`{"channel": "beta"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveChannel("", "0.5.0"); got != ChannelBeta {
		t.Errorf("ResolveChannel with config channel=beta = %v, want beta", got)
	}
	// The explicit flag still outranks the config.
	if got := ResolveChannel("stable", "0.5.0"); got != ChannelStable {
		t.Errorf("ResolveChannel(stable flag) = %v, want stable", got)
	}
}

func TestLatestForChannel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/"+Repo+"/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"tag_name": "v0.5.0", "assets": []}`)
	})
	mux.HandleFunc("/repos/"+Repo+"/releases", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[
			{"tag_name": "v0.6.0-beta.2", "prerelease": true, "assets": []},
			{"tag_name": "v0.6.0-beta.10", "prerelease": true, "assets": []},
			{"tag_name": "v0.5.0", "prerelease": false, "assets": []},
			{"tag_name": "v0.7.0-rc.1", "prerelease": true, "draft": true, "assets": []}
		]`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	rel, err := LatestForChannel(context.Background(), ChannelStable)
	if err != nil || rel.Tag != "v0.5.0" {
		t.Fatalf("stable = %+v, %v; want v0.5.0", rel, err)
	}

	// Beta picks the highest semver among published releases: beta.10 beats
	// beta.2 numerically and the draft rc is ignored.
	rel, err = LatestForChannel(context.Background(), ChannelBeta)
	if err != nil || rel.Tag != "v0.6.0-beta.10" {
		t.Fatalf("beta = %+v, %v; want v0.6.0-beta.10", rel, err)
	}
	if !rel.Prerelease {
		t.Error("beta release not marked prerelease")
	}
}

func TestAssetName(t *testing.T) {
	got := AssetName("0.4.0")
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	want := fmt.Sprintf("banish_0.4.0_%s_%s.%s", runtime.GOOS, runtime.GOARCH, ext)
	if got != want {
		t.Errorf("AssetName = %q, want %q", got, want)
	}
}

func TestVerifyChecksum(t *testing.T) {
	archive := []byte("pretend-tarball-bytes")
	sum := sha256.Sum256(archive)
	hexsum := hex.EncodeToString(sum[:])
	asset := "banish_0.4.0_linux_amd64.tar.gz"
	sums := fmt.Sprintf("deadbeef  other-file.tar.gz\n%s  %s\n", hexsum, asset)

	if err := verifyChecksum(archive, []byte(sums), asset); err != nil {
		t.Errorf("verifyChecksum valid: unexpected error %v", err)
	}
	if err := verifyChecksum([]byte("tampered"), []byte(sums), asset); err == nil {
		t.Error("verifyChecksum tampered: expected mismatch error, got nil")
	}
	if err := verifyChecksum(archive, []byte(sums), "missing.tar.gz"); err == nil {
		t.Error("verifyChecksum missing entry: expected error, got nil")
	}
	if err := verifyChecksum(archive, []byte(sums), asset); err != nil && !strings.Contains(err.Error(), "") {
		t.Fatal(err)
	}
}
