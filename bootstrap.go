package inoculant

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// bootstrapTokenTTLSeconds is how long the token written to the scoped
	// kubeconfig is valid for.
	bootstrapTokenTTLSeconds int64 = 3600
)

type policyRule struct{ group, resource string }

// Bootstrap creates scoped RBAC (ServiceAccount, ClusterRole, ClusterRoleBinding)
// limited to gvks and writes a token-scoped kubeconfig to output.
func Bootstrap(ctx context.Context, cfg *rest.Config, gvks []schema.GroupVersionKind, output string) error {
	client, err := New(cfg)
	if err != nil {
		return err
	}
	return client.Bootstrap(ctx, gvks, output)
}

// Bootstrap creates scoped RBAC limited to gvks and writes a token-scoped
// kubeconfig to outputPath.
func (c *Client) Bootstrap(ctx context.Context, gvks []schema.GroupVersionKind, outputPath string) error {
	if err := c.applyServiceAccount(ctx); err != nil {
		return err
	}
	if err := c.applyClusterRole(ctx, gvks); err != nil {
		return err
	}
	if err := c.applyClusterRoleBinding(ctx); err != nil {
		return err
	}

	ttl := bootstrapTokenTTLSeconds
	tokenResp, err := c.clientset.CoreV1().ServiceAccounts(bootstrapNamespace).CreateToken(
		ctx,
		bootstrapName,
		&authv1.TokenRequest{
			Spec: authv1.TokenRequestSpec{ExpirationSeconds: &ttl},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return fmt.Errorf("create token: %w", err)
	}
	return writeScopedKubeconfig(c.cfg, tokenResp.Status.Token, outputPath)
}

func (c *Client) applyServiceAccount(ctx context.Context) error {
	cfg := corev1.ServiceAccount(bootstrapName, bootstrapNamespace)

	_, err := c.clientset.CoreV1().
		ServiceAccounts(bootstrapNamespace).
		Apply(ctx, cfg, applyOpts)

	return err
}

func (c *Client) applyClusterRole(ctx context.Context, gvks []schema.GroupVersionKind) error {
	rules, err := c.rbacRules(gvks)
	if err != nil {
		return err
	}

	cfg := rbacv1.ClusterRole(bootstrapName).WithRules(rules...)

	_, err = c.clientset.RbacV1().ClusterRoles().Apply(ctx, cfg, applyOpts)

	return err
}

func (c *Client) rbacRules(gvks []schema.GroupVersionKind) ([]*rbacv1.PolicyRuleApplyConfiguration, error) {
	seen := map[policyRule]bool{}
	var rules []*rbacv1.PolicyRuleApplyConfiguration

	for _, gvk := range gvks {
		mapping, err := c.mapper.RESTMapping(
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
			// TODO: confirm minimal verb set per resource (currently get/create/patch/update/list for all)
			WithVerbs("get", "create", "patch", "update", "list"))
	}
	return rules, nil
}

func (c *Client) applyClusterRoleBinding(ctx context.Context) error {
	cfg := rbacv1.ClusterRoleBinding(bootstrapName).
		WithRoleRef(rbacv1.RoleRef().
			WithAPIGroup("rbac.authorization.k8s.io").
			WithKind("ClusterRole").
			WithName(bootstrapName)).
		WithSubjects(rbacv1.Subject().
			WithKind("ServiceAccount").
			WithName(bootstrapName).
			WithNamespace(bootstrapNamespace))

	_, err := c.clientset.RbacV1().ClusterRoleBindings().Apply(ctx, cfg, applyOpts)
	return err
}

// writeScopedKubeconfig writes a kubeconfig authenticated with token to outputPath.
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

	if err := clientcmd.WriteToFile(*kc, outputPath); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}
	return nil
}
