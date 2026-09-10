// Package inoculant bootstraps Kubernetes clusters by applying static
// resources at node initialization time.
package inoculant

import (
	"context"

	"k8s.io/client-go/rest"

	"github.com/unstoppablemango/inoculant/internal/apply"
	"github.com/unstoppablemango/inoculant/internal/client"
)

// Apply server-side applies every YAML/JSON manifest found under dir to the cluster.
func Apply(ctx context.Context, dir string, cfg *rest.Config) error {
	c, err := client.New(cfg)
	if err != nil {
		return err
	}
	return apply.New(c).Apply(ctx, dir)
}
