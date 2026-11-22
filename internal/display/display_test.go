package display

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildInfoAndScales(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		w, h      int
		wantW     int
		wantH     int
		wantWinH  int
		wantScale string
	}{
		{"fallback default", 0, 0, 1920, 1080, 995, "1.000000"},
		{"tiny display", 800, 600, 800, 600, 600, "1.000000"},
		{"1080p", 1920, 1080, 1920, 1080, 995, "1.000000"},
		{"1200p", 1920, 1200, 1920, 1200, 1115, "1.500000"},
		{"1440p", 2560, 1440, 2560, 1440, 1355, "1.500000"},
		{"ultrawide", 3440, 1440, 3440, 1440, 1355, "2.000000"},
		{"4k", 3840, 2160, 3840, 2160, 2075, "2.000000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			info := BuildInfo(tc.w, tc.h)
			if info.FullscreenWidth != tc.wantW || info.FullscreenHeight != tc.wantH {
				t.Fatalf("fullscreen = %dx%d, want %dx%d",
					info.FullscreenWidth, info.FullscreenHeight, tc.wantW, tc.wantH)
			}
			if info.WindowHeight != tc.wantWinH {
				t.Fatalf("WindowHeight = %d, want %d", info.WindowHeight, tc.wantWinH)
			}
			if info.ScaleString() != tc.wantScale {
				t.Fatalf("ScaleString = %q, want %q", info.ScaleString(), tc.wantScale)
			}
		})
	}
}

func TestDisplayOutputParsersAndFallbackChains(t *testing.T) {
	t.Parallel()
	xrandrSample := `Screen 0: minimum 8 x 8, current 4480 x 1440
HDMI-0 connected 1920x1080+0+0
   1920x1080     60.00*+
DP-0 connected primary 2560x1440+1920+0
   2560x1440    144.00*+  60.00
`
	wlrSample := `eDP-1 "Monitor"
  Modes:
    1920x1200 px, 60.002000 Hz (preferred, current)
`
	darwinSample := `Graphics/Displays:
      Displays:
        Color LCD:
          Resolution: 3456 x 2234 Retina
`
	winSample := "1920x1080\r\n3840x2160\r\n"

	// 1. Linux xrandr primary monitor
	pXrandr := prober{
		run: func(name string, _ ...string) ([]byte, error) {
			if name == "xrandr" {
				return []byte(xrandrSample), nil
			}
			return nil, errors.New("not called")
		},
	}
	if w, h := pXrandr.detectResolutionOS("linux"); w != 2560 || h != 1440 {
		t.Fatalf("linux xrandr = %dx%d, want 2560x1440", w, h)
	}

	// 2. Linux Wayland wlr-randr fallback when xrandr fails
	pWlr := prober{
		run: func(name string, _ ...string) ([]byte, error) {
			if name == "wlr-randr" {
				return []byte(wlrSample), nil
			}
			return nil, errors.New("unavailable")
		},
	}
	if w, h := pWlr.detectResolutionOS("linux"); w != 1920 || h != 1200 {
		t.Fatalf("linux wlr-randr = %dx%d, want 1920x1200", w, h)
	}

	// 3. Linux DRM sysfs fallback when both xrandr and wlr-randr fail
	drmDir := filepath.Join(t.TempDir(), "card0-DP-1")
	if err := os.MkdirAll(drmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(drmDir, "modes"), []byte("3840x2160\n1920x1080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pDRM := prober{
		run:     func(string, ...string) ([]byte, error) { return nil, errors.New("no display server") },
		drmGlob: filepath.Join(filepath.Dir(drmDir), "*", "modes"),
	}
	if w, h := pDRM.detectResolutionOS("linux"); w != 3840 || h != 2160 {
		t.Fatalf("linux drm = %dx%d, want 3840x2160", w, h)
	}

	// 4. macOS and Windows probes + full fallback when unavailable
	pMacWin := prober{
		run: func(name string, _ ...string) ([]byte, error) {
			switch name {
			case "system_profiler":
				return []byte(darwinSample), nil
			case "powershell":
				return []byte(winSample), nil
			default:
				return nil, errors.New("unavailable")
			}
		},
	}
	if w, h := pMacWin.detectResolutionOS("darwin"); w != 3456 || h != 2234 {
		t.Fatalf("darwin = %dx%d, want 3456x2234", w, h)
	}
	if w, h := pMacWin.detectResolutionOS("windows"); w != 3840 || h != 2160 {
		t.Fatalf("windows = %dx%d, want 3840x2160", w, h)
	}

	pFail := prober{
		run:     func(string, ...string) ([]byte, error) { return nil, errors.New("offline") },
		drmGlob: filepath.Join(t.TempDir(), "none", "modes"),
	}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		if w, h := pFail.detectResolutionOS(goos); w != 1920 || h != 1080 {
			t.Fatalf("%s fallback = %dx%d, want 1920x1080", goos, w, h)
		}
	}
}
