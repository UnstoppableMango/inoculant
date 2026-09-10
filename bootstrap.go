package inoculant

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/unstoppablemango/inoculant/internal/bootstrap"
	"github.com/unstoppablemango/inoculant/internal/client"
)

// Bootstrap creates scoped RBAC (ServiceAccount, ClusterRole, ClusterRoleBinding)
// limited to gvks and writes a token-scoped kubeconfig to output.
func Bootstrap(ctx context.Context, cfg *rest.Config, gvks []schema.GroupVersionKind, output string) error {
	c, err := client.New(cfg)
	if err != nil {
		return err
	}
	return bootstrap.New(c).Bootstrap(ctx, gvks, output)
}
