package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/engine"
)

func newEngineCmd() *cobra.Command {
	engineCmd := &cobra.Command{
		Use:     "engine",
		Aliases: []string{"engines"},
		Short:   "Manage portable OpenRCT2 engine builds and release channels",
		Args:    cobra.NoArgs,
		RunE:    runEngineStatus,
	}

	engineCmd.AddCommand(
		newEngineStatusCmd(),
		newEngineInstallCmd(),
		newEngineUpdateCmd(),
		newEngineChannelCmd(),
	)
	return engineCmd
}

func newEngineStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Aliases: []string{"list"},
		Short:   "Display installed OpenRCT2 binary status, receipt version, and active channel",
		Args:    cobra.NoArgs,
		RunE:    runEngineStatus,
	}
}

func runEngineStatus(cmd *cobra.Command, _ []string) error {
	out := cmdOut(cmd)
	errOut := cmdErr(cmd)
	paths, cfg, cat, err := loadAppState(errOut)
	if err != nil {
		return err
	}

	activeChannel := cfg.EffectiveChannel()
	chCfg := cat.FindChannel(activeChannel)
	managedBin := paths.ManagedEngineBinaryForOS(runtime.GOOS)
	resolvedBin, resolveErr := engine.ResolveBinary(paths, false, runtime.GOOS)
	rec, _ := engine.LoadReceipt(paths.EngineReceiptFile)

	status := "[✗ Missing]"
	if resolveErr == nil {
		status = "[✓ Ready]"
	}

	fmt.Fprintf(out, "Status:         %s\n", status)
	fmt.Fprintf(out, "Active Channel: %s", activeChannel)
	if chCfg != nil {
		fmt.Fprintf(out, " (%s)\n", chCfg.Repo)
	} else {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "Managed Path:   %s\n", managedBin)
	if resolveErr == nil && resolvedBin != managedBin {
		fmt.Fprintf(out, "Active Binary:  %s (system fallback)\n", resolvedBin)
	}
	if rec != nil {
		if fi, statErr := os.Stat(rec.BinaryPath); statErr == nil && !fi.IsDir() {
			fmt.Fprintf(out, "Installed Tag:  %s (channel: %s, installed: %s)\n", rec.Tag, rec.Channel, rec.InstalledAt)
		}
	}
	return nil
}

func newEngineInstallCmd() *cobra.Command {
	var (
		channelFlag string
		force       bool
	)
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Download and install portable OpenRCT2 into user-space",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executeEngineInstall(cmd, channelFlag, force, false)
		},
	}
	installCmd.Flags().StringVar(&channelFlag, "channel", "", "Channel to install (release or develop)")
	installCmd.Flags().BoolVar(&force, "force", false, "Force re-download even if already installed")
	return installCmd
}

func newEngineUpdateCmd() *cobra.Command {
	var (
		channelFlag string
		force       bool
	)
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Check upstream GitHub Releases and update OpenRCT2 if a newer build is available",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executeEngineInstall(cmd, channelFlag, force, true)
		},
	}
	updateCmd.Flags().StringVar(&channelFlag, "channel", "", "Channel to update (release or develop)")
	updateCmd.Flags().BoolVar(&force, "force", false, "Force re-download even if receipt matches upstream")
	return updateCmd
}

func executeEngineInstall(cmd *cobra.Command, channelFlag string, force, checkUpdate bool) error {
	out := cmdOut(cmd)
	errOut := cmdErr(cmd)
	paths, cfg, cat, err := loadAppState(errOut)
	if err != nil {
		return err
	}

	effectiveChannel := cfg.EffectiveChannel()
	if strings.TrimSpace(channelFlag) != "" {
		norm, chErr := config.NormalizeChannel(channelFlag)
		if chErr != nil {
			return chErr
		}
		effectiveChannel = norm
	}

	chCfg := cat.FindChannel(effectiveChannel)
	if chCfg == nil {
		return fmt.Errorf("channel %q not found in catalog", effectiveChannel)
	}

	ins := engine.NewInstaller(paths, out)
	if _, err := ins.Install(cmd.Context(), *chCfg, force, checkUpdate); err != nil {
		return err
	}

	if strings.TrimSpace(channelFlag) != "" && cfg.Channel != effectiveChannel {
		cfg.Channel = effectiveChannel
		if err := config.SaveConfig(paths, cfg); err != nil {
			return err
		}
	}
	return nil
}

func newEngineChannelCmd() *cobra.Command {
	var installNow bool
	chCmd := &cobra.Command{
		Use:       "channel [release|develop]",
		Short:     "Show or switch the active OpenRCT2 release channel",
		ValidArgs: []string{"release", "develop"},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, cfg, cat, err := loadAppState(errOut)
			if err != nil {
				return err
			}

			if len(args) == 0 {
				fmt.Fprintln(out, cfg.EffectiveChannel())
				return nil
			}

			norm, err := config.NormalizeChannel(args[0])
			if err != nil {
				return err
			}
			chCfg := cat.FindChannel(norm)
			if chCfg == nil {
				return fmt.Errorf("channel %q not found in catalog", norm)
			}

			cfg.Channel = norm
			if err := config.SaveConfig(paths, cfg); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Active channel set to %s (%s)\n", norm, chCfg.Repo)

			if installNow {
				ins := engine.NewInstaller(paths, out)
				_, err := ins.Install(cmd.Context(), *chCfg, false, true)
				return err
			}
			fmt.Fprintln(out, "  Run 'rct engine update' to install the latest build from this channel.")
			return nil
		},
	}
	chCmd.Flags().BoolVar(&installNow, "install", false, "Immediately download and install the selected channel")
	return chCmd
}
