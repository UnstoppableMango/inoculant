package inoculant

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	rbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	bootstrapNamespace = "kube-system"
	bootstrapName      = "inoculant"
)

type policyRule struct{ group, resource string }

func (i *Inoculant) applyServiceAccount(ctx context.Context) error {
	cfg := corev1.ServiceAccount(bootstrapName, bootstrapNamespace)

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

	cfg := rbacv1.ClusterRole(bootstrapName).WithRules(rules...)

	_, err = i.clientset.RbacV1().ClusterRoles().Apply(ctx, cfg, applyOpts)

	return err
}

func (i *Inoculant) rbacRules(gvks []schema.GroupVersionKind) ([]*rbacv1.PolicyRuleApplyConfiguration, error) {
	seen := map[policyRule]bool{}
	var rules []*rbacv1.PolicyRuleApplyConfiguration

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

		rules = append(rules, rbacv1.PolicyRule().
			WithAPIGroups(gvk.Group).
			WithResources(mapping.Resource.Resource).
			WithVerbs("get", "create", "patch", "update", "list")) // TODO: review
	}
	return rules, nil
}

func (i *Inoculant) applyClusterRoleBinding(ctx context.Context) error {
	cfg := rbacv1.ClusterRoleBinding(bootstrapName).
		WithRoleRef(rbacv1.RoleRef().
			WithAPIGroup("rbac.authorization.k8s.io").
			WithKind("ClusterRole").
			WithName(bootstrapName)).
		WithSubjects(rbacv1.Subject().
			WithKind("ServiceAccount").
			WithName(bootstrapName).
			WithNamespace(bootstrapNamespace))

	_, err := i.clientset.RbacV1().ClusterRoleBindings().Apply(ctx, cfg, applyOpts)
	return err
}

func writeScopedKubeconfig(cfg *rest.Config, token, outputPath string) error {
	caData := cfg.CAData
	if len(caData) == 0 && cfg.TLSClientConfig.CAFile != "" {
		var err error
		caData, err = os.ReadFile(cfg.TLSClientConfig.CAFile)
		if err != nil {
			return fmt.Errorf("read CA file: %w", err)
		}
	}

	kc := clientcmdapi.NewConfig()
	kc.Clusters[bootstrapName] = &clientcmdapi.Cluster{
		Server:                   cfg.Host,
		CertificateAuthorityData: caData,
	}
	kc.AuthInfos[bootstrapName] = &clientcmdapi.AuthInfo{Token: token}
	kc.Contexts[bootstrapName] = &clientcmdapi.Context{
		Cluster:  bootstrapName,
		AuthInfo: bootstrapName,
	}
	kc.CurrentContext = bootstrapName

	if dir := filepath.Dir(outputPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}

	return clientcmd.WriteToFile(*kc, outputPath)
}
