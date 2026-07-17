package inoculant

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

const rbacName = "inoculant"

// SetupRBAC creates a ClusterRole scoped to the given GVKs and a ClusterRoleBinding
// for the named x509 user. Uses SSA — safe to run multiple times.
func SetupRBAC(ctx context.Context, cfg *rest.Config, gvks []schema.GroupVersionKind, userName string) error {
	mapper, dynClient, err := newClients(cfg)
	if err != nil {
		return err
	}

	rules, err := rbacRules(gvks, mapper)
	if err != nil {
		return err
	}

	if err := applyClusterRole(ctx, dynClient, rules); err != nil {
		return err
	}
	return applyUserClusterRoleBinding(ctx, dynClient, userName)
}

func applyClusterRole(ctx context.Context, dynClient dynamic.Interface, rules []any) error {
	cr := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata": map[string]any{
			"name": rbacName,
		},
		"rules": rules,
	}}
	_, err := dynClient.Resource(crGVR).Apply(
		ctx, rbacName, cr, metav1.ApplyOptions{FieldManager: "inoculant-setup-rbac", Force: true},
	)
	return err
}

func applyUserClusterRoleBinding(ctx context.Context, dynClient dynamic.Interface, userName string) error {
	crb := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"metadata": map[string]any{
			"name": rbacName,
		},
		"roleRef": map[string]any{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "ClusterRole",
			"name":     rbacName,
		},
		"subjects": []any{
			map[string]any{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "User",
				"name":     userName,
			},
		},
	}}
	_, err := dynClient.Resource(crbGVR).Apply(
		ctx, rbacName, crb, metav1.ApplyOptions{FieldManager: "inoculant-setup-rbac", Force: true},
	)
	return err
}
