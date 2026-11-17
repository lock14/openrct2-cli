package main

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/content"
	"github.com/lock14/openrct2-cli/internal/repolint"
)

func newAddonsCmd() *cobra.Command {
	addonsCmd := &cobra.Command{
		Use:     "addons",
		Aliases: []string{"addon", "plugins", "plugin"},
		Short:   "Manage OpenRCT2 asset packs and JavaScript plugins",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAddonsList(cmd, "")
		},
	}

	addonsCmd.AddCommand(
		newAddonsListCmd(),
		newAddonsShowCmd(),
		newAddonsInstallCmd(),
		newAddonsRemoveCmd(),
		newAddonsAddCmd(),
		newAddonsLintCmd(),
		newAddonsBuildCmd(),
	)
	return addonsCmd
}

func newAddonsListCmd() *cobra.Command {
	var filterType string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List curated and custom OpenRCT2 asset packs and JS plugins with readiness status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAddonsList(cmd, filterType)
		},
	}
	listCmd.Flags().StringVar(&filterType, "type", "", "Filter by type (plugin or assetpack)")
	return listCmd
}

func runAddonsList(cmd *cobra.Command, filterType string) error {
	out := cmdOut(cmd)
	errOut := cmdErr(cmd)
	paths, _, cat, err := loadAppState(errOut)
	if err != nil {
		return err
	}

	ft := strings.ToLower(strings.TrimSpace(filterType))
	if ft != "" && ft != catalog.AddonTypePlugin && ft != catalog.AddonTypeAssetPack {
		return fmt.Errorf(
			"invalid --type %q (expected %q or %q)",
			filterType, catalog.AddonTypePlugin, catalog.AddonTypeAssetPack,
		)
	}

	for _, ad := range cat.Addons {
		if ft != "" && ad.AddonType != ft {
			continue
		}
		badge := "[✗ Missing]  "
		if content.IsInstalled(paths.OpenRCT2Dir, ad) {
			badge = "[✓ Installed]"
		}
		fmt.Fprintf(
			out,
			"%s  %-26s  %-10s  %-8s  %s\n",
			badge,
			ad.ID,
			ad.AddonType,
			ad.Version,
			ad.EffectiveDisplayName(),
		)
	}
	return nil
}

func newAddonsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Display metadata, source URL, SHA-256 checksum, and target files for an add-on",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, _, cat, err := loadAppState(errOut)
			if err != nil {
				return err
			}
			ad := cat.FindAddon(args[0])
			if ad == nil {
				return fmt.Errorf("add-on %q not found (run 'rct addons list' to view available add-ons)", args[0])
			}

			status := "Missing"
			if content.IsInstalled(paths.OpenRCT2Dir, *ad) {
				status = "Installed"
			}
			fmt.Fprintf(out, "ID:           %s\n", ad.ID)
			fmt.Fprintf(out, "Name:         %s\n", ad.EffectiveDisplayName())
			fmt.Fprintf(out, "Type:         %s\n", ad.AddonType)
			if ad.Category != "" {
				fmt.Fprintf(out, "Category:     %s\n", ad.Category)
			}
			if ad.Author != "" {
				fmt.Fprintf(out, "Author:       %s\n", ad.Author)
			}
			if ad.Version != "" {
				fmt.Fprintf(out, "Version:      %s\n", ad.Version)
			}
			fmt.Fprintf(out, "Layer:        %s\n", ad.Layer)
			fmt.Fprintf(out, "Status:       %s\n", status)
			if ad.Description != "" {
				fmt.Fprintf(out, "Description:  %s\n", ad.Description)
			}
			if ad.Homepage != "" {
				fmt.Fprintf(out, "Homepage:     %s\n", ad.Homepage)
			}
			fmt.Fprintf(out, "Source URL:   %s\n", ad.Source.URL)
			if ad.Source.SHA256 != "" {
				fmt.Fprintf(out, "SHA-256:      %s\n", ad.Source.SHA256)
			}
			fmt.Fprintf(out, "Install Mode: %s\n", ad.Source.Mode)
			fmt.Fprintf(out, "Targets:      %s\n", strings.Join(ad.Targets, ", "))
			return nil
		},
	}
}

