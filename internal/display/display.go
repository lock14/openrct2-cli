// Package display provides native primary monitor resolution and OpenRCT2 UI scale detection.
package display

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const probeTimeout = 1500 * time.Millisecond

var (
	reResFormat     = regexp.MustCompile(`^(\d+)x(\d+)$`)
	reDarwinRes     = regexp.MustCompile(`(?i)Resolution:\s*(\d+)\s*x\s*(\d+)`)
	reXrandrRes     = regexp.MustCompile(`^\s*(\d+x\d+)\s+.*\*\+?`)
	reWlrRes        = regexp.MustCompile(`(\d+x\d+)\s+px,\s+\d+(?:\.\d+)?\s+Hz\s+\([^)\r\n]*\bcurrent\b[^)\r\n]*\)`)
	reWinController = regexp.MustCompile(`^(\d+)x(\d+)(?:@\d+)?$`)
)

// Info describes detected display geometry and recommended OpenRCT2 window settings.
type Info struct {
	FullscreenWidth  int
	FullscreenHeight int
	WindowWidth      int
	WindowHeight     int
	WindowScale      float64
}

// ScaleString formats WindowScale with six decimal places matching OpenRCT2 config.ini conventions.
func (d Info) ScaleString() string {
	scale := d.WindowScale
	if scale <= 0 {
		scale = 1.0
	}
	return fmt.Sprintf("%.6f", scale)
}

func runProbe(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

// Detect queries the current OS display server and returns recommended OpenRCT2 display geometry.
func Detect() Info {
	w, h := detectResolutionOS(runtime.GOOS)
	return BuildInfo(w, h)
}

// BuildInfo computes full OpenRCT2 display and window parameters from a monitor resolution.
func BuildInfo(width, height int) Info {
	if width <= 0 || height <= 0 {
		width = 1920
		height = 1080
	}
	winW, winH := WindowDimensions(width, height)
	return Info{
		FullscreenWidth:  width,
		FullscreenHeight: height,
		WindowWidth:      winW,
		WindowHeight:     winH,
		WindowScale:      RecommendedWindowScale(width, height),
	}
}

// RecommendedWindowScale computes a sensible default HiDPI window_scale for width x height.
func RecommendedWindowScale(width, height int) float64 {
	switch {
	case height > 1600 || width >= 3400:
		return 2.0
	case height > 1080 || width > 1920:
		return 1.5
	default:
		return 1.0
	}
}

// WindowDimensions computes non-fullscreen window dimensions leaving room for OS panels/taskbars.
func WindowDimensions(width, height int) (int, int) {
	if width <= 0 {
		width = 1920
	}
	if height <= 0 {
		height = 1080
	}
	winH := height - 85
	if winH < 600 {
		winH = height
	}
	return width, winH
}

func parseResPair(res string) (int, int) {
	m := reResFormat.FindStringSubmatch(strings.TrimSpace(res))
	if len(m) < 3 {
		return 0, 0
	}
	w, wErr := strconv.Atoi(m[1])
	h, hErr := strconv.Atoi(m[2])
	if wErr != nil || hErr != nil || w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}

func parseDarwinResolution(out string) (int, int) {
	if m := reDarwinRes.FindStringSubmatch(out); len(m) >= 3 {
		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		if w > 0 && h > 0 {
			return w, h
		}
	}
	return 0, 0
}

func parseXrandrResolution(out string) (int, int) {
	var primRes, fbRes string
	inPrimary := false

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			if strings.Contains(line, " connected") {
				inPrimary = strings.Contains(line, " primary ") || strings.HasSuffix(line, " primary")
			} else {
				inPrimary = false
			}
			continue
		}
		m := reXrandrRes.FindStringSubmatch(line)
		if len(m) < 2 {
			continue
		}
		if inPrimary && primRes == "" {
			primRes = m[1]
		} else if fbRes == "" {
			fbRes = m[1]
		}
	}
	if primRes != "" {
		return parseResPair(primRes)
	}
	return parseResPair(fbRes)
}

func parseWlrResolution(out string) (int, int) {
	if m := reWlrRes.FindStringSubmatch(out); len(m) >= 2 {
		return parseResPair(m[1])
	}
	return 0, 0
}

func parseWindowsResolution(out string) (int, int) {
	var bestW, bestH int
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		m := reWinController.FindStringSubmatch(line)
		if len(m) < 3 {
			continue
		}
		w, wErr := strconv.Atoi(m[1])
		h, hErr := strconv.Atoi(m[2])
		if wErr == nil && hErr == nil && w*h > bestW*bestH {
			bestW, bestH = w, h
		}
	}
	return bestW, bestH
}

func detectResolutionOS(goos string) (int, int) {
	switch goos {
	case "darwin":
		if out, err := runProbe("system_profiler", "SPDisplaysDataType"); err == nil {
			if w, h := parseDarwinResolution(string(out)); w > 0 && h > 0 {
				return w, h
			}
		}
		return 1920, 1080

	case "windows":
		winScript := "Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | " +
			"ForEach-Object { \"$($_.CurrentHorizontalResolution)x$($_.CurrentVerticalResolution)\" }; " +
			"Add-Type -TypeDefinition 'using System.Runtime.InteropServices; " +
			"public class Dpi { [DllImport(\"user32.dll\")] public static extern bool SetProcessDPIAware(); }'; " +
			"[void][Dpi]::SetProcessDPIAware(); " +
			"Add-Type -AssemblyName System.Windows.Forms; " +
			"[System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Width.ToString() + 'x' + " +
			"[System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Height.ToString()"
		if out, err := runProbe("powershell", "-NoProfile", "-Command", winScript); err == nil {
			if w, h := parseWindowsResolution(string(out)); w > 0 && h > 0 {
				return w, h
			}
		}
		return 1920, 1080

	default: // linux and unix
		if out, err := runProbe("xrandr"); err == nil {
			if w, h := parseXrandrResolution(string(out)); w > 0 && h > 0 {
				return w, h
			}
		}
		if out, err := runProbe("wlr-randr"); err == nil {
			if w, h := parseWlrResolution(string(out)); w > 0 && h > 0 {
				return w, h
			}
		}
		if w, h := detectDRMResolution(); w > 0 && h > 0 {
			return w, h
		}
		return 1920, 1080
	}
}

func detectDRMResolution() (int, int) {
	modes, _ := filepath.Glob("/sys/class/drm/*/modes")
	for _, modeFile := range modes {
		data, err := os.ReadFile(modeFile)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) > 0 {
			if w, h := parseResPair(lines[0]); w > 0 && h > 0 {
				return w, h
			}
		}
	}
	return 0, 0
}
