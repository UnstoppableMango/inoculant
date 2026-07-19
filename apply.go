package inoculant

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
	"k8s.io/client-go/rest"
)

// Apply server-side applies every YAML/JSON manifest found under dir to the cluster.
func Apply(ctx context.Context, dir string, cfg *rest.Config) error {
	client, err := New(cfg)
	if err != nil {
		return err
	}
	return client.Apply(ctx, dir)
}

// Apply walks dir and server-side applies each YAML/JSON manifest it finds.
func (c *Client) Apply(ctx context.Context, dir string) error {
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dir, err)
	}

	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}

		return c.applyFile(ctx, path)
	})
}

func (c *Client) applyFile(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	objs, err := parseManifests(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	for _, obj := range objs {
		if err := c.applyObject(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyObject(ctx context.Context, obj *unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()
	mapping, err := c.mapper.RESTMapping(
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
		ri = c.client.Resource(mapping.Resource).Namespace(ns)
	} else {
		ri = c.client.Resource(mapping.Resource)
	}

	if _, err := ri.Apply(ctx, obj.GetName(), obj, applyOpts); err != nil {
		return fmt.Errorf("apply %s %s/%s: %w", gvk.Kind, obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}