func newAddonsInstallCmd() *cobra.Command {
	var (
		force      bool
		filterType string
	)
	installCmd := &cobra.Command{
		Use:     "install [id|all]",
		Aliases: []string{"fetch"},
		Short:   "Download, verify, and install one or all OpenRCT2 asset packs and JS plugins",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, _, cat, err := loadAppState(errOut)
			if err != nil {
				return err
			}

			ins := content.NewInstaller(paths, force, out)
			if len(args) == 0 || strings.EqualFold(args[0], "all") {
				ft := strings.ToLower(strings.TrimSpace(filterType))
				var targets []catalog.Addon
				for _, ad := range cat.Addons {
					if ft != "" && ad.AddonType != ft {
						continue
					}
					targets = append(targets, ad)
				}
				return ins.InstallAll(cmd.Context(), targets)
			}

			ad := cat.FindAddon(args[0])
			if ad == nil {
				return fmt.Errorf("add-on %q not found (run 'rct addons list' to view available add-ons)", args[0])
			}
			return ins.InstallAddon(cmd.Context(), *ad)
		},
	}
	installCmd.Flags().BoolVar(&force, "force", false, "Re-download and overwrite existing add-on files")
	installCmd.Flags().StringVar(&filterType, "type", "", "Filter when installing all (plugin or assetpack)")
	return installCmd
}

func newAddonsRemoveCmd() *cobra.Command {
	var purgeCatalog bool
	rmCmd := &cobra.Command{
		Use:     "remove <id>",
		Aliases: []string{"uninstall", "rm"},
		Short:   "Remove an installed add-on from OpenRCT2 (and optionally delete custom catalog entry)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, _, cat, err := loadAppState(errOut)
			if err != nil {
				return err
			}
			ad := cat.FindAddon(args[0])
			if ad == nil {
				return fmt.Errorf("add-on %q not found", args[0])
			}

			removedFiles, err := content.UninstallAddon(paths, *ad, out)
			if err != nil {
				return err
			}

			removedCatalog := false
			if ad.Layer == catalog.LayerUser || purgeCatalog {
				removedCatalog, err = catalog.RemoveUserAddon(paths.CatalogDir, ad.ID)
				if err != nil {
					return err
				}
				if removedCatalog {
					fmt.Fprintf(out, "✓ Removed custom catalog entry for %s\n", ad.ID)
				}
			}

			if removedFiles == 0 && !removedCatalog {
				fmt.Fprintf(out, "%s is not currently installed\n", ad.EffectiveDisplayName())
			}
			return nil
		},
	}
	rmCmd.Flags().BoolVar(&purgeCatalog, "purge-catalog", false, "Also delete user catalog.d JSON entry if present")
	return rmCmd
}

