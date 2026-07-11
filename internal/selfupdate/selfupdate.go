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

// apiBase is a variable so tests can point the client at a local server.
var apiBase = "https://api.github.com"

// Channel selects which releases an update check considers. Stable is what
// everyone gets; beta additionally sees prereleases (tags like v1.2.0-beta.1
// or v1.2.0-rc.1, published as GitHub prereleases).
type Channel string

const (
	ChannelStable Channel = "stable"
	ChannelBeta   Channel = "beta"
)

// ParseChannel validates a user-supplied channel name.
func ParseChannel(s string) (Channel, error) {
	switch Channel(s) {
	case ChannelStable, ChannelBeta:
		return Channel(s), nil
	}
	return "", fmt.Errorf("unknown channel %q (want stable or beta)", s)
}

// ResolveChannel picks the channel for update checks. Precedence: the
// explicit flag value, then "channel" in ~/.banish/config.json, then
// inference from the running version - a prerelease build tracks the beta
// channel so testers keep receiving candidates and the stable release that
// eventually supersedes them. Everything else defaults to stable.
func ResolveChannel(flag, current string) Channel {
	if ch, err := ParseChannel(flag); err == nil && flag != "" {
		return ch
	}
	if ch, err := ParseChannel(configuredChannel()); err == nil {
		return ch
	}
	if v := strings.TrimPrefix(current, "v"); v != "dev" && strings.Contains(v, "-") {
		return ChannelBeta
	}
	return ChannelStable
}

// configuredChannel reads the "channel" field from ~/.banish/config.json.
// Missing file or field yields "".
func configuredChannel() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".banish", "config.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Channel string `json:"channel"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg.Channel
}

// Release is the subset of the GitHub release payload banish needs.
type Release struct {
	Tag        string
	Prerelease bool
	Assets     map[string]string // asset name -> download URL
}

// releasePayload mirrors the GitHub release JSON shape.
type releasePayload struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (p *releasePayload) release() *Release {
	rel := &Release{Tag: p.Tag, Prerelease: p.Prerelease, Assets: make(map[string]string, len(p.Assets))}
	for _, a := range p.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel
}

// Latest fetches the most recent stable release. GitHub's latest endpoint
// never returns prereleases, which is exactly the stable-channel contract.
// The context bounds the network call so callers on interactive paths can
// keep it short.
func Latest(ctx context.Context) (*Release, error) {
	var payload releasePayload
	if err := getJSON(ctx, apiBase+"/repos/"+Repo+"/releases/latest", &payload); err != nil {
		return nil, err
	}
	if payload.Tag == "" {
		return nil, fmt.Errorf("no tag in latest release")
	}
	return payload.release(), nil
}

// LatestForChannel fetches the newest release visible on the given channel.
// Stable delegates to Latest. Beta lists recent releases and picks the
// highest version among prereleases and stable alike, so a beta user is
// offered the stable release once it overtakes the candidates.
func LatestForChannel(ctx context.Context, ch Channel) (*Release, error) {
	if ch != ChannelBeta {
		return Latest(ctx)
	}
	var payload []releasePayload
	if err := getJSON(ctx, apiBase+"/repos/"+Repo+"/releases?per_page=30", &payload); err != nil {
		return nil, err
	}
	var best *Release
	for i := range payload {
		p := &payload[i]
		if p.Draft || p.Tag == "" {
			continue
		}
		if best == nil || compareSemver(strings.TrimPrefix(p.Tag, "v"), strings.TrimPrefix(best.Tag, "v")) > 0 {
			best = p.release()
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no published releases found")
	}
	return best, nil
}

func getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
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

// compareSemver returns 1 if a > b, -1 if a < b, 0 if equal, following
// semver precedence: numeric core components first, then prerelease rules -
// a version without a prerelease outranks the same core with one
// (1.2.0 > 1.2.0-beta.2 > 1.2.0-beta.1 > 1.2.0-alpha.1).
func compareSemver(a, b string) int {
	acore, apre, _ := strings.Cut(a, "-")
	bcore, bpre, _ := strings.Cut(b, "-")
	if c := compareCore(acore, bcore); c != 0 {
		return c
	}
	switch {
	case apre == "" && bpre == "":
		return 0
	case apre == "":
		return 1
	case bpre == "":
		return -1
	}
	return comparePrerelease(apre, bpre)
}

// compareCore compares dot-separated numeric components. Non-numeric or
// missing components count as zero.
func compareCore(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(as) {
			av, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bv, _ = strconv.Atoi(bs[i])
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

// comparePrerelease compares dot-separated prerelease identifiers per the
// semver spec: numeric identifiers compare numerically and rank below any
// alphanumeric identifier; otherwise ASCII order decides; a longer set of
// identifiers outranks a shared prefix (beta.1.1 > beta.1).
func comparePrerelease(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aNum := atoiStrict(as[i])
		bn, bNum := atoiStrict(bs[i])
		switch {
		case aNum && bNum:
			if an != bn {
				if an > bn {
					return 1
				}
				return -1
			}
		case aNum:
			return -1
		case bNum:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(as) > len(bs):
		return 1
	case len(as) < len(bs):
		return -1
	}
	return 0
}

// atoiStrict parses s as an integer, reporting whether it is fully numeric.
func atoiStrict(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
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
	Channel   Channel   `json:"channel,omitempty"`
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

// LatestCached returns the newest release tag known on the given channel,
// refreshing the cache at most once per CheckInterval. A cache written for a
// different channel is ignored so switching channels takes effect on the
// next check. It returns an empty string if no check has succeeded and a
// refresh is not due or fails. The refresh is bounded by timeout so it never
// blocks an interactive command for long.
func LatestCached(timeout time.Duration, ch Channel) string {
	c := readCache()
	// Legacy caches predate the channel field; treat them as stable.
	cached := c.Channel
	if cached == "" {
		cached = ChannelStable
	}
	if cached == ch && time.Since(c.LastCheck) < CheckInterval {
		return c.LatestTag
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	rel, err := LatestForChannel(ctx, ch)
	if err != nil {
		// Keep the stale tag but do not hammer the API: record the attempt.
		stale := ""
		if cached == ch {
			stale = c.LatestTag
		}
		writeCache(checkCache{LastCheck: time.Now(), LatestTag: stale, Channel: ch})
		return stale
	}
	writeCache(checkCache{LastCheck: time.Now(), LatestTag: rel.Tag, Channel: ch})
	return rel.Tag
}
