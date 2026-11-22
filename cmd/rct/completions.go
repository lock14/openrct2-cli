package main

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/rctcfg"
)

func registerChannelFlagCompletion(cmd *cobra.Command, flagName string) {
	_ = cmd.RegisterFlagCompletionFunc(
		flagName,
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{config.ChannelRelease, config.ChannelDevelop}, cobra.ShellCompDirectiveNoFileComp
		},
	)
}

func registerAddonTypeFlagCompletion(cmd *cobra.Command, flagName string) {
	_ = cmd.RegisterFlagCompletionFunc(
		flagName,
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{catalog.AddonTypePlugin, catalog.AddonTypeAssetPack}, cobra.ShellCompDirectiveNoFileComp
		},
	)
}

func registerDeployComponentFlagCompletion(cmd *cobra.Command, flagName string) {
	_ = cmd.RegisterFlagCompletionFunc(
		flagName,
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return rctcfg.ValidDeployComponents(), cobra.ShellCompDirectiveNoFileComp
		},
	)
}

func completeAddonIDs(includeAll bool) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		paths, _ := loadPathsAndConfig(io.Discard)
		cat, err := catalog.LoadLayered(paths.CatalogDir, false)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		completions := make([]string, 0, len(cat.Addons)+1)
		if includeAll {
			completions = append(completions, "all\tInstall all matching add-ons")
		}
		for _, ad := range cat.Addons {
			completions = append(completions, ad.ID+"\t"+ad.EffectiveDisplayName())
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeConfigKeys(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return config.ValidKeys(), cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) == 1 && args[0] == config.KeyChannel {
		return []string{config.ChannelRelease, config.ChannelDevelop}, cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveDefault
}
