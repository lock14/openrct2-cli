package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/display"
	"github.com/lock14/openrct2-cli/internal/gamedata"
	"github.com/lock14/openrct2-cli/internal/rctcfg"
)

func newConfigCmd() *cobra.Command {
	cfgCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI settings and deploy, diff, sync, or uninstall OpenRCT2 configurations",
		Args:  cobra.NoArgs,
		RunE:  runConfigShow,
	}

	cfgCmd.AddCommand(
		newConfigShowCmd(),
		newConfigGetCmd(),
		newConfigSetCmd(),
		newConfigInstallCmd(),
		newConfigDiffCmd(),
		newConfigSyncCmd(),
		newConfigUninstallCmd(),
	)
	return cfgCmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display resolved platform paths and active CLI settings with provenance",
		Args:  cobra.NoArgs,
		RunE:  runConfigShow,
	}
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
	out := cmdOut(cmd)
	errOut := cmdErr(cmd)
	paths, cfg := loadPathsAndConfig(errOut)

	fmt.Fprintf(out, "CLI Config File:  %s\n", paths.ConfigFile)
	fmt.Fprintf(out, "User Catalog Dir: %s\n", paths.CatalogDir)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "channel:          %s (%s)\n", cfg.EffectiveChannel(), config.ValueSource(cfg.Channel))
	fmt.Fprintf(out, "openrct2_dir:     %s (%s)\n", paths.OpenRCT2Dir, config.ValueSource(cfg.OpenRCT2Dir))
	fmt.Fprintf(out, "bin_dir:          %s (%s)\n", paths.BinDir, config.ValueSource(cfg.BinDir))
	fmt.Fprintf(out, "engine_dir:       %s (%s)\n", paths.EngineDir, config.ValueSource(cfg.EngineDir))
	if runtime.GOOS == "darwin" {
		fmt.Fprintf(out, "apps_dir:         %s (%s)\n", paths.AppsDir, config.ValueSource(cfg.AppsDir))
	}
	if runtime.GOOS == "linux" {
		fmt.Fprintf(out, "desktop_dir:      %s (xdg)\n", paths.DesktopDir)
	}
	if cfg.RCT2Path != "" {
		fmt.Fprintf(out, "rct2_path:        %s (config)\n", cfg.RCT2Path)
	}
	if cfg.RCT1Path != "" {
		fmt.Fprintf(out, "rct1_path:        %s (config)\n", cfg.RCT1Path)
	}
	return nil
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [key]",
		Short: "Retrieve an effective CLI configuration value",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runConfigShow(cmd, nil)
			}
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, cfg := loadPathsAndConfig(errOut)

			switch strings.ToLower(strings.TrimSpace(args[0])) {
			case "channel":
				fmt.Fprintln(out, cfg.EffectiveChannel())
			case "openrct2_dir", "openrct2-dir":
				fmt.Fprintln(out, paths.OpenRCT2Dir)
			case "bin_dir", "bin-dir":
				fmt.Fprintln(out, paths.BinDir)
			case "engine_dir", "engine-dir":
				fmt.Fprintln(out, paths.EngineDir)
			case "apps_dir", "apps-dir":
				fmt.Fprintln(out, paths.AppsDir)
			case "rct2_path", "rct2-path":
				fmt.Fprintln(out, cfg.RCT2Path)
			case "rct1_path", "rct1-path":
				fmt.Fprintln(out, cfg.RCT1Path)
			default:
				return unknownConfigKeyErr(args[0])
			}
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Persist a CLI configuration setting in <ConfigDir>/config.json",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, cfg := loadPathsAndConfig(errOut)

			key := strings.ToLower(strings.TrimSpace(args[0]))
			val := strings.TrimSpace(args[1])

			switch key {
			case "channel":
				norm, err := config.NormalizeChannel(val)
				if err != nil {
					return err
				}
				cfg.Channel = norm
				val = norm
			case "openrct2_dir", "openrct2-dir":
				cfg.OpenRCT2Dir = val
				key = "openrct2_dir"
			case "bin_dir", "bin-dir":
				cfg.BinDir = val
				key = "bin_dir"
			case "engine_dir", "engine-dir":
				cfg.EngineDir = val
				key = "engine_dir"
			case "apps_dir", "apps-dir":
				cfg.AppsDir = val
				key = "apps_dir"
			case "rct2_path", "rct2-path":
				cfg.RCT2Path = val
				key = "rct2_path"
			case "rct1_path", "rct1-path":
				cfg.RCT1Path = val
				key = "rct1_path"
			default:
				return unknownConfigKeyErr(args[0])
			}

			if err := config.SaveConfig(paths, cfg); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Set %s = %s\n", key, val)
			return nil
		},
	}
}

