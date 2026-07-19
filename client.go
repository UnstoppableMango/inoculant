package inoculant

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// applyOpts is used for every server-side apply this package performs,
// both for user manifests (apply.go) and bootstrap RBAC (bootstrap.go).
var applyOpts = metav1.ApplyOptions{
	FieldManager: "inoculant",
	Force:        true, // TODO: confirm inoculant should always win field conflicts (currently unconditional)
}

// Client applies manifests and bootstraps scoped RBAC against a Kubernetes cluster.
type Client struct {
	cfg       *rest.Config
	mapper    meta.RESTMapper
	client    dynamic.Interface
	clientset *kubernetes.Clientset
}

// New builds a Client from cfg.
func New(cfg *rest.Config) (*Client, error) {
	http, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("build http client: %w", err)
	}

	c := &Client{cfg: cfg}
	if c.mapper, err = apiutil.NewDynamicRESTMapper(cfg, http); err != nil {
		return nil, fmt.Errorf("build rest mapper: %w", err)
	}
	if c.client, err = dynamic.NewForConfigAndClient(cfg, http); err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	if c.clientset, err = kubernetes.NewForConfigAndClient(cfg, http); err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	return c, nil
}
