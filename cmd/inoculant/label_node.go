package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unstoppablemango/inoculant"
)

var labelNodeLabels []string

var labelNodeCmd = &cobra.Command{
	Use:   "label-node",
	Short: "Apply labels to the Kubernetes node this pod is running on (runs as init container)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(labelNodeLabels) == 0 {
			return fmt.Errorf("--label is required: label-node with no labels does nothing")
		}

		nodeName := os.Getenv("NODE_NAME")
		if nodeName == "" {
			return fmt.Errorf("NODE_NAME environment variable is required (expected to be set via the downward API to spec.nodeName)")
		}

		cfg, err := restConfig()
		if err != nil {
			return err
		}

		labels := make(map[string]string, len(labelNodeLabels))
		for _, s := range labelNodeLabels {
			k, v, err := parseLabel(s)
			if err != nil {
				return err
			}
			labels[k] = v
		}

		return inoculant.Labels(cmd.Context(), cfg, nodeName, labels)
	},
}

// parseLabel parses key=value.
func parseLabel(s string) (string, string, error) {
	k, v, ok := strings.Cut(s, "=")
	if !ok || k == "" {
		return "", "", fmt.Errorf("invalid label %q: want key=value", s)
	}
	return k, v, nil
}

func init() {
	rootCmd.AddCommand(labelNodeCmd)
	labelNodeCmd.Flags().StringArrayVar(&labelNodeLabels, "label", nil, "Label to apply to the node: key=value (repeatable)")
}
