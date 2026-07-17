package main

import (
	"github.com/spf13/cobra"
	"github.com/unmango/go/cli"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var kubeconfig string

var rootCmd = &cobra.Command{
	Use:   "inoculant",
	Short: "Inoculant is a tool for bootstrapping Kubernetes resources",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig file")
	rootCmd.AddCommand(applyCmd)

	bootstrapCmd.Flags().StringArrayVar(&bootstrapAllowGVKs, "allow", nil, "GVK to allow: GROUP/VERSION/KIND (repeatable; empty group: /v1/ConfigMap)")
	bootstrapCmd.Flags().StringVar(&bootstrapOutput, "output", "/scoped-kubeconfig/kubeconfig", "Path to write the scoped kubeconfig")
	rootCmd.AddCommand(bootstrapCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		cli.Fail(err)
	}
}

func restConfig() (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
