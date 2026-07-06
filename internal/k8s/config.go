// Package k8s resolves client configuration: explicit kubeconfig flag first,
// then in-cluster ServiceAccount, then default kubeconfig loading rules.
package k8s

import (
	"fmt"
	"os"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Clients bundles the two client flavors the console needs.
type Clients struct {
	Dynamic dynamic.Interface
	Typed   kubernetes.Interface
	Host    string
}

// NewClients resolves config and constructs clients. kubeconfig may be empty.
func NewClients(kubeconfig string) (*Clients, error) {
	cfg, err := resolveConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	// Informer startup lists many types at once; don't get throttled.
	cfg.QPS = 50
	cfg.Burst = 100

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("building dynamic client: %w", err)
	}
	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("building typed client: %w", err)
	}
	return &Clients{Dynamic: dyn, Typed: typed, Host: cfg.Host}, nil
}

func resolveConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("loading kubeconfig %s: %w", kubeconfig, err)
		}
		return cfg, nil
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config: %w", err)
		}
		return cfg, nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules() // honors $KUBECONFIG
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules,
		&clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("default kubeconfig: %w", err)
	}
	return cfg, nil
}
