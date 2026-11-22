// Package engine manages downloading, verifying, extracting, tracking, and launching
// portable OpenRCT2 builds across Linux, macOS, and Windows.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/fsutil"
)

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
		APIBase: defaultGitHubAPIBase,
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

	var lookupErr error
	apiURL := buildLatestReleaseAPIURL(ins.APIBase, ch.Repo)
	if apiURL != "" && len(recipe.Assets) > 0 {
		release, err := fetchLatestRelease(ctx, ins.Client, apiURL)
		if err != nil {
			lookupErr = err
		} else if url, digest, matched := SelectAsset(recipe, release.Assets); matched {
			return release.TagName, url, digest, nil
		} else {
			lookupErr = fmt.Errorf("no asset matched %s patterns in release %s", platKey, release.TagName)
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

	if lookupErr != nil {
		return "", "", "", fmt.Errorf("resolving %s (%s): %w", ch.Repo, platKey, lookupErr)
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
	existingReceipt, receiptErr := LoadReceipt(ins.Paths.EngineReceiptFile)
	if receiptErr != nil {
		fmt.Fprintf(ins.Out, "  Warning: ignoring unreadable engine receipt (%v)\n", receiptErr)
	}

	if !force && !checkUpdate && IsBinaryReady(destBin) {
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

	tag, url, sha256Hex, err := ins.ResolveAsset(ctx, ch)
	if err != nil {
		return nil, err
	}

	if checkUpdate && !force && existingReceipt != nil && IsBinaryReady(destBin) {
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

	if err := ins.deployDownloadedBundle(url, tmpPayload, payloadBin, destBin); err != nil {
		return nil, err
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

func (ins *Installer) deployDownloadedBundle(url, tmpPayload, payloadBin, destBin string) error {
	lowerURL := strings.ToLower(url)
	if qIdx := strings.IndexAny(lowerURL, "?#"); qIdx >= 0 {
		lowerURL = lowerURL[:qIdx]
	}

	switch {
	case strings.HasSuffix(lowerURL, ".zip") && ins.goos() == "darwin":
		return ins.installMacAppZip(tmpPayload, destBin)
	case strings.HasSuffix(lowerURL, ".zip") && ins.goos() == "windows":
		return ins.installWindowsPortableZip(tmpPayload, destBin)
	default:
		return ins.installLinuxAppImage(tmpPayload, payloadBin, destBin)
	}
}
