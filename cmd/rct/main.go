// Package main is the entrypoint for the unified OpenRCT2 CLI (rct).
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/engine"
)

var (
	// Build-time metadata injected via -ldflags.
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		dryRun   bool
		headless bool
	)

	rootCmd := &cobra.Command{
		Use:     "rct [file|uri] [flags]",
		Short:   "Unified OpenRCT2 launcher, setup tool, engine updater, and add-on manager",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
		Long: `Launch OpenRCT2 directly, run 1-command turnkey setup, switch between stable
release and develop builds, auto-discover Steam and GOG RCT1/RCT2 assets, sync
curated WASD configurations, and manage official asset packs and JS plugins.

Run 'rct' with no arguments to launch OpenRCT2 directly, pass a park/save/URI
('rct mypark.park', 'rct --headless server.park --port 11753'), or run
'rct setup' for turnkey installation.`,
		Args:               cobra.ArbitraryArgs,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableFlagParsing: true,
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		RunE: func(cmd *cobra.Command, args []string) error {
			if hasStandaloneBoolFlag(args, "-h", "--help") {
				return cmd.Help()
			}
			if hasStandaloneBoolFlag(args, "-v", "--version") {
				fmt.Fprintf(cmdOut(cmd), "%s version %s\n", cmd.DisplayName(), cmd.Version)
				return nil
			}

			target, extraArgs, parsedDryRun, parsedHeadless, err := parseRootArgs(args)
			if err != nil {
				return err
			}

			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, cfg, _, err := loadAppState(errOut)
			if err != nil {
				return err
			}

			var customUserDir string
			if cfg != nil && strings.TrimSpace(cfg.OpenRCT2Dir) != "" {
				customUserDir = paths.OpenRCT2Dir
			}

			plan, err := engine.PrepareLaunch(paths, engine.LaunchOptions{
				TargetOS:          runtime.GOOS,
				Headless:          headless || parsedHeadless,
				DryRun:            dryRun || parsedDryRun,
				CustomUserDataDir: customUserDir,
				Target:            target,
				ExtraArgs:         extraArgs,
			})
			if err != nil {
				return err
			}
			return engine.Execute(cmd.Context(), plan, out, errOut)
		},
	}

	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print synthesized OpenRCT2 launch command without executing")
	rootCmd.Flags().BoolVar(&headless, "headless", false, "Launch OpenRCT2 in headless server/CLI mode")
	rootCmd.InitDefaultHelpFlag()
	rootCmd.InitDefaultVersionFlag()

	rootCmd.AddCommand(
		newSetupCmd(),
		newEngineCmd(),
		newGamesCmd(),
		newAddonsCmd(),
		newConfigCmd(),
	)

	return rootCmd
}

func hasStandaloneBoolFlag(args []string, shortFlag, longFlag string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == shortFlag || arg == longFlag {
			return true
		}
		if name, val, hasEq := strings.Cut(arg, "="); hasEq && (name == shortFlag || name == longFlag) {
			b, err := strconv.ParseBool(val)
			return err == nil && b
		}
	}
	return false
}

func parseRootArgs(args []string) (string, []string, bool, bool, error) {
	var target string
	var extra []string
	var dryRun, headless bool

	for i, arg := range args {
		if arg == "--" {
			if i+1 < len(args) {
				extra = append(extra, args[i+1:]...)
			}
			break
		}
		if arg == "--dry-run" {
			dryRun = true
			continue
		}
		if arg == "--headless" {
			headless = true
			continue
		}
		if name, val, hasEq := strings.Cut(arg, "="); hasEq && (name == "--dry-run" || name == "--headless") {
			b, err := strconv.ParseBool(val)
			if err != nil {
				return "", nil, false, false, fmt.Errorf("invalid boolean value %q for %s: %w", val, name, err)
			}
			if name == "--dry-run" {
				dryRun = b
			} else {
				headless = b
			}
			continue
		}
		if target == "" && !strings.HasPrefix(arg, "-") {
			target = arg
			continue
		}
		extra = append(extra, arg)
	}
	return target, extra, dryRun, headless, nil
}

func cmdOut(cmd *cobra.Command) io.Writer {
	if cmd != nil {
		return cmd.OutOrStdout()
	}
	return os.Stdout
}

func cmdErr(cmd *cobra.Command) io.Writer {
	if cmd != nil {
		return cmd.ErrOrStderr()
	}
	return os.Stderr
}

func loadPathsAndConfig(errOut io.Writer) (*config.Paths, *config.AppConfig) {
	if errOut == nil {
		errOut = os.Stderr
	}
	p := config.GetPaths()
	cfg, err := config.LoadConfig(p)
	if err != nil {
		fmt.Fprintf(errOut, "Warning: could not parse %s (%v); using defaults\n", p.ConfigFile, err)
		cfg = &config.AppConfig{}
	}
	config.ApplyConfigPaths(p, cfg, runtime.GOOS)
	return p, cfg
}

func loadAppState(errOut io.Writer) (*config.Paths, *config.AppConfig, *catalog.Catalog, error) {
	p, cfg := loadPathsAndConfig(errOut)
	cat, err := catalog.LoadLayered(p.CatalogDir, false)
	if err != nil {
		return p, cfg, nil, err
	}
	for _, d := range cat.Diagnostics {
		fmt.Fprintf(errOut, "warning: %s\n", d.String())
	}
	return p, cfg, cat, nil
}

func resolveTargetChannel(
	cfg *config.AppConfig,
	cat *catalog.Catalog,
	channelFlag string,
) (string, *catalog.ChannelConfig, error) {
	effectiveChannel := cfg.EffectiveChannel()
	if strings.TrimSpace(channelFlag) != "" {
		norm, err := config.NormalizeChannel(channelFlag)
		if err != nil {
			return "", nil, err
		}
		effectiveChannel = norm
	}
	chCfg := cat.FindChannel(effectiveChannel)
	if chCfg == nil {
		return "", nil, fmt.Errorf("channel %q not found in catalog", effectiveChannel)
	}
	return effectiveChannel, chCfg, nil
}
