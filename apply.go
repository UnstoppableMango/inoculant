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

// Options configures Apply behavior.
type Options struct {
	// AllowedGVKs restricts which GroupVersionKinds inoculant will apply.
	// Empty slice means all GVKs are permitted.
	AllowedGVKs []schema.GroupVersionKind
}

func Apply(ctx context.Context, dir string, cfg *rest.Config, opts Options) error {
	mapper, dynClient, err := newClients(cfg)
	if err != nil {
		return err
	}

	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return err
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

		return applyFile(ctx, path, ext, opts, mapper, dynClient)
	})
}

func applyFile(ctx context.Context, path, ext string, opts Options, mapper meta.RESTMapper, dynClient dynamic.Interface) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var objs []*unstructured.Unstructured
	if ext == ".json" {
		obj, err := parseJSON(data)
		if err != nil {
			return err
		}
		objs = []*unstructured.Unstructured{obj}
	} else {
		objs, err = splitYAML(data)
		if err != nil {
			return err
		}
	}

	for _, obj := range objs {
		if err := applyObject(ctx, obj, opts, mapper, dynClient); err != nil {
			return err
		}
	}
	return nil
}

func applyObject(ctx context.Context, obj *unstructured.Unstructured, opts Options, mapper meta.RESTMapper, dynClient dynamic.Interface) error {
	gvk := obj.GroupVersionKind()

	if len(opts.AllowedGVKs) > 0 && !gvkAllowed(gvk, opts.AllowedGVKs) {
		return fmt.Errorf("GVK %s not in allowed list", gvk)
	}

	mapping, err := mapper.RESTMapping(schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
	if err != nil {
		return err
	}

	var ri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns := obj.GetNamespace()
		if ns == "" {
			ns = "default"
		}
		ri = dynClient.Resource(mapping.Resource).Namespace(ns)
	} else {
		ri = dynClient.Resource(mapping.Resource)
	}

	_, err = ri.Apply(ctx, obj.GetName(), obj, metav1.ApplyOptions{FieldManager: "inoculant", Force: true})
	return err
}

func gvkAllowed(gvk schema.GroupVersionKind, allowed []schema.GroupVersionKind) bool {
	for _, a := range allowed {
		if a == gvk {
			return true
		}
	}
	return false
}
