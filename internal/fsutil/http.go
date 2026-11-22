package fsutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// NewSafeHTTPClient returns a shallow copy of client (or a new client with defaultTimeout
// when client is nil) enforcing a 10-redirect cap and rejecting HTTPS-to-HTTP downgrades.
func NewSafeHTTPClient(client *http.Client, defaultTimeout time.Duration) *http.Client {
	var safeClient http.Client
	if client == nil {
		safeClient = http.Client{Timeout: defaultTimeout}
	} else {
		safeClient = *client
	}
	origRedirect := safeClient.CheckRedirect
	safeClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if len(via) > 0 && strings.EqualFold(via[len(via)-1].URL.Scheme, "https") &&
			!strings.EqualFold(req.URL.Scheme, "https") {
			return fmt.Errorf("refusing redirect downgrade from https to %s", req.URL.Scheme)
		}
		if origRedirect != nil {
			return origRedirect(req, via)
		}
		return nil
	}
	return &safeClient
}

// DownloadVerifiedContext fetches url under ctx using client (or a bounded default
// client if nil), rejects HTTPS-to-HTTP redirect downgrades and payloads exceeding
// MaxDownloadBytes, verifies HTTP 200 OK and optional expectedSHA256 hex digest,
// and atomically writes the verified payload to targetPath with perm.
func DownloadVerifiedContext(
	ctx context.Context,
	client *http.Client,
	url, targetPath, expectedSHA256 string,
	perm os.FileMode,
) (int64, error) {
	if targetPath == "" {
		return 0, errors.New("empty target path")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	safeClient := NewSafeHTTPClient(client, 10*time.Minute)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := safeClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	hasher := sha256.New()
	cleanSum := strings.TrimSpace(expectedSHA256)
	lr := &io.LimitedReader{R: resp.Body, N: MaxDownloadBytes + 1}
	written, _, err := writeReaderTempAndRename(
		targetPath,
		io.TeeReader(lr, hasher),
		perm,
		true,
		func() error {
			if lr.N <= 0 {
				return fmt.Errorf("download from %s exceeds maximum size (%d bytes)", url, MaxDownloadBytes)
			}
			if cleanSum != "" {
				actualSum := hex.EncodeToString(hasher.Sum(nil))
				if !strings.EqualFold(actualSum, cleanSum) {
					return fmt.Errorf("sha256 mismatch for %s: got %s, want %s", url, actualSum, cleanSum)
				}
			}
			return nil
		},
	)
	return written, err
}
