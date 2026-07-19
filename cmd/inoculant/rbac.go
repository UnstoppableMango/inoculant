package main

import (
	"github.com/spf13/cobra"
	"github.com/unstoppablemango/inoculant"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var setupRBACCmd = &cobra.Command{
	Use:   "setup-rbac",
	Short: "Create scoped ClusterRole + ClusterRoleBinding for the inoculant x509 user",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := restConfig()
		if err != nil {
			return err
		}

		gvks := make([]schema.GroupVersionKind, 0, len(setupRBACAllows))
		for _, s := range setupRBACAllows {
			gvk, err := parseGVK(s)
			if err != nil {
				return err
			}
			gvks = append(gvks, gvk)
		}

		return inoculant.SetupRBAC(cmd.Context(), cfg, gvks, setupRBACUser)
	},
}
