// Package client builds the Kubernetes clients shared by the apply and
// bootstrap packages.
package client

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// ApplyOptions is used for every server-side apply inoculant performs,
// both for user manifests and bootstrap RBAC.
func ApplyOptions() metav1.ApplyOptions {
	return metav1.ApplyOptions{
		FieldManager: "inoculant",
		Force:        true, // TODO: confirm inoculant should always win field conflicts (currently unconditional)
	}
}

// Client holds the Kubernetes clients needed to apply manifests and
// bootstrap RBAC against a cluster.
type Client struct {
	Cfg       *rest.Config
	Mapper    meta.RESTMapper
	Dynamic   dynamic.Interface
	Clientset *kubernetes.Clientset
}

// New builds a Client from cfg.
func New(cfg *rest.Config) (*Client, error) {
	http, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("build http client: %w", err)
	}

	c := &Client{Cfg: cfg}
	if c.Mapper, err = apiutil.NewDynamicRESTMapper(cfg, http); err != nil {
		return nil, fmt.Errorf("build rest mapper: %w", err)
	}
	if c.Dynamic, err = dynamic.NewForConfigAndClient(cfg, http); err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	if c.Clientset, err = kubernetes.NewForConfigAndClient(cfg, http); err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	return c, nil
}
