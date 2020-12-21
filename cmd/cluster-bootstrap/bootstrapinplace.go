package main

import (
	"errors"
	"github.com/openshift/cluster-bootstrap/pkg/bootstrapinplace"
	"github.com/spf13/cobra"
)

var (
	CmdBootstrapInPlace = &cobra.Command{
		Use:          "bootstrap-in-place",
		Short:        "Enrich the master Ignition with control plane static pods manifests and all required resources",
		Long:         "",
		PreRunE:      validateBootstrapInPlaceOpts,
		RunE:         runCmdBootstrapInPlace,
		SilenceUsage: true,
	}

	bootstrapInPlaceOpts struct {
		assetDir     string
		ignitionPath string
	}
)

func init() {
	cmdRoot.AddCommand(CmdBootstrapInPlace)
	CmdBootstrapInPlace.Flags().StringVar(&bootstrapInPlaceOpts.assetDir, "asset-dir", "", "Path to the cluster asset folder.")
	CmdBootstrapInPlace.Flags().StringVar(&bootstrapInPlaceOpts.ignitionPath, "ignition-path", "/assets/master.ign", "The location of master ignition")

}

func runCmdBootstrapInPlace(cmd *cobra.Command, args []string) error {

	ib, err := bootstrapinplace.NewBootstrapInPlaceCommand(bootstrapinplace.ConfigBootstrapInPlace{
		AssetDir:     bootstrapInPlaceOpts.assetDir,
		IgnitionPath: bootstrapInPlaceOpts.ignitionPath,
	})
	if err != nil {
		return err
	}

	return ib.UpdateIgnitionWithBootstrapInPlaceData()
}

func validateBootstrapInPlaceOpts(cmd *cobra.Command, args []string) error {
	if bootstrapInPlaceOpts.ignitionPath == "" {
		return errors.New("missing required flag: --ignition-path")
	}
	if bootstrapInPlaceOpts.assetDir == "" {
		return errors.New("missing required flag: --asset-dir")
	}
	return nil
}