func newAddonsAddCmd() *cobra.Command {
	var (
		displayName string
		addonType   string
		url         string
		sha256Hex   string
		mode        string
		targetPath  string
		targetDir   string
		targets     []string
		versionStr  string
		description string
	)
	addCmd := &cobra.Command{
		Use:   "add <id> [flags]",
		Short: "Register a custom plugin or asset pack in <ConfigDir>/catalog.d/addon.<id>.json",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, _ := loadPathsAndConfig(errOut)

			id := strings.ToLower(strings.TrimSpace(args[0]))
			if strings.TrimSpace(url) == "" {
				return errors.New("--url is required")
			}
			effType := strings.ToLower(strings.TrimSpace(addonType))
			if effType == "" {
				effType = catalog.AddonTypePlugin
			}
			effMode := strings.ToLower(strings.TrimSpace(mode))
			if effMode == "" {
				if strings.HasSuffix(strings.ToLower(url), ".zip") {
					effMode = catalog.InstallModeZip
				} else {
					effMode = catalog.InstallModeFile
				}
			}
			if effMode == catalog.InstallModeFile && strings.TrimSpace(targetPath) == "" {
				base := path.Base(url)
				if base == "" || base == "." || base == "/" {
					base = id + ".js"
				}
				if effType == catalog.AddonTypePlugin {
					targetPath = "plugin/" + base
				} else {
					targetPath = "assetpack/" + base
				}
			}
			if effMode == catalog.InstallModeZip &&
				strings.TrimSpace(targetDir) == "" &&
				effType == catalog.AddonTypePlugin {
				targetDir = "plugin"
			}
			if strings.TrimSpace(displayName) == "" {
				displayName = id
			}

			ad := catalog.Addon{
				ID:          id,
				DisplayName: displayName,
				AddonType:   effType,
				Version:     versionStr,
				Description: description,
				Source: catalog.AddonSource{
					URL:        url,
					SHA256:     sha256Hex,
					Mode:       effMode,
					TargetPath: targetPath,
					TargetDir:  targetDir,
				},
				Targets: targets,
			}

			savedPath, err := catalog.SaveUserAddon(paths.CatalogDir, ad)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Saved custom add-on %q -> %s\n", id, savedPath)
			return nil
		},
	}

	addCmd.Flags().StringVar(&url, "url", "", "Download URL for the plugin (.js) or archive (.zip)")
	addCmd.Flags().StringVar(&displayName, "name", "", "Human-readable display name")
	addCmd.Flags().StringVar(&addonType, "type", "plugin", "Add-on type (plugin or assetpack)")
	addCmd.Flags().StringVar(&mode, "mode", "", "Install mode (file or zip; inferred from URL if omitted)")
	addCmd.Flags().StringVar(&targetPath, "target-path", "", "Relative destination file path for file mode")
	addCmd.Flags().StringVar(&targetDir, "target-dir", "", "Relative destination directory for zip mode")
	addCmd.Flags().StringSliceVar(&targets, "target", nil, "Expected relative file path(s) for readiness checks")
	addCmd.Flags().StringVar(&sha256Hex, "sha256", "", "Optional 64-character SHA-256 hex digest")
	addCmd.Flags().StringVar(&versionStr, "version-tag", "", "Version label (e.g. v1.0.0)")
	addCmd.Flags().StringVar(&description, "description", "", "Short description of the add-on")

	return addCmd
}

func newAddonsLintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lint [root]",
		Short: "Validate repository catalog entities, templates, docs, and user catalog.d schemas",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmdOut(cmd)
			errOut := cmdErr(cmd)
			paths, _ := loadPathsAndConfig(errOut)

			if _, err := catalog.LoadLayered(paths.CatalogDir, true); err != nil {
				return fmt.Errorf("user catalog validation failed: %w", err)
			}

			rootDir := "."
			if len(args) == 1 {
				rootDir = args[0]
			}
			if _, err := filepath.Abs(rootDir); err == nil {
				issues := repolint.LintRepo(rootDir)
				if len(issues) > 0 {
					for _, iss := range issues {
						fmt.Fprintf(errOut, "lint error: %s\n", iss.String())
					}
					return fmt.Errorf("found %d repository lint issue(s)", len(issues))
				}
			}

			fmt.Fprintln(out, "✓ All catalog entities, templates, and documentation passed validation!")
			return nil
		},
	}
}

func newAddonsBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "build [root]",
		Short:  "Synchronize README.md curated add-ons table from catalog/*.json",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmdOut(cmd)
			rootDir := "."
			if len(args) == 1 {
				rootDir = args[0]
			}
			cat, err := catalog.LoadCurated()
			if err != nil {
				return err
			}
			readmePath := filepath.Join(rootDir, "README.md")
			if err := repolint.SyncReadme(readmePath, cat); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Synchronized %s from curated catalog\n", readmePath)
			return nil
		},
	}
}
