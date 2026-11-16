package display

import "testing"

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
		{"1080p", 1920, 1080, 1920, 1080, 995, "1.000000"},
		{"1200p", 1920, 1200, 1920, 1200, 1115, "1.500000"},
		{"1440p", 2560, 1440, 2560, 1440, 1355, "1.500000"},
		{"4k", 3840, 2160, 3840, 2160, 2075, "2.000000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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

func TestDisplayOutputParsers(t *testing.T) {
	t.Parallel()
	xrandrSample := `Screen 0: minimum 8 x 8, current 4480 x 1440
HDMI-0 connected 1920x1080+0+0
   1920x1080     60.00*+
DP-0 connected primary 2560x1440+1920+0
   2560x1440    144.00*+  60.00
`
	if w, h := parseXrandrResolution(xrandrSample); w != 2560 || h != 1440 {
		t.Fatalf("parseXrandrResolution = %dx%d, want 2560x1440", w, h)
	}

	wlrSample := `eDP-1 "Monitor"
  Modes:
    1920x1200 px, 60.002000 Hz (preferred, current)
`
	if w, h := parseWlrResolution(wlrSample); w != 1920 || h != 1200 {
		t.Fatalf("parseWlrResolution = %dx%d, want 1920x1200", w, h)
	}

	darwinSample := `Graphics/Displays:
      Displays:
        Color LCD:
          Resolution: 3456 x 2234 Retina
`
	if w, h := parseDarwinResolution(darwinSample); w != 3456 || h != 2234 {
		t.Fatalf("parseDarwinResolution = %dx%d, want 3456x2234", w, h)
	}

	winSample := "1920x1080\r\n3840x2160\r\n"
	if w, h := parseWindowsResolution(winSample); w != 3840 || h != 2160 {
		t.Fatalf("parseWindowsResolution = %dx%d, want 3840x2160", w, h)
	}

	info := Detect()
	if info.FullscreenWidth <= 0 || info.FullscreenHeight <= 0 {
		t.Fatalf("Detect() returned invalid geometry: %+v", info)
	}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		w, h := detectResolutionOS(goos)
		if w <= 0 || h <= 0 {
			t.Fatalf("detectResolutionOS(%s) = %dx%d", goos, w, h)
		}
	}
}
