// Package apply server-side applies YAML/JSON manifest directories to a
// cluster.
package apply

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/unstoppablemango/inoculant/internal/client"
	"github.com/unstoppablemango/inoculant/internal/manifest"
)

// Applier server-side applies manifest directories using c.
type Applier struct {
	c *client.Client
}

// New builds an Applier backed by c.
func New(c *client.Client) *Applier {
	return &Applier{c: c}
}

// Apply walks dir and server-side applies each YAML/JSON manifest it finds.
func (a *Applier) Apply(ctx context.Context, dir string) error {
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dir, err)
	}

	klog.InfoS("applying manifests", "dir", dir)

	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			klog.V(1).InfoS("skipping non-manifest file", "path", path)
			return nil
		}

		return a.applyFile(ctx, path)
	})
}

func (a *Applier) applyFile(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	objs, err := manifest.Parse(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	for _, obj := range objs {
		if err := a.applyObject(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

func (a *Applier) applyObject(ctx context.Context, obj *unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()
	mapping, err := a.c.Mapper.RESTMapping(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		gvk.Version,
	)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", gvk, err)
	}

	var ri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns := obj.GetNamespace()
		if ns == "" {
			ns = metav1.NamespaceDefault
		}
		ri = a.c.Dynamic.Resource(mapping.Resource).Namespace(ns)
	} else {
		ri = a.c.Dynamic.Resource(mapping.Resource)
	}

	klog.InfoS("applying object", "kind", gvk.Kind, "namespace", obj.GetNamespace(), "name", obj.GetName())

	if _, err := ri.Apply(ctx, obj.GetName(), obj, client.ApplyOptions); err != nil {
		return fmt.Errorf("apply %s %s/%s: %w", gvk.Kind, obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}
