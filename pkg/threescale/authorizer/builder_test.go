package authorizer

import (
	"net/http"
	"testing"
)

func TestClientBuilder_BuildSystemClient(t *testing.T) {
	const token = "any"
	builder := NewClientBuilder(http.DefaultClient)

	_, err := builder.BuildSystemClient("invalid.due.to.no.scheme", token)
	if err == nil {
		t.Errorf("expected failure due to no scheme provided")
	}

	_, err = builder.BuildSystemClient("/", token)
	if err == nil {
		t.Error("expected failure due to badly configured admin portal")
	}

	_, err = builder.BuildSystemClient("http://expect.pass", token)
	if err != nil {
		t.Errorf("unexpected failure buidling http client")
	}

	_, err = builder.BuildSystemClient("https://expect.pass", token)
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
