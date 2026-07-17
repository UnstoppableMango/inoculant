package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unmango/go/cli"
	inoculant "github.com/unstoppablemango/inoculant"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
)

var kubeconfig string
var allowedGVKs []string

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

		opts := inoculant.Options{}
		for _, s := range allowedGVKs {
			gvk, err := parseGVK(s)
			if err != nil {
				return err
			}
			opts.AllowedGVKs = append(opts.AllowedGVKs, gvk)
		}

		return inoculant.Apply(cmd.Context(), args[0], cfg, opts)
	},
}

// parseGVK parses [group/]version/Kind. Core group resources omit the group: v1/ConfigMap.
func parseGVK(s string) (schema.GroupVersionKind, error) {
	parts := strings.Split(s, "/")
	switch len(parts) {
	case 2:
		return schema.GroupVersionKind{Group: "", Version: parts[0], Kind: parts[1]}, nil
	case 3:
		return schema.GroupVersionKind{Group: parts[0], Version: parts[1], Kind: parts[2]}, nil
	default:
		return schema.GroupVersionKind{}, fmt.Errorf("invalid GVK %q: expected [group/]version/Kind", s)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig file")
	applyCmd.Flags().StringArrayVar(&allowedGVKs, "allowed-gvk", nil, "Permitted GVK ([group/]version/Kind); repeatable; empty = allow all")
	rootCmd.AddCommand(applyCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		cli.Fail(err)
	}
}
