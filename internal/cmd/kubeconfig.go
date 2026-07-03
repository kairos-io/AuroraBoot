package cmd

import (
	"errors"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// loadKubeConfig resolves a Kubernetes REST config. When path is set it is
// loaded verbatim from that single file. Otherwise in-cluster config is
// attempted first; a genuine in-cluster failure (e.g. a broken service-account
// token) is surfaced rather than silently masked. When we are simply not
// running in a cluster (rest.ErrNotInCluster), we fall through to the default
// client-go loading rules, which honour a multi-file KUBECONFIG env value and
// ~/.kube/config.
func loadKubeConfig(path string) (*rest.Config, error) {
	if path != "" {
		return clientcmd.BuildConfigFromFlags("", path)
	}
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, rest.ErrNotInCluster) {
		return nil, err
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
}
