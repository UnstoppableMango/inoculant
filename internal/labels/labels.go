// Package labels server-side applies labels to a single Node.
package labels

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"

	"github.com/unstoppablemango/inoculant/internal/client"
)

var nodeGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}

// Labeler server-side applies node labels using c.
type Labeler struct {
	c *client.Client
}

// New builds a Labeler backed by c.
func New(c *client.Client) *Labeler {
	return &Labeler{c: c}
}

// Apply server-side applies labels onto the Node named nodeName.
func (l *Labeler) Apply(ctx context.Context, nodeName string, labels map[string]string) error {
	mapping, err := l.c.Mapper.RESTMapping(
		schema.GroupKind{Group: nodeGVK.Group, Kind: nodeGVK.Kind},
		nodeGVK.Version,
	)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", nodeGVK, err)
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(nodeGVK)
	obj.SetName(nodeName)
	obj.SetLabels(labels)

	klog.InfoS("applying node labels", "node", nodeName, "labels", labels)

	ri := l.c.Dynamic.Resource(mapping.Resource)
	if _, err := ri.Apply(ctx, nodeName, obj, client.ApplyOptions()); err != nil {
		return fmt.Errorf("apply labels on node %s: %w", nodeName, err)
	}
	return nil
}
