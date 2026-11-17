// Package engine manages downloading, verifying, extracting, tracking, and launching
// portable OpenRCT2 builds across Linux, macOS, and Windows.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/fsutil"
)

// Receipt records metadata for the currently installed user-space OpenRCT2 engine build.
type Receipt struct {
	Channel     string `json:"channel"`
	Tag         string `json:"tag"`
	SHA256      string `json:"sha256,omitempty"`
	BinaryPath  string `json:"binary_path"`
	InstalledAt string `json:"installed_at"`
}

// LoadReceipt reads the engine installation receipt from path if present.
func LoadReceipt(path string) (*Receipt, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var r Receipt
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// SaveReceipt writes r atomically to path.
func SaveReceipt(path string, r Receipt) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return err
	}
	_, _, err := fsutil.WriteFileAtomicIfChanged(path, buf.Bytes(), 0o644)
	return err
}

// GitHubRelease represents a release payload from the GitHub Releases REST API.
type GitHubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []GitHubAsset `json:"assets"`
}

// GitHubAsset represents a downloadable release asset from the GitHub Releases REST API.
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest,omitempty"`
}

// SelectAsset evaluates recipe.Assets regex patterns in order and returns the first match.
func SelectAsset(recipe catalog.PlatformRecipe, assets []GitHubAsset) (string, string, bool) {
	for _, pattern := range recipe.Assets {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			continue
		}
		for _, asset := range assets {
			name := asset.Name
			if re.MatchString(name) &&
				!strings.HasSuffix(name, ".zsync") &&
				!strings.HasSuffix(name, ".asc") &&
				!strings.HasSuffix(name, ".sig") {
				digest := strings.TrimSpace(asset.Digest)
				digest = strings.TrimPrefix(strings.ToLower(digest), "sha256:")
				return asset.BrowserDownloadURL, digest, true
			}
		}
	}
	return "", "", false
}

// Installer downloads and deploys portable OpenRCT2 builds from declarative channel recipes.
type Installer struct {
	Paths   *config.Paths
	Client  *http.Client
	Out     io.Writer
	APIBase string
	GOOS    string
	GOARCH  string
}

// NewInstaller creates a new OpenRCT2 engine installer targeting paths.
func NewInstaller(paths *config.Paths, out io.Writer) *Installer {
	if out == nil {
		out = io.Discard
	}
	return &Installer{
		Paths:   paths,
		Client:  &http.Client{Timeout: 5 * time.Minute},
		Out:     out,
		APIBase: "https://api.github.com",
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
	}
}

func (ins *Installer) goos() string {
	if ins.GOOS != "" {
		return ins.GOOS
	}
	return runtime.GOOS
}

func (ins *Installer) goarch() string {
	if ins.GOARCH != "" {
		return ins.GOARCH
	}
	return runtime.GOARCH
}

// PlatformKey formats an OS/arch pair as "<goos>/<goarch>".
func PlatformKey(goos, goarch string) string {
	return fmt.Sprintf("%s/%s", goos, goarch)
}

// ResolveAsset resolves the latest (or pinned fallback) tag, download URL, and SHA-256 for ch.
func (ins *Installer) ResolveAsset(
	ctx context.Context,
	ch catalog.ChannelConfig,
) (string, string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	platKey := PlatformKey(ins.goos(), ins.goarch())
	recipe, ok := ch.Platforms[platKey]
	if !ok || recipe.Unsupported {
		return "", "", "", fmt.Errorf("channel %q has no portable build recipe for %s", ch.ID, platKey)
	}

	var apiURL string
	if strings.HasPrefix(ch.Repo, "http://") || strings.HasPrefix(ch.Repo, "https://") {
		apiURL = ch.Repo
	} else if ch.Repo != "" {
		base := strings.TrimRight(ins.APIBase, "/")
		if base == "" {
			base = "https://api.github.com"
		}
		apiURL = fmt.Sprintf("%s/repos/%s/releases/latest", base, ch.Repo)
	}

	if apiURL != "" && len(recipe.Assets) > 0 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err == nil {
			req.Header.Set("User-Agent", fsutil.UserAgent)
			client := ins.Client
			if client == nil {
				client = &http.Client{Timeout: 30 * time.Second}
			}
			resp, doErr := client.Do(req)
			if doErr == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var release GitHubRelease
					if decErr := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&release); decErr == nil {
						if url, digest, matched := SelectAsset(recipe, release.Assets); matched {
							return release.TagName, url, digest, nil
						}
					}
				}
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return "", "", "", err
	}

	if recipe.Pinned != nil && strings.TrimSpace(recipe.Pinned.URL) != "" {
		fmt.Fprintf(
			ins.Out,
			"  Warning: live GitHub lookup unavailable for %s; falling back to pinned %s\n",
			ch.Repo,
			recipe.Pinned.Version,
		)
		return recipe.Pinned.Version, recipe.Pinned.URL, strings.TrimSpace(recipe.Pinned.SHA256), nil
	}

	return "", "", "", fmt.Errorf("no matching OpenRCT2 release asset found for %s (%s)", ch.Repo, platKey)
}

