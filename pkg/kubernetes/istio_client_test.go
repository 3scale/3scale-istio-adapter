package kubernetes

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/3scale/3scale-istio-adapter/config"

	"istio.io/api/policy/v1beta1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/rest/fake"
)

func TestNewIstioClientWrapper(t *testing.T) {
	_, err := NewIstioClient("", nil)
	if err == nil || !strings.Contains(err.Error(), "KUBERNETES_SERVICE_HOST") {
		t.Errorf("expected to have failed create with invalid config")
	}

	_, err = NewIstioClient("", &rest.Config{Host: "fake"})
	if err != nil {
		t.Errorf("unexpected error when creating Istio client - %v", err)
	}
}

func TestCreateHandler(t *testing.T) {
	const svcID = "12345"
	const systemURL = "http://fake.com"
	const accessToken = "54321"

	const mockName = "test"

	hs := HandlerSpec{
		Adapter:    "threescale",
		Params:     config.Params{ServiceId: svcID, SystemUrl: systemURL, AccessToken: accessToken},
		Connection: v1beta1.Connection{Address: "nowhere:5555"},
	}

	returnWith := &IstioResource{
		TypeMeta: v1.TypeMeta{
			Kind:       handlerKind,
			APIVersion: istioObjGroupName + "/" + istioObjGroupVersion,
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      mockName,
			Namespace: DefaultNamespace,
		},
		Spec: hs,
	}

	rwb, err := json.Marshal(returnWith)
	if err != nil {
		t.Error("error converting struct to json")
	}

	client := IstioClientImpl{
		rc: &fake.RESTClient{
			GroupVersion:         schema.GroupVersion{Group: istioObjGroupName, Version: istioObjGroupVersion},
			NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: scheme.Codecs},
			Client: fake.CreateHTTPClient(func(request *http.Request) (response *http.Response, e error) {
				stream, err := request.GetBody()
				if err != nil {
					return nil, err
				}
				b, err := ioutil.ReadAll(stream)
				if err != nil {
					return nil, err
				}

				if strings.TrimSpace(string(b)) != string(rwb) {
					t.Errorf("expected handler to be converted to runtime object before being sent to server")
				}

				return &http.Response{StatusCode: http.StatusOK, Header: defaultHeader(t), Body: ioutil.NopCloser(bytes.NewBuffer(b))}, nil
			}),
		},
	}

	_, err = client.CreateHandler(mockName, DefaultNamespace, hs)
	if err != nil {
		t.Errorf("unexpected error creating handler " + err.Error())
	}
}

func defaultHeader(t *testing.T) http.Header {
	t.Helper()
	header := http.Header{}
	header.Set("Content-Type", runtime.ContentTypeJSON)
	return header
}
