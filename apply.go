package inoculant

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	metav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	rbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
)

func (i *Inoculant) applyServiceAccount(ctx context.Context) error {
	cfg := &corev1.ServiceAccountApplyConfiguration{
		ObjectMetaApplyConfiguration: &metav1.ObjectMetaApplyConfiguration{
			Name:      new(bootstrapName),
			Namespace: new(bootstrapNamespace),
		},
	}

	_, err := i.clientset.CoreV1().
		ServiceAccounts(bootstrapNamespace).
		Apply(ctx, cfg, applyOpts)

	return err
}

func (i *Inoculant) applyClusterRole(ctx context.Context, gvks []schema.GroupVersionKind) error {
	rules, err := i.rbacRules(gvks)
	if err != nil {
		return err
	}

	cfg := &rbacv1.ClusterRoleApplyConfiguration{
		ObjectMetaApplyConfiguration: &metav1.ObjectMetaApplyConfiguration{
			Name: new(bootstrapName),
		},
		Rules: rules,
	}

	_, err = i.clientset.RbacV1().ClusterRoles().Apply(ctx, cfg, applyOpts)

	return err
}

func (i *Inoculant) rbacRules(gvks []schema.GroupVersionKind) ([]rbacv1.PolicyRuleApplyConfiguration, error) {
	seen := map[policyRule]bool{}
	var rules []rbacv1.PolicyRuleApplyConfiguration

	for _, gvk := range gvks {
		mapping, err := i.mapper.RESTMapping(
			schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
			gvk.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", gvk, err)
		}

		pr := policyRule{gvk.Group, mapping.Resource.Resource}
		if seen[pr] {
			continue
		}
		seen[pr] = true

		rules = append(rules, rbacv1.PolicyRuleApplyConfiguration{
			APIGroups: []string{gvk.Group},
			Resources: []string{mapping.Resource.Resource},
			Verbs:     []string{"get", "create", "patch", "update", "list"}, // TODO: review
		})
	}
	return rules, nil
}

func (i *Inoculant) applyClusterRoleBinding(ctx context.Context) error {
	cfg := &rbacv1.ClusterRoleBindingApplyConfiguration{
		ObjectMetaApplyConfiguration: &metav1.ObjectMetaApplyConfiguration{
			Name: new(bootstrapName),
		},
		RoleRef: &rbacv1.RoleRefApplyConfiguration{
			APIGroup: new("rbac.authorization.k8s.io"),
			Kind:     new("ClusterRole"),
			Name:     new(bootstrapName),
		},
		Subjects: []rbacv1.SubjectApplyConfiguration{{
			APIGroup:  new("rbac.authorization.k8s.io"),
			Kind:      new("ServiceAccount"),
			Name:      new(bootstrapName),
			Namespace: new(bootstrapNamespace),
		}},
	}

	_, err := i.clientset.RbacV1().ClusterRoleBindings().Apply(ctx, cfg, applyOpts)
	return err
}
