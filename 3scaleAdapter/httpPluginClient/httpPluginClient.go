package httpPluginClient

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func UnmarshalHTTPPlugin(data []byte) (HTTPPlugin, error) {
	var r HTTPPlugin
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *HTTPPlugin) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type HTTPPlugin struct {
	ServiceID      int64       `json:"service_id"`
	SystemEndpoint string      `json:"system_endpoint"`
	HTTPRequest    HTTPRequest `json:"http_request"`
}

type HTTPRequest struct {
	Path    string              `json:"path"`
	Method  string              `json:"method"`
	Headers map[string][]string `json:"headers"`
}

type Client struct {
	BaseURL    *url.URL
	httpClient *http.Client
}

const (
	AuthorizationPath = "/auth"
	TelemetryPath     = "/report"
)

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 2 * time.Second,
		}
	}

	defaultBaseUrl, _ := url.Parse("http://localhost:8090")

	c := &Client{
		httpClient: httpClient,
		BaseURL:    defaultBaseUrl,
	}
	return c
}

func (c *Client) newRequest(method, path string, body interface{}) (*http.Request, error) {
	rel := &url.URL{Path: path}
	u := c.BaseURL.ResolveReference(rel)
	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return resp, err
}

func (c *Client) Authorize(accessToken string, serviceID string, systemEndpointURL *url.URL, originalRequest *http.Request) (bool, error) {
	ok := false

	ServiceIDint64, err := strconv.ParseInt(serviceID, 10, 64)

	systemEndpointURL.User = url.User(accessToken)

	body := HTTPPlugin{
		ServiceID:      ServiceIDint64,
		SystemEndpoint: systemEndpointURL.String(),
		HTTPRequest: HTTPRequest{
			Path: originalRequest.URL.Path,
			// Method comes from istio in lowercase, we need to upper it for the 3scale Http Plugin
			Method:  strings.ToUpper(originalRequest.Method),
			Headers: originalRequest.Header,
		},
	}

	req, err := c.newRequest("PUT", AuthorizationPath, body)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		ok = true
	} else {
		ok = false
	}

	return ok, err
}
