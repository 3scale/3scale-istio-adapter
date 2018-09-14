package httpPluginClient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const (
	validUserKey     = "12345"
	validAccessToken = "abcdef"
	validServiceID   = "111111"
)

func Test_Authorize(t *testing.T) {


	var AuthorizationTests = []struct {
		accessToken string
		userKey     string
		ok          bool
	}{
		{validAccessToken, validUserKey, true},
		{validAccessToken, "", false},
		{"", validUserKey, false},
		{"", "", false},
		{"aaaaaaaaaaaa", "222222", false},
	}

	for _, tt := range AuthorizationTests {

		systemURL, _ := url.Parse("https://mysystemurl-admin.3scale.net/")

		originalRequest := &http.Request{
			Method: "get",
			URL: &url.URL{
				User: &url.Userinfo{},
				Path: "/path?query=1&query=2&query=3&user_key=" + tt.userKey,
			},
			Header: map[string][]string{"user-key": {"user_key_header"}},
		}

		c := NewClient(nil)

		ts := HTTPPluginMock(t)

		c.BaseURL, _ = url.Parse(ts.URL)

		ok, err := c.Authorize(tt.accessToken, validServiceID, systemURL, originalRequest)
		if err != nil {
			t.Errorf("Authorize returned error: %#v", err)
		}

		if ok != tt.ok {
			t.Error("Expected")
		}
	}

}

// Mocking the 3scale HTTP Plugin
func HTTPPluginMock(t *testing.T) *httptest.Server {

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Validate the authorization request, PUT /auth
		if r.RequestURI == authorizationPath && r.Method == "PUT" {
			decoder := json.NewDecoder(r.Body)
			var jsonBody httpPlugin
			err := decoder.Decode(&jsonBody)
			if err != nil {
				t.Errorf("Failed to unmarshall request: %#v", err)
			}

			// Validate the final object
			parsedURL, err := url.Parse(jsonBody.HTTPRequest.Path)
			if err != nil {
				t.Errorf("Could not parse HTTPRequest.Path: %#v", err)
			}

			// System URL is properly constructed?
			systemURL, err := url.Parse(jsonBody.SystemEndpoint)
			if err != nil {
				t.Errorf("Invalid SystemURL: %#v", err)
			}

			// System URL has the proper auth?
			if systemURL.User.String() != validAccessToken {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, "Forbidden")
			}

			// User_key is properly constructed?
			userKey := parsedURL.Query()["user_key"]
			if userKey[0] == validUserKey {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, "Ok")
			} else {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, "Forbidden")
			}
		}
	}))
	return ts
}
