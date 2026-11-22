package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/fsutil"
)

const (
	defaultGitHubAPIBase   = "https://api.github.com"
	maxGitHubReleaseBytes  = 8 << 20
	defaultMetadataTimeout = 30 * time.Second
)

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

// PlatformKey formats an OS/arch pair as "<goos>/<goarch>".
func PlatformKey(goos, goarch string) string {
	return fmt.Sprintf("%s/%s", goos, goarch)
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
			if re.MatchString(name) && !isSignatureOrMetadataAsset(name) {
				digest := strings.TrimSpace(asset.Digest)
				digest, _ = strings.CutPrefix(strings.ToLower(digest), "sha256:")
				return asset.BrowserDownloadURL, digest, true
			}
		}
	}
	return "", "", false
}

func isSignatureOrMetadataAsset(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".zsync") ||
		strings.HasSuffix(lower, ".asc") ||
		strings.HasSuffix(lower, ".sig")
}

func buildLatestReleaseAPIURL(apiBase, repo string) string {
	trimmedRepo := strings.TrimSpace(repo)
	if trimmedRepo == "" {
		return ""
	}
	if strings.HasPrefix(trimmedRepo, "http://") || strings.HasPrefix(trimmedRepo, "https://") {
		return trimmedRepo
	}
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		base = defaultGitHubAPIBase
	}
	return fmt.Sprintf("%s/repos/%s/releases/latest", base, trimmedRepo)
}

func fetchLatestRelease(
	ctx context.Context,
	client *http.Client,
	apiURL string,
) (*GitHubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fsutil.UserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	safeClient := fsutil.NewSafeHTTPClient(client, defaultMetadataTimeout)
	resp, err := safeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned HTTP %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxGitHubReleaseBytes)).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding release JSON: %w", err)
	}
	return &release, nil
}
