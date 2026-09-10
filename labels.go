package inoculant

import (
	"context"

	"k8s.io/client-go/rest"

	"github.com/unstoppablemango/inoculant/internal/client"
	"github.com/unstoppablemango/inoculant/internal/labels"
)

// Labels server-side applies nodeLabels onto the Node named nodeName.
func Labels(ctx context.Context, cfg *rest.Config, nodeName string, nodeLabels map[string]string) error {
	c, err := client.New(cfg)
	if err != nil {
		return err
	}
	return labels.New(c).Apply(ctx, nodeName, nodeLabels)
}
