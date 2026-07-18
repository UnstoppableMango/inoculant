package inoculant

import (
	"context"
	"fmt"
	"os"

	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	bootstrapNamespace = "kube-system"
	bootstrapName      = "inoculant"
)

var (
	saGVR  = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}
	crGVR  = schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}
	crbGVR = schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}
)

// Bootstrap creates a ServiceAccount, ClusterRole, and ClusterRoleBinding scoped
// to the given GVKs, then writes a token-based kubeconfig to outputPath.
// Intended to run as an init container using the cluster-admin credential.
func Bootstrap(ctx context.Context, cfg *rest.Config, gvks []schema.GroupVersionKind, outputPath string) error {
	mapper, dynClient, err := newClients(cfg)
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}

	rules, err := rbacRules(gvks, mapper)
	if err != nil {
		return err
	}

	if err := applyBootstrapServiceAccount(ctx, dynClient); err != nil {
		return err
	}
	if err := applyBootstrapClusterRole(ctx, dynClient, rules); err != nil {
		return err
	}
	if err := applyBootstrapClusterRoleBinding(ctx, dynClient); err != nil {
		return err
	}

	dur := int64(3600)
	tokenResp, err := clientset.CoreV1().ServiceAccounts(bootstrapNamespace).CreateToken(
		ctx,
		bootstrapName,
		&authv1.TokenRequest{
			Spec: authv1.TokenRequestSpec{ExpirationSeconds: &dur},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return fmt.Errorf("create token: %w", err)
	}

	return writeScopedKubeconfig(cfg, tokenResp.Status.Token, outputPath)
}

type policyRule struct{ group, resource string }

func rbacRules(gvks []schema.GroupVersionKind, mapper meta.RESTMapper) ([]any, error) {
	seen := map[policyRule]bool{}
	var rules []any

	for _, gvk := range gvks {
		mapping, err := mapper.RESTMapping(
			schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
			gvk.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("resolve %s/%s/%s: %w", gvk.Group, gvk.Version, gvk.Kind, err)
		}
		pr := policyRule{gvk.Group, mapping.Resource.Resource}
		if seen[pr] {
			continue
		}
		seen[pr] = true
		rules = append(rules, map[string]any{
			"apiGroups": []any{gvk.Group},
			"resources": []any{mapping.Resource.Resource},
			"verbs":     []any{"get", "create", "patch", "update", "list"},
		})
	}
	return rules, nil
}

func applyBootstrapServiceAccount(ctx context.Context, dynClient dynamic.Interface) error {
	sa := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata": map[string]any{
			"name":      bootstrapName,
			"namespace": bootstrapNamespace,
		},
	}}
	_, err := dynClient.Resource(saGVR).Namespace(bootstrapNamespace).Apply(
		ctx, bootstrapName, sa, metav1.ApplyOptions{FieldManager: "inoculant-bootstrap", Force: true},
	)
	return err
}

func applyBootstrapClusterRole(ctx context.Context, dynClient dynamic.Interface, rules []any) error {
	cr := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata": map[string]any{
			"name": bootstrapName,
		},
		"rules": rules,
	}}
	_, err := dynClient.Resource(crGVR).Apply(
		ctx, bootstrapName, cr, metav1.ApplyOptions{FieldManager: "inoculant-bootstrap", Force: true},
	)
	return err
}

func applyBootstrapClusterRoleBinding(ctx context.Context, dynClient dynamic.Interface) error {
	crb := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"metadata": map[string]any{
			"name": bootstrapName,
		},
		"roleRef": map[string]any{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "ClusterRole",
			"name":     bootstrapName,
		},
		"subjects": []any{
			map[string]any{
				"kind":      "ServiceAccount",
				"name":      bootstrapName,
				"namespace": bootstrapNamespace,
			},
		},
	}}
	_, err := dynClient.Resource(crbGVR).Apply(
		ctx, bootstrapName, crb, metav1.ApplyOptions{FieldManager: "inoculant-bootstrap", Force: true},
	)
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

	return clientcmd.WriteToFile(*kc, outputPath)
}