func unknownConfigKeyErr(rawKey string) error {
	return fmt.Errorf(
		"unknown configuration key %q "+
			"(valid: channel, openrct2_dir, bin_dir, engine_dir, apps_dir, rct2_path, rct1_path)",
		rawKey,
	)
}

func parseDeployComponent(raw string) (rctcfg.DeployComponent, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return rctcfg.ComponentAll, nil
	case "configs":
		return rctcfg.ComponentConfigs, nil
	case "config", "ini":
		return rctcfg.ComponentConfig, nil
	case "shortcuts", "keys":
		return rctcfg.ComponentShortcuts, nil
	case "desktop":
		return rctcfg.ComponentDesktop, nil
	case "icons", "icon":
		return rctcfg.ComponentIcons, nil
	default:
		return "", fmt.Errorf(
			"invalid --only component %q (expected all, configs, config, shortcuts, desktop, or icons)",
			raw,
		)
	}
}

func newConfigInstallCmd() *cobra.Command {
	var (
		force    bool
		onlyFlag string
	)
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Deploy embedded config.ini, WASD shortcuts.json, and Linux FreeDesktop files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, cfg := loadPathsAndConfig(errOut)

			comp, err := parseDeployComponent(onlyFlag)
			if err != nil {
				return err
			}

			iniRCT2, iniRCT1 := rctcfg.ReadExistingGamePaths(filepath.Join(paths.OpenRCT2Dir, "config.ini"))
			disc := gamedata.Discover(gamedata.DefaultSearchRoots(cfg.RCT2Path, cfg.RCT1Path, iniRCT2, iniRCT1))
			rct2 := disc.RCT2Path
			if rct2 == "" {
				rct2 = cfg.RCT2Path
			}
			rct1 := disc.RCT1Path
			if rct1 == "" {
				rct1 = cfg.RCT1Path
			}

			return rctcfg.Deploy(paths, rctcfg.DeployOptions{
				TargetOS:  runtime.GOOS,
				Component: comp,
				Force:     force,
				Render: rctcfg.RenderOptions{
					Display:    display.Detect(),
					RCT2Path:   rct2,
					RCT1Path:   rct1,
					SaveDir:    paths.SaveDir,
					PlayerName: rctcfg.ResolvePlayerName(),
					ExecPath:   paths.ManagedEngineBinaryForOS(runtime.GOOS),
				},
			}, out)
		},
	}
	installCmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config.ini with rendered curated template")
	installCmd.Flags().StringVar(
		&onlyFlag, "only", "all", "Deploy subset (all, configs, config, shortcuts, desktop, icons)",
	)
	return installCmd
}

func newConfigDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Compare live system OpenRCT2 configurations against rendered templates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, cfg := loadPathsAndConfig(errOut)
			return rctcfg.DiffConfigs(paths, rctcfg.RenderOptions{
				Display:    display.Detect(),
				RCT2Path:   cfg.RCT2Path,
				RCT1Path:   cfg.RCT1Path,
				SaveDir:    paths.SaveDir,
				PlayerName: rctcfg.ResolvePlayerName(),
				ExecPath:   paths.ManagedEngineBinaryForOS(runtime.GOOS),
			}, runtime.GOOS, out)
		},
	}
}

func newConfigSyncCmd() *cobra.Command {
	var repoDir string
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync live system OpenRCT2 tweaks back into repository templates (sanitizing personal paths)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, _ := loadPathsAndConfig(errOut)
			return rctcfg.SyncConfigs(paths, repoDir, runtime.GOOS, out)
		},
	}
	syncCmd.Flags().StringVar(&repoDir, "repo", ".", "Path to openrct2-cli git repository root")
	return syncCmd
}

func newConfigUninstallCmd() *cobra.Command {
	var removeAll bool
	unCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove deployed OpenRCT2 configurations and desktop entries (preserving saves and parks)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, _ := loadPathsAndConfig(errOut)
			return rctcfg.Uninstall(paths, removeAll, runtime.GOOS, out)
		},
	}
	unCmd.Flags().BoolVar(&removeAll, "all", false, "Also remove managed user-space OpenRCT2 engine binaries")
	return unCmd
}
