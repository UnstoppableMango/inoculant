package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unstoppablemango/inoculant"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Create scoped RBAC and write a token kubeconfig (runs as init container)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := restConfig()
		if err != nil {
			return err
		}

		gvks := make([]schema.GroupVersionKind, 0, len(bootstrapAllowGVKs))
		for _, s := range bootstrapAllowGVKs {
			gvk, err := parseGVK(s)
			if err != nil {
				return err
			}
			gvks = append(gvks, gvk)
		}

		return inoculant.Bootstrap(cmd.Context(), cfg, gvks, bootstrapOutput)
	},
}

// parseGVK parses GROUP/VERSION/KIND (empty group: /VERSION/KIND).
func parseGVK(s string) (schema.GroupVersionKind, error) {
	parts := strings.SplitN(s, "/", 3)
	if len(parts) != 3 {
		return schema.GroupVersionKind{}, fmt.Errorf("invalid GVK %q: want GROUP/VERSION/KIND", s)
	}
	return schema.GroupVersionKind{Group: parts[0], Version: parts[1], Kind: parts[2]}, nil
}
