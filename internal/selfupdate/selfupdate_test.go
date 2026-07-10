package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	}
	for _, c := range cases {
		if got := IsNewer(c.tag, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.tag, c.current, got, c.want)
		}
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
