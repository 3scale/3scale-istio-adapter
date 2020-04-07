## gRPC Server

An [out of process gRPC Adapter](https://github.com/istio/istio/wiki/Mixer-Out-Of-Process-Adapter-Dev-Guide) which integrates 3scale with Istio

### Configuring the adapter

The runtime behaviour of the adapter can be modified by editing the deployment and setting or
configuring the following environment variables:

| Variable                         | Description                                                                                        | Default |
|----------------------------------|----------------------------------------------------------------------------------------------------|---------|
| THREESCALE_LISTEN_ADDR           | Sets the listen address for the gRPC server                                                        | 0       |
| THREESCALE_LOG_LEVEL             | Sets the minimum log output level. Accepted values are one of `debug`,`info`,`warn`,`error`,`none` | info    |
| THREESCALE_LOG_JSON              | Controls whether the log is formatted as JSON                                                      | true    |
| THREESCALE_LOG_GRPC              | Controls whether the log includes gRPC info                                                        | false   |
| THREESCALE_REPORT_METRICS        | Controls whether 3scale system and backend metrics are collected and reported to Prometheus        | true    |
| THREESCALE_METRICS_PORT          | Sets the port which 3scale `/metrics` endpoint can be scrapped from                                | 8080    |
| THREESCALE_CACHE_TTL_SECONDS     | Time period, in seconds, to wait before purging expired items from the cache                       | 300     |
| THREESCALE_CACHE_REFRESH_SECONDS | Time period in seconds, before a background process attempts to refresh cached entries             | 180     |
| THREESCALE_CACHE_ENTRIES_MAX     | Max number of items that can be stored in the cache at any time. Set to 0 to disable caching       | 1000    |
| THREESCALE_CACHE_REFRESH_RETRIES | Sets the number of times unreachable hosts will be retried during a cache update loop              | 1       |
| THREESCALE_ALLOW_INSECURE_CONN   | Allow to skip certificate verification when calling 3scale API's. Enabling is not recommended      | false   |
| THREESCALE_CLIENT_TIMEOUT_SECONDS| Sets the number of seconds to wait before terminating requests to 3scale System and Backend        | 10      |
| THREESCALE_GRPC_CONN_MAX_SECONDS | Sets the maximum amount of seconds (+/-10% jitter) a connection may exist before it will be closed | 1       |
| THREESCALE_USE_CACHED_BACKEND    | If true, attempt to create an in-memory apisonator cache for authorization requests                | false   |

#### Caching behaviour
By default, responses from 3scale System API's will be cached. Entries will be purged from the cache when they
become older than the `THREESCALE_CACHE_TTL_SECONDS` value. Again by default however, automatic refreshing of cached entries will be attempted
some seconds before they expire, based on the `THREESCALE_CACHE_REFRESH_SECONDS` value. Automatic refreshing can be disabled by setting this value
higher than the `THREESCALE_CACHE_TTL_SECONDS` value.

Caching can be disabled entirely by setting `THREESCALE_CACHE_ENTRIES_MAX` to a non-positive value.

Through the refreshing process, cached values whose hosts become unreachable will be retried before eventually being purged
when past their expiry.