package inoculant

import (
	"context"
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
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func Apply(ctx context.Context, dir string, cfg *rest.Config) error {
	client, err := New(cfg)
	if err != nil {
		return err
	}
	return client.Apply(ctx, dir)
}

type Inoculant struct {
	mapper meta.RESTMapper
	client dynamic.Interface
}

func New(cfg *rest.Config) (*Inoculant, error) {
	mapper, client, err := newClients(cfg)
	if err != nil {
		return nil, err
	}
	return &Inoculant{
		mapper: mapper,
		client: client,
	}, nil
}

func (i *Inoculant) Apply(ctx context.Context, dir string) (err error) {
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

		return i.applyFile(ctx, path)
	})
}

func (i *Inoculant) applyFile(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	objs, err := parseManifests(data)
	if err != nil {
		return err
	}

	for _, obj := range objs {
		if err := i.applyObject(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

func (i *Inoculant) applyObject(ctx context.Context, obj *unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()
	mapping, err := i.mapper.RESTMapping(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		gvk.Version,
	)
	if err != nil {
		return err
	}

	var ri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns := obj.GetNamespace()
		if ns == "" {
			ns = "default"
		}
		ri = i.client.Resource(mapping.Resource).Namespace(ns)
	} else {
		ri = i.client.Resource(mapping.Resource)
	}

	_, err = ri.Apply(ctx,
		obj.GetName(),
		obj,
		metav1.ApplyOptions{
			FieldManager: "inoculant",
			Force:        true,
		},
	)
	return err
}

func newClients(cfg *rest.Config) (meta.RESTMapper, dynamic.Interface, error) {
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, nil, err
	}

	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		return nil, nil, err
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}

	return mapper, dynClient, nil
}
