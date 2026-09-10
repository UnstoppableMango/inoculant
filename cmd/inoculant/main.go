package main

import (
	"flag"

	"github.com/spf13/cobra"
	"github.com/unmango/go/cli"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var kubeconfig string

var rootCmd = &cobra.Command{
	Use:   "inoculant",
	Short: "Inoculant is a tool for bootstrapping Kubernetes resources",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig file")

	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)
	rootCmd.PersistentFlags().AddGoFlagSet(klogFlags)
}

func main() {
	err := rootCmd.Execute()
	klog.Flush()
	if err != nil {
		cli.Fail(err)
	}
}

func restConfig() (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
