// Package bootstrap creates scoped RBAC for inoculant and writes a
// token-scoped kubeconfig limited to it.
package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	rbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"

	"github.com/unstoppablemango/inoculant/internal/client"
)

const (
	namespace = "kube-system"
	name      = "inoculant"

	// tokenTTLSeconds is how long the token written to the scoped
	// kubeconfig is valid for.
	tokenTTLSeconds int64 = 3600
)

type policyRule struct{ group, resource string }

// Bootstrapper creates scoped RBAC and a token kubeconfig using c.
type Bootstrapper struct {
	c *client.Client
}

// New builds a Bootstrapper backed by c.
func New(c *client.Client) *Bootstrapper {
	return &Bootstrapper{c: c}
}

// Bootstrap creates scoped RBAC limited to gvks and writes a token-scoped
// kubeconfig to outputPath.
func (b *Bootstrapper) Bootstrap(ctx context.Context, gvks []schema.GroupVersionKind, outputPath string) error {
	if err := b.applyServiceAccount(ctx); err != nil {
		return err
	}
	if err := b.applyClusterRole(ctx, gvks); err != nil {
		return err
	}
	if err := b.applyClusterRoleBinding(ctx); err != nil {
		return err
	}

	ttl := tokenTTLSeconds
	klog.InfoS("requesting service account token", "namespace", namespace, "name", name, "ttlSeconds", ttl)
	tokenResp, err := b.c.Clientset.CoreV1().ServiceAccounts(namespace).CreateToken(
		ctx,
		name,
		&authv1.TokenRequest{
			Spec: authv1.TokenRequestSpec{
				ExpirationSeconds: &ttl,
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return fmt.Errorf("create token: %w", err)
	}
	return writeScopedKubeconfig(b.c.Cfg, tokenResp.Status.Token, outputPath)
}

func (b *Bootstrapper) applyServiceAccount(ctx context.Context) error {
	cfg := corev1.ServiceAccount(name, namespace)

	klog.InfoS("applying service account", "namespace", namespace, "name", name)
	_, err := b.c.Clientset.CoreV1().
		ServiceAccounts(namespace).
		Apply(ctx, cfg, client.ApplyOptions())

	return err
}

func (b *Bootstrapper) applyClusterRole(ctx context.Context, gvks []schema.GroupVersionKind) error {
	rules, err := b.rbacRules(gvks)
	if err != nil {
		return err
	}

	cfg := rbacv1.ClusterRole(name).WithRules(rules...)

	klog.InfoS("applying cluster role", "name", name, "rules", len(rules))
	_, err = b.c.Clientset.RbacV1().ClusterRoles().Apply(ctx, cfg, client.ApplyOptions())

	return err
}

func (b *Bootstrapper) rbacRules(gvks []schema.GroupVersionKind) ([]*rbacv1.PolicyRuleApplyConfiguration, error) {
	seen := map[policyRule]bool{}
	var rules []*rbacv1.PolicyRuleApplyConfiguration

	for _, gvk := range gvks {
		resource, err := b.resolveResource(gvk)
		if err != nil {
			return nil, err
		}

		pr := policyRule{gvk.Group, resource}
		if seen[pr] {
			continue
		}
		seen[pr] = true

		verbs := []string{"get", "create", "patch", "update", "list"}
		// Kubernetes' RBAC escalation check is keyed on the referenced
		// Role/ClusterRole, not the Binding: creating a Role/ClusterRole
		// with rules the applier doesn't hold requires "escalate" on
		// roles/clusterroles, and creating a (Cluster)RoleBinding that
		// references one requires "bind" on that same roles/clusterroles
		// resource, not on role bindings.
		if resource == "clusterroles" || resource == "roles" {
			verbs = append(verbs, "escalate", "bind")
		}

		rules = append(rules, rbacv1.PolicyRule().
			WithAPIGroups(gvk.Group).
			WithResources(resource).
			WithVerbs(verbs...))
	}
	return rules, nil
}

// resolveResource maps gvk to its plural resource name. It prefers the
// cluster's live discovery mapping, which is authoritative for anything
// already registered. If the kind isn't registered yet, it falls back to
// apimachinery's heuristic Kind->resource guess: bootstrap runs before
// apply, so a GVK it's asked to scope RBAC for (e.g. a CRD inoculant itself
// is about to install) may not exist on the cluster yet.
func (b *Bootstrapper) resolveResource(gvk schema.GroupVersionKind) (string, error) {
	mapping, err := b.c.Mapper.RESTMapping(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		gvk.Version,
	)
	if err == nil {
		return mapping.Resource.Resource, nil
	}
	if !meta.IsNoMatchError(err) {
		return "", fmt.Errorf("resolve %s: %w", gvk, err)
	}

	klog.InfoS("kind not yet registered, guessing resource name", "gvk", gvk)
	guessed, _ := meta.UnsafeGuessKindToResource(gvk)
	return guessed.Resource, nil
}

func (b *Bootstrapper) applyClusterRoleBinding(ctx context.Context) error {
	cfg := rbacv1.ClusterRoleBinding(name).
		WithRoleRef(rbacv1.RoleRef().
			WithAPIGroup("rbac.authorization.k8s.io").
			WithKind("ClusterRole").
			WithName(name)).
		WithSubjects(rbacv1.Subject().
			WithKind("ServiceAccount").
			WithName(name).
			WithNamespace(namespace))

	klog.InfoS("applying cluster role binding", "name", name)
	_, err := b.c.Clientset.RbacV1().ClusterRoleBindings().Apply(ctx, cfg, client.ApplyOptions())
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
	kc.Clusters[name] = &clientcmdapi.Cluster{
		Server:                   cfg.Host,
		CertificateAuthorityData: caData,
		InsecureSkipTLSVerify:    cfg.TLSClientConfig.Insecure,
		TLSServerName:            cfg.TLSClientConfig.ServerName,
	}
	kc.AuthInfos[name] = &clientcmdapi.AuthInfo{Token: token}
	kc.Contexts[name] = &clientcmdapi.Context{
		Cluster:  name,
		AuthInfo: name,
	}
	kc.CurrentContext = name

	if dir := filepath.Dir(outputPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}

	klog.InfoS("writing scoped kubeconfig", "path", outputPath)
	if err := clientcmd.WriteToFile(*kc, outputPath); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}
	return nil
}
