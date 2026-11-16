package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/gamedata"
	"github.com/lock14/openrct2-cli/internal/rctcfg"
)

func newGamesCmd() *cobra.Command {
	gamesCmd := &cobra.Command{
		Use:     "games",
		Aliases: []string{"game", "gamedata"},
		Short:   "Discover, validate, and link RCT2 and RCT1 base game asset directories",
		Args:    cobra.NoArgs,
		RunE:    runGamesStatus,
	}

	gamesCmd.AddCommand(
		newGamesStatusCmd(),
		newGamesScanCmd(),
		newGamesSetCmd(),
	)
	return gamesCmd
}

func newGamesStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show configured RCT2 and RCT1 directories and validate required asset files",
		Args:  cobra.NoArgs,
		RunE:  runGamesStatus,
	}
}

func runGamesStatus(cmd *cobra.Command, _ []string) error {
	out := cmdOut(cmd)
	errOut := cmdErr(cmd)
	paths, cfg := loadPathsAndConfig(errOut)

	iniPath := filepath.Join(paths.OpenRCT2Dir, "config.ini")
	iniRCT2, iniRCT1 := rctcfg.ReadExistingGamePaths(iniPath)

	effRCT2 := iniRCT2
	if effRCT2 == "" {
		effRCT2 = cfg.RCT2Path
	}
	effRCT1 := iniRCT1
	if effRCT1 == "" {
		effRCT1 = cfg.RCT1Path
	}

	if edition, ok := gamedata.ValidateRCT2Dir(effRCT2); ok {
		fmt.Fprintf(out, "RCT2 (game_path): [✓ Valid]   %s (%s)\n", effRCT2, edition)
	} else if effRCT2 != "" {
		fmt.Fprintf(out, "RCT2 (game_path): [✗ Invalid] %s (missing Data/g1.dat)\n", effRCT2)
	} else {
		fmt.Fprintln(out, "RCT2 (game_path): [✗ Missing] (run 'rct games scan' or 'rct games set --rct2 <dir>')")
	}

	if gamedata.ValidateRCT1Dir(effRCT1) {
		fmt.Fprintf(out, "RCT1 (rct1_path): [✓ Valid]   %s\n", effRCT1)
	} else if effRCT1 != "" {
		fmt.Fprintf(out, "RCT1 (rct1_path): [✗ Invalid] %s (missing Data/csg1.dat)\n", effRCT1)
	} else {
		fmt.Fprintln(out, "RCT1 (rct1_path): [Optional]  not configured")
	}

	return nil
}

func newGamesScanCmd() *cobra.Command {
	var (
		extraRoots []string
		dryRun     bool
	)
	scanCmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan Steam, GOG, Wine, and local libraries for RCT2 and RCT1 installations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, cfg := loadPathsAndConfig(errOut)

			iniRCT2, iniRCT1 := rctcfg.ReadExistingGamePaths(filepath.Join(paths.OpenRCT2Dir, "config.ini"))
			hints := append([]string{}, extraRoots...)
			hints = append(hints, cfg.RCT2Path, cfg.RCT1Path, iniRCT2, iniRCT1)

			roots := gamedata.DefaultSearchRoots(hints...)
			res := gamedata.Discover(roots)

			if res.RCT2Path == "" && res.RCT1Path == "" {
				fmt.Fprintln(out, "No RCT2 or RCT1 installations found in standard Steam/GOG paths.")
				fmt.Fprintln(out, "Use 'rct games set --rct2 <dir>' to link a custom installation folder.")
				return nil
			}

			if res.RCT2Path != "" {
				fmt.Fprintf(out, "✓ Found %s: %s\n", res.RCT2Edition, res.RCT2Path)
			}
			if res.RCT1Path != "" {
				fmt.Fprintf(out, "✓ Found RollerCoaster Tycoon 1: %s\n", res.RCT1Path)
			}

			if dryRun {
				return nil
			}
			return rctcfg.UpdateConfigGamePaths(paths, res.RCT2Path, res.RCT1Path, out)
		},
	}
	scanCmd.Flags().StringSliceVar(&extraRoots, "root", nil, "Additional search root directory to scan")
	scanCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report discovered paths without modifying config.ini")
	return scanCmd
}

func newGamesSetCmd() *cobra.Command {
	var (
		rct2Dir string
		rct1Dir string
	)
	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Validate and set RCT2 and/or RCT1 asset paths in config.ini and CLI settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, cfg := loadPathsAndConfig(errOut)

			rct2Clean := strings.TrimSpace(rct2Dir)
			rct1Clean := strings.TrimSpace(rct1Dir)
			if rct2Clean == "" && rct1Clean == "" {
				return errors.New("specify at least one of --rct2 or --rct1")
			}

			if rct2Clean != "" {
				abs2, err := filepath.Abs(rct2Clean)
				if err == nil {
					rct2Clean = abs2
				}
				if _, ok := gamedata.ValidateRCT2Dir(rct2Clean); !ok {
					return fmt.Errorf(
						"invalid RCT2 directory %q (expected Data/g1.dat and ObjData/ or Assets/g1.dat)",
						rct2Clean,
					)
				}
				cfg.RCT2Path = rct2Clean
			}
			if rct1Clean != "" {
				abs1, err := filepath.Abs(rct1Clean)
				if err == nil {
					rct1Clean = abs1
				}
				if !gamedata.ValidateRCT1Dir(rct1Clean) {
					return fmt.Errorf("invalid RCT1 directory %q (expected Data/csg1.dat)", rct1Clean)
				}
				cfg.RCT1Path = rct1Clean
			}

			if err := config.SaveConfig(paths, cfg); err != nil {
				return err
			}
			return rctcfg.UpdateConfigGamePaths(paths, rct2Clean, rct1Clean, out)
		},
	}
	setCmd.Flags().StringVar(&rct2Dir, "rct2", "", "Path to RollerCoaster Tycoon 2 or RCT Classic directory")
	setCmd.Flags().StringVar(&rct1Dir, "rct1", "", "Path to RollerCoaster Tycoon 1 (Deluxe) directory")
	return setCmd
}
