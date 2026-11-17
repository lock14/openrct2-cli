package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lock14/openrct2-cli/internal/config"
)

const legacyOptAppImage = "/opt/OpenRCT2/OpenRCT2.AppImage"

// LaunchOptions configures direct OpenRCT2 execution or dry-run preview synthesis.
type LaunchOptions struct {
	TargetOS          string
	Headless          bool
	DryRun            bool
	CustomUserDataDir string
	Target            string
	ExtraArgs         []string
}

// LaunchPlan holds the resolved OpenRCT2 executable and synthesized CLI arguments.
type LaunchPlan struct {
	Binary string
	Args   []string
	DryRun bool
}

// ResolveBinary locates an installed OpenRCT2 executable across managed user-space,
// legacy /opt/OpenRCT2, and system PATH.
func ResolveBinary(paths *config.Paths, headless bool, targetOS string) (string, error) {
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	if paths != nil {
		cand := paths.ManagedEngineBinaryForOS(targetOS)
		if headless {
			cand = paths.ManagedHeadlessBinaryForOS(targetOS)
		}
		if cand != "" {
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				return cand, nil
			}
		}
		if payload := paths.EnginePayloadForOS(targetOS); payload != "" {
			if fi, err := os.Stat(payload); err == nil && !fi.IsDir() {
				return payload, nil
			}
		}
	}

	if targetOS == "linux" {
		if fi, err := os.Stat(legacyOptAppImage); err == nil && !fi.IsDir() {
			return legacyOptAppImage, nil
		}
	}

	if headless {
		if p, err := exec.LookPath("openrct2-cli"); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("openrct2"); err == nil {
		return p, nil
	}

	return "", errors.New("OpenRCT2 binary not found; run 'rct setup' or 'rct engine install'")
}

// PrepareLaunch synthesizes a LaunchPlan for OpenRCT2 from paths and opts.
func PrepareLaunch(paths *config.Paths, opts LaunchOptions) (*LaunchPlan, error) {
	targetOS := opts.TargetOS
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	bin, err := ResolveBinary(paths, opts.Headless, targetOS)
	if err != nil {
		return nil, err
	}

	var args []string
	baseLower := strings.ToLower(filepath.Base(bin))
	if opts.Headless && !strings.Contains(baseLower, "openrct2-cli") {
		args = append(args, "--headless")
	}
	if strings.TrimSpace(opts.Target) != "" {
		args = append(args, strings.TrimSpace(opts.Target))
	}
	if strings.TrimSpace(opts.CustomUserDataDir) != "" {
		args = append(args, "--user-data-path="+strings.TrimSpace(opts.CustomUserDataDir))
	}
	args = append(args, opts.ExtraArgs...)

	return &LaunchPlan{
		Binary: bin,
		Args:   args,
		DryRun: opts.DryRun,
	}, nil
}

// FormatCommandPreview formats a shell-safe human-readable command preview string.
func FormatCommandPreview(bin string, args []string) string {
	tokens := make([]string, 0, 1+len(args))
	tokens = append(tokens, quotePreviewToken(bin))
	for _, a := range args {
		tokens = append(tokens, quotePreviewToken(a))
	}
	return strings.Join(tokens, " ")
}

func quotePreviewToken(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\"'\\$") {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	return s
}

// Execute runs plan (or prints the preview command when plan.DryRun is true).
func Execute(ctx context.Context, plan *LaunchPlan, out, errOut io.Writer) error {
	if plan == nil {
		return errors.New("nil launch plan")
	}
	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}
	if plan.DryRun {
		fmt.Fprintln(out, FormatCommandPreview(plan.Binary, plan.Args))
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, plan.Binary, plan.Args...)
	cmd.Stdout = out
	cmd.Stderr = errOut
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
