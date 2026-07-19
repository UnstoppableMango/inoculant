package inoculant

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var applyOpts = metav1.ApplyOptions{
	FieldManager: "inoculant",
	Force:        true, // TODO: review usage
}

func Apply(ctx context.Context, dir string, cfg *rest.Config) error {
	client, err := New(cfg)
	if err != nil {
		return err
	}
	return client.Apply(ctx, dir)
}

type Inoculant struct {
	mapper    meta.RESTMapper
	client    dynamic.Interface
	clientset *kubernetes.Clientset
}

func New(cfg *rest.Config) (*Inoculant, error) {
	http, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, err
	}

	c := &Inoculant{}
	if c.mapper, err = apiutil.NewDynamicRESTMapper(cfg, http); err != nil {
		return nil, err
	}
	if c.client, err = dynamic.NewForConfigAndClient(cfg, http); err != nil {
		return nil, err
	}
	if c.clientset, err = kubernetes.NewForConfigAndClient(cfg, http); err != nil {
		return nil, err
	}
	return c, nil
}

func (i *Inoculant) Apply(ctx context.Context, dir string) error {
	dir, err := filepath.EvalSymlinks(dir)
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

func (i *Inoculant) Bootstrap(ctx context.Context, gvks []schema.GroupVersionKind, outputPath string) error {
	if err := i.applyServiceAccount(ctx); err != nil {
		return err
	}
	if err := i.applyClusterRole(ctx, gvks); err != nil {
		return err
	}
	if err := i.applyClusterRoleBinding(ctx); err != nil {
		return err
	}

	dur := int64(3600)
	tokenResp, err := i.clientset.CoreV1().ServiceAccounts(bootstrapNamespace).CreateToken(
		ctx,
		bootstrapName,
		&authv1.TokenRequest{
			Spec: authv1.TokenRequestSpec{ExpirationSeconds: &dur},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return fmt.Errorf("create token: %w", err)
	}
	fmt.Println("got token", tokenResp.Status.Token)

	return nil
	// return writeScopedKubeconfig(cfg, tokenResp.Status.Token, outputPath)
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
