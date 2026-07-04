package main

import (
	"github.com/spf13/cobra"
	"github.com/unmango/go/cli"
)

var rootCmd = &cobra.Command{
	Use:   "inoculant",
	Short: "Inoculant is a tool for bootstrapping Kubernetes resources",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		cli.Fail(err)
	}
}
