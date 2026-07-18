package main

import (
	"github.com/spf13/cobra"
	"github.com/unstoppablemango/inoculant"
)

var applyCmd = &cobra.Command{
	Use:   "apply <dir>",
	Short: "Apply static manifests from a directory to the cluster",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := restConfig()
		if err != nil {
			return err
		}

		return inoculant.Apply(cmd.Context(), args[0], cfg)
	},
}
