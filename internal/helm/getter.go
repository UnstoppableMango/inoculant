package helm

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/unstoppablemango/inoculant/internal/client"
)

// restClientGetter adapts inoculant's *client.Client, built from a bare
// *rest.Config with no retained kubeconfig or clientcmd.ClientConfig
// anywhere else in the codebase, into the genericclioptions.RESTClientGetter
// Helm's action.Configuration.Init requires.
type restClientGetter struct {
	c         *client.Client
	namespace string
}

func (g *restClientGetter) ToRESTConfig() (*rest.Config, error) {
	return g.c.Cfg, nil
}

func (g *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return memory.NewMemCacheClient(g.c.Clientset.Discovery()), nil
}

func (g *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return g.c.Mapper, nil
}

// ToRawKubeConfigLoader returns a clientcmd.ClientConfig whose only
// load-bearing behavior for install/upgrade is Namespace(): kube.Client's
// internal namespace() falls back to it whenever its own Namespace field is
// unset, which action.Configuration.Init never sets.
func (g *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	overrides := &clientcmd.ConfigOverrides{
		Context: clientcmdapi.Context{Namespace: g.namespace},
	}
	return clientcmd.NewNonInteractiveClientConfig(
		*clientcmdapi.NewConfig(), "", overrides, nil,
	)
}
