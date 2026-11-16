package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/content"
	"github.com/lock14/openrct2-cli/internal/display"
	"github.com/lock14/openrct2-cli/internal/engine"
	"github.com/lock14/openrct2-cli/internal/gamedata"
	"github.com/lock14/openrct2-cli/internal/rctcfg"
)

func newSetupCmd() *cobra.Command {
	var (
		force          bool
		channelFlag    string
		skipAddonsFlag bool
	)

	setupCmd := &cobra.Command{
		Use:          "setup",
		Short:        "Complete 1-command setup (RCT1/RCT2 discovery, configs, engine, asset packs, and plugins)",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			ctx := cmd.Context()

			paths, cfg, cat, err := loadAppState(errOut)
			if err != nil {
				return err
			}

			effectiveChannel := cfg.EffectiveChannel()
			if strings.TrimSpace(channelFlag) != "" {
				normCh, chErr := config.NormalizeChannel(channelFlag)
				if chErr != nil {
					return chErr
				}
				effectiveChannel = normCh
			}
			chCfg := cat.FindChannel(effectiveChannel)
			if chCfg == nil {
				return fmt.Errorf("channel %q is not defined in catalog", effectiveChannel)
			}

			fmt.Fprintln(out, "============================================================")
			fmt.Fprintln(out, "  Initiating OpenRCT2 Setup")
			fmt.Fprintln(out, "============================================================")
			fmt.Fprintln(out)

			// Step 1/5: Discover RCT2 & RCT1 game directories
			fmt.Fprintln(out, ">>> Step 1/5: Auto-discovering RCT2 and RCT1 game installations...")
			existingRCT2, existingRCT1 := rctcfg.ReadExistingGamePaths(filepath.Join(paths.OpenRCT2Dir, "config.ini"))
			roots := gamedata.DefaultSearchRoots(cfg.RCT2Path, cfg.RCT1Path, existingRCT2, existingRCT1)
			disc := gamedata.Discover(roots)
			if disc.RCT2Path != "" {
				fmt.Fprintf(out, "✓ Found %s -> %s\n", disc.RCT2Edition, disc.RCT2Path)
			} else {
				fmt.Fprintln(
					out,
					"  ! RCT2 assets not found automatically (set later with 'rct games set --rct2 <dir>')",
				)
			}
			if disc.RCT1Path != "" {
				fmt.Fprintf(out, "✓ Found RollerCoaster Tycoon 1 -> %s\n", disc.RCT1Path)
			}
			fmt.Fprintln(out)
			if err := ctx.Err(); err != nil {
				return err
			}

			// Step 2/5: Deploy configurations and Linux FreeDesktop integration
			fmt.Fprintln(out, ">>> Step 2/5: Deploying OpenRCT2 configurations and desktop integration...")
			disp := display.Detect()
			if err := rctcfg.Deploy(paths, rctcfg.DeployOptions{
				TargetOS:  runtime.GOOS,
				Component: rctcfg.ComponentAll,
				Force:     force,
				Render: rctcfg.RenderOptions{
					Display:    disp,
					RCT2Path:   disc.RCT2Path,
					RCT1Path:   disc.RCT1Path,
					SaveDir:    paths.SaveDir,
					PlayerName: rctcfg.ResolvePlayerName(),
					ExecPath:   paths.ManagedEngineBinaryForOS(runtime.GOOS),
				},
			}, out); err != nil {
				return fmt.Errorf("deploying configurations: %w", err)
			}
			fmt.Fprintln(out)
			if err := ctx.Err(); err != nil {
				return err
			}

			var failedSteps []string

			// Step 3/5: Install portable OpenRCT2 engine
			fmt.Fprintf(out, ">>> Step 3/5: Installing OpenRCT2 engine (%s channel)...\n", chCfg.ID)
			engIns := engine.NewInstaller(paths, out)
			if _, err := engIns.Install(ctx, *chCfg, force, false); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				fmt.Fprintf(errOut, "Warning: engine installation encountered: %v\n", err)
				failedSteps = append(failedSteps, "engine")
			}
			fmt.Fprintln(out)
			if err := ctx.Err(); err != nil {
				return err
			}

			if !skipAddonsFlag {
				var packs, plugins []catalog.Addon
				for _, ad := range cat.Addons {
					if ad.AddonType == catalog.AddonTypeAssetPack {
						packs = append(packs, ad)
					} else {
						plugins = append(plugins, ad)
					}
				}

				contentIns := content.NewInstaller(paths, force, out)

				// Step 4/5: Official audio/music/title-sequence asset packs
				fmt.Fprintln(out, ">>> Step 4/5: Installing official OpenRCT2 asset packs...")
				if err := contentIns.InstallAll(ctx, packs); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					fmt.Fprintf(errOut, "Warning: asset pack installation encountered: %v\n", err)
					failedSteps = append(failedSteps, "asset packs")
				}
				fmt.Fprintln(out)
				if err := ctx.Err(); err != nil {
					return err
				}

				// Step 5/5: Curated community JS plugins
				fmt.Fprintln(out, ">>> Step 5/5: Installing curated OpenRCT2 JavaScript plugins...")
				if err := contentIns.InstallAll(ctx, plugins); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					fmt.Fprintf(errOut, "Warning: plugin installation encountered: %v\n", err)
					failedSteps = append(failedSteps, "plugins")
				}
				fmt.Fprintln(out)
			}

			if strings.TrimSpace(channelFlag) != "" && cfg.Channel != effectiveChannel {
				cfg.Channel = effectiveChannel
				_ = config.SaveConfig(paths, cfg)
			}

			if len(failedSteps) > 0 {
				summary := strings.Join(failedSteps, ", ")
				fmt.Fprintln(out, "============================================================")
				fmt.Fprintf(out, "  ! OpenRCT2 setup finished with errors in: %s\n", summary)
				fmt.Fprintln(out, "============================================================")
				return fmt.Errorf("setup completed with errors in: %s", summary)
			}

			fmt.Fprintln(out, "============================================================")
			fmt.Fprintln(out, "  ✓ OpenRCT2 setup complete!")
			fmt.Fprintln(out, "  Run 'rct' to start building your park!")
			fmt.Fprintln(out, "============================================================")
			return nil
		},
	}

	setupCmd.Flags().BoolVar(&force, "force", false, "Force re-download and overwrite existing files from templates")
	setupCmd.Flags().StringVar(
		&channelFlag, "channel", "", "Engine release channel to install (release or develop)",
	)
	setupCmd.Flags().BoolVar(&skipAddonsFlag, "skip-addons", false, "Skip downloading asset packs and JS plugins")

	return setupCmd
}
