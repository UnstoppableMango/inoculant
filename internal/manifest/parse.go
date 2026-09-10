// Package manifest decodes YAML/JSON Kubernetes manifests into unstructured
// objects.
package manifest

import (
	"bytes"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Parse decodes a stream of YAML or JSON documents into unstructured
// objects, matching kubectl's apply semantics.
func Parse(data []byte) ([]*unstructured.Unstructured, error) {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)

	var objs []*unstructured.Unstructured
	for {
		obj := &unstructured.Unstructured{}
		if err := dec.Decode(obj); err != nil {
			if err == io.EOF {
				return objs, nil
			}
			return nil, fmt.Errorf("decode manifest: %w", err)
		}
		if len(obj.Object) == 0 {
			continue
		}
		objs = append(objs, obj)
	}
}
