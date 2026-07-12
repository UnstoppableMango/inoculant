package inoculant

import (
	"bytes"
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func parseJSON(data []byte) (*unstructured.Unstructured, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: m}, nil
}

func splitYAML(data []byte) ([]*unstructured.Unstructured, error) {
	var objs []*unstructured.Unstructured
	for doc := range bytes.SplitSeq(data, []byte("\n---")) {
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