// Install downloads and installs OpenRCT2 for channel ch. When checkUpdate is true,
// upstream release metadata is compared against any local receipt and skipped if already current.
func (ins *Installer) Install(
	ctx context.Context,
	ch catalog.ChannelConfig,
	force, checkUpdate bool,
) (*Receipt, error) {
	if ins.Paths == nil {
		return nil, errors.New("nil configuration paths")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	destBin := ins.Paths.ManagedEngineBinaryForOS(ins.goos())
	payloadBin := ins.Paths.EnginePayloadForOS(ins.goos())
	existingReceipt, _ := LoadReceipt(ins.Paths.EngineReceiptFile)

	if !force && !checkUpdate {
		if isBinaryReady(destBin) {
			if existingReceipt == nil || strings.EqualFold(existingReceipt.Channel, ch.ID) {
				verLabel := ""
				if existingReceipt != nil && existingReceipt.Tag != "" {
					verLabel = " (" + existingReceipt.Tag + ")"
				}
				fmt.Fprintf(
					ins.Out,
					"✓ %s%s already installed -> %s\n",
					ch.EffectiveDisplayName(),
					verLabel,
					destBin,
				)
				return existingReceipt, nil
			}
		}
	}

	tag, url, sha256Hex, err := ins.ResolveAsset(ctx, ch)
	if err != nil {
		return nil, err
	}

	if checkUpdate && !force && existingReceipt != nil && isBinaryReady(destBin) {
		sameChannel := strings.EqualFold(existingReceipt.Channel, ch.ID)
		sameTag := existingReceipt.Tag != "" && existingReceipt.Tag == tag
		sameHash := sha256Hex == "" || strings.EqualFold(existingReceipt.SHA256, sha256Hex)
		if sameChannel && sameTag && sameHash {
			fmt.Fprintf(ins.Out, "✓ %s (%s) is already up to date\n", ch.EffectiveDisplayName(), tag)
			return existingReceipt, nil
		}
	}

	fmt.Fprintf(ins.Out, "Installing %s (%s)...\n", ch.EffectiveDisplayName(), tag)
	fmt.Fprintf(ins.Out, "  Downloading %s\n", url)

	tmpDir, err := os.MkdirTemp("", "rct-engine-dl-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tmpPayload := filepath.Join(tmpDir, "payload.bin")
	if _, err := fsutil.DownloadVerifiedContext(ctx, ins.Client, url, tmpPayload, sha256Hex, 0o755); err != nil {
		return nil, fmt.Errorf("downloading engine asset: %w", err)
	}

	lowerURL := strings.ToLower(url)
	if qIdx := strings.IndexAny(lowerURL, "?#"); qIdx >= 0 {
		lowerURL = lowerURL[:qIdx]
	}

	switch {
	case strings.HasSuffix(lowerURL, ".zip") && ins.goos() == "darwin":
		if err := ins.installMacAppZip(tmpPayload, destBin); err != nil {
			return nil, err
		}
	case strings.HasSuffix(lowerURL, ".zip") && ins.goos() == "windows":
		if err := ins.installWindowsPortableZip(tmpPayload, destBin); err != nil {
			return nil, err
		}
	default:
		if err := ins.installLinuxAppImage(tmpPayload, payloadBin, destBin); err != nil {
			return nil, err
		}
	}

	receipt := Receipt{
		Channel:     ch.ID,
		Tag:         tag,
		SHA256:      sha256Hex,
		BinaryPath:  destBin,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := SaveReceipt(ins.Paths.EngineReceiptFile, receipt); err != nil {
		return nil, fmt.Errorf("saving engine receipt: %w", err)
	}

	fmt.Fprintf(ins.Out, "✓ Installed %s (%s) -> %s\n", ch.EffectiveDisplayName(), tag, destBin)
	return &receipt, nil
}
