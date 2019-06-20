module github.com/3scale/3scale-istio-adapter

go 1.12

require (
	github.com/3scale/3scale-go-client v0.0.0-20190408085735-e366adc214e9
	github.com/3scale/3scale-porta-go-client v0.0.2
	github.com/cenkalti/backoff v2.1.1+incompatible // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/gogo/googleapis v1.2.0
	github.com/gogo/protobuf v1.2.1
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/sync v0.0.0-20190412183630-56d357773e84 // indirect
	github.com/google/go-cmp v0.3.0 // indirect
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/gregjones/httpcache v0.0.0-20190203031600-7a902570cb17 // indirect
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645 // indirect
	github.com/hashicorp/golang-lru v0.5.1 // indirect
	github.com/imdario/mergo v0.3.6 // indirect
	github.com/json-iterator/go v1.1.5 // indirect
	github.com/prometheus/client_golang v0.9.3-0.20190127221311-3c4408c8b829
	github.com/spf13/viper v1.3.2
	github.com/uber/jaeger-client-go v0.0.0-20190312182356-f4d58ba83788 // indirect
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45 // indirect
	google.golang.org/appengine v1.5.0 // indirect
	google.golang.org/genproto v0.0.0-20190530194941-fb225487d101 // indirect
	google.golang.org/grpc v1.20.1
	istio.io/api v0.0.0-20190604023128-6f137ab2ce6d
	istio.io/istio v0.0.0-20190620052820-59b2c7e37677
	istio.io/pkg v0.0.0-20190603185215-940899ee7e72
	k8s.io/api v0.0.0-20190222213804-5cb15d344471
	k8s.io/apimachinery v0.0.0-20190221213512-86fb29eff628
	k8s.io/client-go v10.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20190418160015-6b3d3b2d5666 // indirect
)

replace github.com/golang/sync v0.0.0-20190412183630-56d357773e84 => golang.org/x/sync v0.0.0-20190412183630-56d357773e84
