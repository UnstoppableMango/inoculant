package inoculant

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"
)

func Apply(ctx context.Context, dir string, cfg *rest.Config) error {
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return err
	}

	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		return err
	}

	dynClient, err := dynamic.NewForConfig(cfg)
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
			gvk := obj.GroupVersionKind()
			mapping, err := mapper.RESTMapping(schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
			if err != nil {
				return err
			}

			var ri dynamic.ResourceInterface
			if mapping.Scope.Name() == "namespace" {
				ns := obj.GetNamespace()
				if ns == "" {
					ns = "default"
				}
				ri = dynClient.Resource(mapping.Resource).Namespace(ns)
			} else {
				ri = dynClient.Resource(mapping.Resource)
			}

			if _, err := ri.Apply(ctx, obj.GetName(), obj, metav1.ApplyOptions{FieldManager: "inoculant"}); err != nil {
				return err
			}
		}
		return nil
	})
}

func parseJSON(data []byte) (*unstructured.Unstructured, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: m}, nil
}

func splitYAML(data []byte) ([]*unstructured.Unstructured, error) {
	var objs []*unstructured.Unstructured
	for _, doc := range bytes.Split(data, []byte("\n---")) {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}
		jsonBytes, err := yaml.YAMLToJSON(doc)
		if err != nil {
			return nil, err
		}
		if string(jsonBytes) == "null" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(jsonBytes, &m); err != nil {
			return nil, err
		}
		objs = append(objs, &unstructured.Unstructured{Object: m})
	}
	return objs, nil
}
