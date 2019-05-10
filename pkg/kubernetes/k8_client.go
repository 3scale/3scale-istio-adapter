package kubernetes

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewK8Client creates a new Kubernetes client from the provided configuration path
// or existing configuration. If no configuration is provided confPath will be used to generate one.
// This is a wrapper supporting both out-of-cluster and in-cluster configs
func NewK8Client(confPath string, conf *rest.Config) (*K8sClient, error) {
	if conf == nil {
		config, err := getConfigFromConfPath(confPath)
		if err != nil {
			return nil, err
		}
		conf = config
	}
	cs, err := getClientSetFromConfig(conf)
	if err != nil {
		return nil, err
	}
	return &K8sClient{conf, cs}, nil
}

// DiscoverManagedServices for deployments whose labels match the provided filter
// If provided namespace is empty string, all readable namespaces as authorised by the receivers config will be read
func (c *K8sClient) DiscoverManagedServices(namespace string, filterByLabels ...string) (*v1.DeploymentList, error) {
	opts := metav1.ListOptions{LabelSelector: formatLabelFilter(filterByLabels)}

	return c.cs.AppsV1().Deployments(namespace).List(opts)
}

// GetSecret by name from the provided namespace
// If no name is provided search is done by provided filter.
// Name and filters are mutually exclusive with provided name taking precedence.
// If search by filter is done and multiple or no secrets are found then an error is returned.
func (c *K8sClient) GetSecret(name, namespace string, filterByLabels ...string) (*corev1.Secret, error) {
	if name != "" {
		return c.cs.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
	}

	opts := metav1.ListOptions{LabelSelector: formatLabelFilter(filterByLabels)}
	secrets, err := c.cs.CoreV1().Secrets(namespace).List(opts)
	if err != nil {
		return nil, err
	}

	var matchingSecret *corev1.Secret
	var e error
	switch len(secrets.Items) {
	case 0:
		e = fmt.Errorf("no secret found in accessible namesapce that match provided filter(s)")
	case 1:
		matchingSecret = &secrets.Items[0]
	default:
		e = fmt.Errorf("unable to determine a matching secret from provided info - multiple secrets found")
	}
	return matchingSecret, e
}

// NewIstioClient creates a new client from an existing kubernetes client
// capable of manipulating known custom resources handler, instance and rule.
// It does not take care of creating the CRD for these extensions
func (c *K8sClient) NewIstioClient() (*IstioClientImpl, error) {
	s := runtime.NewScheme()
	schemeGroupVersion := schema.GroupVersion{Group: istioObjGroupName, Version: istioObjGroupVersion}

	addKnownTypes := func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypeWithName(getKnownGvk(handlerKind), &IstioResource{})
		scheme.AddKnownTypeWithName(getKnownGvk(instanceKind), &IstioResource{})
		scheme.AddKnownTypeWithName(getKnownGvk(ruleKind), &IstioResource{})

		metav1.AddToGroupVersion(scheme, schemeGroupVersion)
		return nil
	}

	schemeBuilder := runtime.NewSchemeBuilder(addKnownTypes)
	err := schemeBuilder.AddToScheme(s)
	if err != nil {
		return nil, err
	}

	cfg := rest.Config{
		Host:    c.conf.Host,
		APIPath: "/apis",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &schemeGroupVersion,
			NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(s)},
		},
		BearerToken:     c.conf.BearerToken,
		TLSClientConfig: c.conf.TLSClientConfig,
		UserAgent:       rest.DefaultKubernetesUserAgent(),
	}

	rc, err := rest.UnversionedRESTClientFor(&cfg)
	if err != nil {
		return nil, err
	}

	return &IstioClientImpl{&cfg, rc}, nil
}

// getConfigFromConfPath returns k8 client config from provided path
func getConfigFromConfPath(confPath string) (*rest.Config, error) {
	var conf *rest.Config
	var err error

	if confPath == "" {
		//fetch in cluster config
		conf, err = rest.InClusterConfig()
	} else {
		//use local kubeconfigs current context
		conf, err = clientcmd.BuildConfigFromFlags("", confPath)
	}
	return conf, err
}

// getClientSetFromConfig returns the appropriate kubernetes client based on the provided configuration
func getClientSetFromConfig(conf *rest.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(conf)
}

func getKnownGvk(name string) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   istioObjGroupName,
		Version: istioObjGroupVersion,
		Kind:    name,
	}
}

func formatLabelFilter(input []string) string {
	return strings.Join(input, ",")
}
