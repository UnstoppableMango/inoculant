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
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/unstoppablemango/inoculant/internal/client"
	"github.com/unstoppablemango/inoculant/internal/helm"
	"github.com/unstoppablemango/inoculant/internal/kustomize"
	"github.com/unstoppablemango/inoculant/internal/manifest"
)

// managedByLabel marks every object inoculant applies, so a prune pass can
// find candidates for deletion without needing separate tracking state.
// Helm chart directories (see internal/helm) are deliberately excluded:
// Helm tracks its own release lifecycle via Secret-based storage and never
// stamps this label, so released objects are never prune candidates.
const (
	managedByLabel = "inoculant.unmango.dev/managed-by"
	managedByValue = "inoculant"
)

// objectKey identifies a live cluster object for desired/actual set diffing.
type objectKey struct {
	schema.GroupVersionResource
	Namespace string
	Name      string
}

// Applier server-side applies manifest directories using c.
type Applier struct {
	c *client.Client
}

// New builds an Applier backed by c.
func New(c *client.Client) *Applier {
	return &Applier{c: c}
}

// Apply walks dir and server-side applies each YAML/JSON manifest it finds,
// then prunes any previously-applied object no longer present in dir.
func (a *Applier) Apply(ctx context.Context, dir string) error {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dir, err)
	}

	klog.InfoS("applying manifests", "dir", resolved)

	fSys := filesys.MakeFsOnDisk()
	desired := map[objectKey]struct{}{}
	if err := filepath.WalkDir(resolved, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if kustomize.IsRoot(fSys, path) {
				if err := a.applyKustomization(ctx, path, desired); err != nil {
					return err
				}
				return filepath.SkipDir
			}
			if helm.IsRoot(fSys, path) {
				if err := a.applyHelmChart(ctx, path); err != nil {
					return err
				}
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".yaml", ".yml", ".json":
			return a.applyFile(ctx, path, desired)
		default:
			klog.V(1).InfoS("skipping non-manifest file", "path", path)
		}
		return nil
	}); err != nil {
		return err
	}

	return a.prune(ctx, desired)
}

func (a *Applier) applyFile(ctx context.Context, path string, desired map[objectKey]struct{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	objs, err := manifest.Parse(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	return a.applyObjects(ctx, objs, desired)
}

func (a *Applier) applyKustomization(ctx context.Context, dir string, desired map[objectKey]struct{}) error {
	klog.InfoS("building kustomization", "dir", dir)

	objs, err := kustomize.Build(dir)
	if err != nil {
		return err
	}

	return a.applyObjects(ctx, objs, desired)
}

// applyHelmChart installs or upgrades the Helm release rooted at dir.
// Unlike applyFile/applyKustomization, this does not populate desired:
// Helm tracks its own release lifecycle via Secret-based storage
// (internal/helm), so its objects never carry managedByLabel and are
// never prune candidates.
func (a *Applier) applyHelmChart(ctx context.Context, dir string) error {
	rel, err := helm.Install(ctx, a.c, dir, metav1.NamespaceDefault)
	if err != nil {
		return fmt.Errorf("apply helm chart %s: %w", dir, err)
	}

	klog.InfoS("helm release applied",
		"name", rel.Name, "namespace", rel.Namespace,
		"revision", rel.Version, "status", rel.Info.Status)
	return nil
}

func (a *Applier) applyObjects(ctx context.Context, objs []*unstructured.Unstructured, desired map[objectKey]struct{}) error {
	for _, obj := range objs {
		if err := a.applyObject(ctx, obj, desired); err != nil {
			return err
		}
	}
	return nil
}

func (a *Applier) applyObject(ctx context.Context, obj *unstructured.Unstructured, desired map[objectKey]struct{}) error {
	gvk := obj.GroupVersionKind()
	mapping, err := a.c.Mapper.RESTMapping(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		gvk.Version,
	)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", gvk, err)
	}

	ns := obj.GetNamespace()
	var ri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if ns == "" {
			ns = metav1.NamespaceDefault
			obj.SetNamespace(ns)
		}
		ri = a.c.Dynamic.Resource(mapping.Resource).Namespace(ns)
	} else {
		ri = a.c.Dynamic.Resource(mapping.Resource)
	}

	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[managedByLabel] = managedByValue
	obj.SetLabels(labels)

	klog.InfoS("applying object", "kind", gvk.Kind, "namespace", ns, "name", obj.GetName())

	if _, err := ri.Apply(ctx, obj.GetName(), obj, client.ApplyOptions()); err != nil {
		return fmt.Errorf("apply %s %s/%s: %w", gvk.Kind, ns, obj.GetName(), err)
	}

	desired[objectKey{GroupVersionResource: mapping.Resource, Namespace: ns, Name: obj.GetName()}] = struct{}{}
	return nil
}
