# Change Log

Notable changes to 3scale Istio Mixer Adapter will be tracked in this document.

## 2.0.1 - 2020-09-29

### Fixed

- The configuration for the failure policy (`BACKEND_CACHE_POLICY_FAIL_CLOSED`)
  was not being taken into account and its default value was false rather than
  true. ([#175](https://github.com/3scale/3scale-istio-adapter/pull/175))
- Added missing documentation in the gRPC server for the flush intervals and
  the failure policy, and removed the unused `THREESCALE_` prefix. ([#175](https://github.com/3scale/3scale-istio-adapter/pull/175))


## 2.0.0.1 - 2020-07-30

### Security

- Updated dependencies on golang.org/x/text and golang.org/x/crypto to fix CVE-2020-14040 and CVE-2020-9283.
  ([#171](https://github.com/3scale/3scale-istio-adapter/pull/171))

## 2.0.0 - 2020-07-24

## Added

- An authorization cache which maintains local counters for rate limits and
  periodically flushes metrics to 3scale to increase performance.
  ([#167](https://github.com/3scale/3scale-istio-adapter/pull/167))
- A ConfigMap to provide configuration to the gRPC adapter
  ([#164](https://github.com/3scale/3scale-istio-adapter/pull/164))
    * A configuration option to the gRPC server (`USE_CACHED_BACKEND`) to enable the authorization cache.
    * A configuration option to the gRPC server (`BACKEND_CACHE_FLUSH_INTERVAL_SECONDS`) to set the interval at which metrics get reported from the cache to 3scale.
    * A configuration option to the gRPC server (`BACKEND_CACHE_POLICY_FAIL_CLOSED`) to determine the fate of a request if 3scale Apisonator is unreachable.
- Prometheus metrics for authorization cache
  ([#162](https://github.com/3scale/3scale-istio-adapter/pull/162))
- Support for `last` and priority of mapping rules as defined in 3scale Porta
  ([#150](https://github.com/3scale/3scale-istio-adapter/pull/150))

## Changed

- Removal of `THREESCALE_` prefix from existing environment variables
  ([#164](https://github.com/3scale/3scale-istio-adapter/pull/164))
- Metrics have been converged to include both Porta and Apisonator requests, differentiated by labels.
  ([#162](https://github.com/3scale/3scale-istio-adapter/pull/162))
- Updated response codes to match HTTP equivalent, as returned by APIcast
  ([#144](https://github.com/3scale/3scale-istio-adapter/pull/144))
  ([#138](https://github.com/3scale/3scale-istio-adapter/pull/138))
  ([#136](https://github.com/3scale/3scale-istio-adapter/pull/136))

## Fixed

- Issue with `help` command for CLI tool returning excess information from imported dependency.
  ([#151](https://github.com/3scale/3scale-istio-adapter/pull/151))

## 1.0.0.1 - 2020-07-29

### Security

- Updated dependencies on golang.org/x/text and golang.org/x/crypto to fix CVE-2020-14040 and CVE-2020-9283.
  ([#171](https://github.com/3scale/3scale-istio-adapter/pull/171))

## 1.0.0 - 2019-08-07

## Added

- A field to the handler params (`backend_url`) to allow overriding the 3scale backend the adapter should reach out to.
  ([#111](https://github.com/3scale/3scale-istio-adapter/pull/111))

## Fixed

- The CLI tool no longer panics when provided with a name that fails Kubernetes validation.
  ([#113](https://github.com/3scale/3scale-istio-adapter/pull/113))

## 0.7.1 - 2019-06-17

## Added

- A configuration option to the gRPC server (`THREESCALE_LOG_GRPC`) to allow suppression of gRPC logging.
  A configuration option to the gRPC server (`THREESCALE_GRPC_CONN_MAX_SECONDS`) to allow setting specific keepalive parameters.
  ([#104](https://github.com/3scale/3scale-istio-adapter/pull/104))

## Changed

- The Kubernetes service is now headless to support client side load balancing.
  The CLI tool now generates the connection address prefixed with `dns:///`.
  ([#104](https://github.com/3scale/3scale-istio-adapter/pull/104))
- The dependency on Istio's `api` and `istio` packages is now based on version `1.1.8`
  ([#108](https://github.com/3scale/3scale-istio-adapter/pull/108))


## 0.7.0 - 2019-06-12

## Added

- Support for reading a services 3scale service ID from the pod's label providing a service discovery mechanism.
  This in turn requires a `name` flag to be added to the CLI tool.
  ([#93](https://github.com/3scale/3scale-istio-adapter/pull/93))
- A `namesapce` flag has been added to the CLI tool to support multi-tenancy in Maistra.
  ([#103](https://github.com/3scale/3scale-istio-adapter/pull/103))

## Changed

- The CLI tool now generates different output:
  The `destination.labels["service-mesh.3scale.net"]` and `destination.labels["service-mesh.3scale.net/uid"]` labels have been changed solely to
  `destination.labels["service-mesh.3scale.net/credentials"]` in order to support service discovery and the `context.reporter.kind == "inbound"` label has
  been added to match for ingress only traffic.
  ([#93](https://github.com/3scale/3scale-istio-adapter/pull/93))
- The Docker image is now built from RHEL 8's UBI minimal base image in place of Alpine image.
  The VERSION argument is derived at build time unless specified.
  ([#98](https://github.com/3scale/3scale-istio-adapter/pull/98))
- In support of multi-tenancy in Maistra, the `namespace` field has been removed from the provided sample templates.
  ([#103](https://github.com/3scale/3scale-istio-adapter/pull/103))
- The removal of non-required labels and the reformatting of some existing labels has been made to Prometheus metrics reporting latency.
  ([#105](https://github.com/3scale/3scale-istio-adapter/pull/105))
- The dependency on Istio's `api` and `istio` packages is now based on version `1.1.7`
  ([#107](https://github.com/3scale/3scale-istio-adapter/pull/107))

## Fixed

- Latency reports between the adapter and 3scale system are now reported correctly.
  ([#105](https://github.com/3scale/3scale-istio-adapter/pull/105))

## Removed

- The `github.com/3scale/3scale-istio-adapter/pkg/templating` package has been removed.
  In turn the `service`, `uid` and `fixup` flags have been removed from the CLI.
  ([#93](https://github.com/3scale/3scale-istio-adapter/pull/93))


## 0.6.0 - 2019-05-02

## Added

- The [OpenID Connect](https://github.com/3scale/3scale-istio-adapter/blob/v0.6.0/README.md#openid-connect-pattern) authentication pattern is now supported via Istio's [end user authn](https://istio.io/help/ops/security/end-user-auth/) and the CLI tool to generate templates can now generate an instance template supporting this pattern. The hybrid pattern has been updated to include OIDC as well. (#[89](https://github.com/3scale/3scale-istio-adapter/pull/89))

## Changed

- The CLI tool enables the so-called "fixup" mode when no explicit unique id is
  specified, so that automatic generation of identifierss from URLs are modified
  to comply with k8s format and rules. Now calling the tool without the --uid
  option will autoenable the --fixup one. ([#91](https://github.com/3scale/3scale-istio-adapter/pull/91))

## 0.5.1 - 2019-04-09

### Fixed

- The templates generated by the CLI tool now reference the right instance
  ([#84](https://github.com/3scale/3scale-istio-adapter/pull/84))
- The templates generated by the CLI tool use lower case to match headers
  ([#83](https://github.com/3scale/3scale-istio-adapter/pull/83))
- A segmentation fault caused by freeing a null pointer has been fixed by
  updating the 3scale backend client library to a newer release.
  ([#81](https://github.com/3scale/3scale-istio-adapter/issues/81), [#82](https://github.com/3scale/3scale-istio-adapter/pull/82))

### Added

- The adapter now prints out its version when running and the adapter Deployment
  definition is now generated by a script.
  ([#80](https://github.com/3scale/3scale-istio-adapter/pull/80))
- Documentation updates for Istio 1.1
  ([#79](https://github.com/3scale/3scale-istio-adapter/pull/79))

