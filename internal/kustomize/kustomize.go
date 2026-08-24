// Package kustomize renders kustomize overlay directories into unstructured
// objects, matching the input format the apply package expects from plain
// manifests.
package kustomize

import (
	"fmt"
	"path/filepath"

	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/unstoppablemango/inoculant/internal/manifest"
)

// IsRoot reports whether dir contains a kustomization file recognized by
// kustomize (kustomization.yaml, kustomization.yml, or Kustomization).
func IsRoot(fSys filesys.FileSystem, dir string) bool {
	for _, name := range konfig.RecognizedKustomizationFileNames() {
		if fSys.Exists(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

// Build renders the kustomize overlay rooted at dir into unstructured
// objects.
func Build(dir string) ([]*unstructured.Unstructured, error) {
	fSys := filesys.MakeFsOnDisk()
	k := krusty.MakeKustomizer(krusty.MakeDefaultOptions())

	resMap, err := k.Run(fSys, dir)
	if err != nil {
		return nil, fmt.Errorf("build kustomization %s: %w", dir, err)
	}

	yml, err := resMap.AsYaml()
	if err != nil {
		return nil, fmt.Errorf("render kustomization %s: %w", dir, err)
	}

	return manifest.Parse(yml)
}
