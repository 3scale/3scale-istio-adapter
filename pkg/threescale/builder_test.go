package threescale

import (
	"net/http"
	"testing"
)

func TestClientBuilder_BuildSystemClient(t *testing.T) {
	builder := NewClientBuilder(http.DefaultClient)

	_, err := builder.BuildSystemClient("invalid.due.to.no.scheme")
	if err == nil {
		t.Errorf("expected failure due to no scheme provided")
	}

	// todo : this test is commented while https://github.com/3scale/3scale-porta-go-client/pull/19 remains
	// open and the dependency not updated

	//_, err = builder.BuildSystemClient("/")
	//if err == nil {
	//	t.Error("expected failure due to badly configured admin portal")
	//}

	_, err = builder.BuildSystemClient("https://expect.pass")
	if err != nil {
		t.Errorf("unexpected failure buidling http client")
	}

}

func TestClientBuilder_BuildBackendClient(t *testing.T) {
	builder := NewClientBuilder(http.DefaultClient)

	_, err := builder.BuildBackendClient("invalid.due.to.no.scheme")
	if err == nil {
		t.Errorf("expected failure due to no scheme provided")
	}

	_, err = builder.BuildBackendClient("https://expect.pass")
	if err != nil {
		t.Errorf("unexpected failure buidling http client")
	}

}
