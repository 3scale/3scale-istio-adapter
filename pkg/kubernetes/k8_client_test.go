package kubernetes

import (
	"fmt"
	"strings"
	"testing"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"
)

func TestNewK8Client(t *testing.T) {
	_, err := NewK8Client("", nil)
	if err == nil || !strings.Contains(err.Error(), "KUBERNETES_SERVICE_HOST") {
		t.Errorf("expected to have failed create with invalid config")
	}

	_, err = NewK8Client("", &rest.Config{})
	if err != nil {
		t.Errorf("expected to use passed configuration to generate client")
	}
}

func TestDiscoverManagedServices(t *testing.T) {
	deployType := metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"}
	const validLabelKey = "found"

	fakeDeployOne := &v1.Deployment{
		TypeMeta: deployType,
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deploy-one",
			Namespace: "test",
			Labels:    map[string]string{validLabelKey: "true"},
		},
	}

	fakeDeployTwo := &v1.Deployment{
		TypeMeta: deployType,
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deploy-two",
			Namespace: "na",
		},
	}

	client := fake.NewSimpleClientset()
	client.AppsV1().Deployments("test").Create(fakeDeployOne)
	client.AppsV1().Deployments("test").Create(fakeDeployTwo)
	k8 := K8Client{cs: client}

	inputs := []struct {
		name       string
		discoverNs string
		withLabels []string
		expectLen  int
		expectName string
	}{
		{
			name:       "Test no deployments are returned for bogus filter",
			withLabels: []string{"not-found"},
			expectLen:  0,
		},
		{
			name:       "Test valid filter but incorrect namespace",
			discoverNs: "na",
			withLabels: []string{fmt.Sprintf("%s=true", validLabelKey)},
			expectLen:  0,
		},
		{
			name:       "Test multiple filters do not match",
			withLabels: []string{fmt.Sprintf("%s=true", validLabelKey), "bogus"},
		},
		{
			name:       "Test happy path",
			withLabels: []string{fmt.Sprintf("%s=true", validLabelKey)},
			expectLen:  1,
			expectName: "deploy-one",
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			ds, err := k8.DiscoverManagedServices(input.discoverNs, input.withLabels...)
			if err != nil {
				t.Errorf("unexpected err - %s", err.Error())
			}

			if len(ds.Items) != input.expectLen {
				t.Errorf("unexpected number of deployments returned. Expected %d, but got %d", input.expectLen, len(ds.Items))
			}

			if input.expectLen == 0 {
				return
			}

			if ds.Items[0].Name != input.expectName {
				t.Errorf("unexpected deployments returned. Expected %s, but got %s", input.expectName, ds.Items[0].Name)
			}
		})
	}

}

func TestGetSecret(t *testing.T) {
	const kind = "Secret"
	const version = "v1"

	testSecrets := []*corev1.Secret{
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       kind,
				APIVersion: version,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret-one",
				Namespace: "test",
				Labels:    map[string]string{"generic": "true"},
			},
			Data: map[string][]byte{
				"test": []byte("yes"),
				"mock": []byte("kind of"),
			},
			Type: corev1.SecretTypeOpaque,
		},
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       kind,
				APIVersion: version,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret-two",
				Namespace: "test",
				Labels: map[string]string{
					"test-search": "true",
					"generic":     "true",
				},
			},
			Data: map[string][]byte{
				"test-2": []byte("yes"),
				"mock":   []byte("kind of"),
			},
			Type: corev1.SecretTypeOpaque,
		},
	}

	client := fake.NewSimpleClientset()
	for _, secret := range testSecrets {
		client.CoreV1().Secrets(secret.ObjectMeta.Namespace).Create(secret)
	}

	k8 := K8Client{cs: client}

	inputs := []struct {
		name       string
		namespace  string
		secretName string
		labels     []string
		expectKey  string
		expectErr  bool
		reactor    func()
	}{
		{
			name:       "Test search for secret by name",
			secretName: "secret-one",
			expectKey:  "test",
		},
		{
			name:      "Test search for secret by label not found",
			labels:    []string{"test-search=false"},
			expectErr: true,
		},
		{
			name:      "Test expect error on multiple label matches",
			labels:    []string{"generic=true"},
			expectErr: true,
		},
		{
			name:      "Test search for secret by label",
			expectKey: "test-2",
			labels:    []string{"test-search=true"},
		},
		{
			name:      "Test error when list request fails request",
			namespace: "none",
			expectErr: true,
			reactor: func() {
				client.PrependReactor("list", "secrets", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, fmt.Errorf("401")
				})
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			if input.reactor != nil {
				input.reactor()
			}

			s, err := k8.GetSecret(input.secretName, input.namespace, input.labels...)
			if err != nil {
				if input.expectErr {
					return
				}
				t.Errorf("unexpected error" + err.Error())
			}
			if len(s.Data[input.expectKey]) == 0 {
				t.Errorf("expected key not present in secret data")
			}
		})

	}
}

func TestNewIstioClient(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &rest.Config{
		Host: "fake",
	}
	k8 := K8Client{conf: cfg, cs: client}
	ic, err := k8.NewIstioClient()
	if err != nil {
		t.Errorf("unexpected error when creating istio client")
	}

	if ic.conf.Host != "fake" {
		t.Errorf("host not propogated from k8 client")
	}
	expect := schema.GroupVersion{Group: istioObjGroupName, Version: istioObjGroupVersion}

	if *ic.conf.ContentConfig.GroupVersion != expect {
		t.Errorf("incorrect GVK specified")
	}
}
