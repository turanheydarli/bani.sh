// Package selfupdate handles checking for, downloading, and applying banish
// releases in place. Releases are published on GitHub as tar.gz archives named
// banish_<version>_<os>_<arch>.tar.gz alongside a checksums.txt, matching the
// layout produced by the release pipeline and the install script.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is the GitHub slug releases are published under. It intentionally does
// not match the go module path; the release assets live here.
const Repo = "turanheydarli/bani.sh"

const apiLatest = "https://api.github.com/repos/" + Repo + "/releases/latest"

// Release is the subset of the GitHub release payload banish needs.
type Release struct {
	Tag    string
	Assets map[string]string // asset name -> download URL
}

// Latest fetches the most recent published release. The context bounds the
// network call so callers on interactive paths can keep it short.
func Latest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiLatest, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned %s", resp.Status)
	}

	var payload struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Tag == "" {
		return nil, fmt.Errorf("no tag in latest release")
	}
	rel := &Release{Tag: payload.Tag, Assets: make(map[string]string, len(payload.Assets))}
	for _, a := range payload.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}

// AssetName is the archive name for the current OS and architecture at the
// given version (without a leading "v"), matching the release pipeline.
func AssetName(version string) string {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("banish_%s_%s_%s.%s", version, runtime.GOOS, runtime.GOARCH, ext)
}

// IsNewer reports whether the release tag names a version strictly newer than
// current. Both may carry a leading "v"; "dev" is always treated as older so a
// locally built binary is offered the latest release.
func IsNewer(tag, current string) bool {
	if current == "dev" || current == "" {
		return true
	}
	return compareSemver(strings.TrimPrefix(tag, "v"), strings.TrimPrefix(current, "v")) > 0
}

// compareSemver returns 1 if a > b, -1 if a < b, 0 if equal, comparing the
// dot-separated numeric components. Non-numeric or missing components count as
// zero, which is sufficient for the plain x.y.z tags banish publishes.
func compareSemver(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(as) {
			av, _ = strconv.Atoi(numericPrefix(as[i]))
		}
		if i < len(bs) {
			bv, _ = strconv.Atoi(numericPrefix(bs[i]))
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}
	return 0
}

// numericPrefix returns the leading run of digits in s (e.g. "0-rc1" -> "0").
func numericPrefix(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return s[:i]
		}
	}
	return s
}

// Apply downloads the release asset for this platform, verifies its checksum
// against the release checksums.txt, extracts the banish binary, and replaces
// the running executable in place. It returns the path that was replaced.
func Apply(ctx context.Context, rel *Release, version string) (string, error) {
	asset := AssetName(version)
	assetURL, ok := rel.Assets[asset]
	if !ok {
		return "", fmt.Errorf("release %s has no asset %s", rel.Tag, asset)
	}
	sumURL, ok := rel.Assets["checksums.txt"]
	if !ok {
		return "", fmt.Errorf("release %s has no checksums.txt", rel.Tag)
	}

	archive, err := download(ctx, assetURL)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", asset, err)
	}
	sums, err := download(ctx, sumURL)
	if err != nil {
		return "", fmt.Errorf("download checksums: %w", err)
	}
	if err := verifyChecksum(archive, sums, asset); err != nil {
		return "", err
	}

	bin, err := extractBinary(archive)
	if err != nil {
		return "", err
	}

	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if err := replaceExecutable(exe, bin); err != nil {
		return "", err
	}
	return exe, nil
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// verifyChecksum confirms the sha256 of archive matches the entry for asset in
// a checksums.txt body ("<hex>  <name>" per line).
func verifyChecksum(archive, sums []byte, asset string) error {
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == asset {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum listed for %s", asset)
	}
	sum := sha256.Sum256(archive)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", asset, got, want)
	}
	return nil
}

// extractBinary pulls the banish executable out of a tar.gz (Unix) or zip
// (Windows) archive.
func extractBinary(archive []byte) ([]byte, error) {
	if runtime.GOOS == "windows" {
		return extractFromZip(archive)
	}
	return extractFromTarGz(archive)
}

func extractFromTarGz(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == "banish" {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("banish binary not found in archive")
}

func extractFromZip(archive []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "banish.exe" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("banish.exe not found in archive")
}

// replaceExecutable writes bin over the file at path, preserving its mode. It
// stages the new binary in the same directory and swaps it into place, moving
// the current binary aside first so a running process can be replaced.
func replaceExecutable(path string, bin []byte) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(path)
	mode := os.FileMode(0o755)
	if err == nil {
		mode = info.Mode()
	}

	tmp, err := os.CreateTemp(dir, ".banish-upgrade-*")
	if err != nil {
		if os.IsPermission(err) {
			return permissionError(path)
		}
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}

	backup := path + ".old"
	if err := os.Rename(path, backup); err != nil {
		if os.IsPermission(err) {
			return permissionError(path)
		}
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		// Roll back so the user is not left without a binary.
		os.Rename(backup, path)
		return err
	}
	cleanup = false
	os.Remove(backup)
	return nil
}

func permissionError(path string) error {
	return fmt.Errorf("cannot write %s without elevated permissions - re-run with sudo, or reinstall to a user directory (BANISH_INSTALL_DIR=$HOME/.local/bin)", path)
}

// --- update-check cache ---

type checkCache struct {
	LastCheck time.Time `json:"last_check"`
	LatestTag string    `json:"latest_tag"`
}

func cachePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".banish", "update-check.json")
}

func readCache() checkCache {
	var c checkCache
	p := cachePath()
	if p == "" {
		return c
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c)
	return c
}

func writeCache(c checkCache) {
	p := cachePath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o644)
}

// CheckInterval is how long a cached update check is trusted before a refresh.
const CheckInterval = 24 * time.Hour

// LatestCached returns the newest known release tag, refreshing the cache at
// most once per CheckInterval. It returns an empty string if no check has
// succeeded and a refresh is not due or fails. The refresh is bounded by
// timeout so it never blocks an interactive command for long.
func LatestCached(timeout time.Duration) string {
	c := readCache()
	if time.Since(c.LastCheck) < CheckInterval {
		return c.LatestTag
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	rel, err := Latest(ctx)
	if err != nil {
		// Keep the stale tag but do not hammer the API: record the attempt.
		writeCache(checkCache{LastCheck: time.Now(), LatestTag: c.LatestTag})
		return c.LatestTag
	}
	writeCache(checkCache{LastCheck: time.Now(), LatestTag: rel.Tag})
	return rel.Tag
}
