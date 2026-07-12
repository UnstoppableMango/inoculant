package main

import (
	"github.com/spf13/cobra"
	"github.com/unmango/go/cli"
	inoculant "github.com/unstoppablemango/inoculant"
	"k8s.io/client-go/tools/clientcmd"
)

var kubeconfig string

var rootCmd = &cobra.Command{
	Use:   "inoculant",
	Short: "Inoculant is a tool for bootstrapping Kubernetes resources",
}

var applyCmd = &cobra.Command{
	Use:   "apply <dir>",
	Short: "Apply static manifests from a directory to the cluster",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return err
		}

		return inoculant.Apply(cmd.Context(), args[0], cfg)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig file")
	rootCmd.AddCommand(applyCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		cli.Fail(err)
	}
}
