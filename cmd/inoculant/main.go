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
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		cli.Fail(err)
	}
}

func restConfig() (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
