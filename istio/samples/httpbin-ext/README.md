### ServiceEntry

ServiceEntry is a mechanism which enables APIs/services running outside of the mesh to appear as part of the
mesh to other services. The adapter is compatible with a ServiceEntry but cannot match on labels as used in the
typical routing scenario.

The manifests in this directory configure a ServiceEntry, Gateway and VirtualService to route to http://httpbin.org.

This allows external requests to reach the httpbin service via the ingress gateway via the `httpbin` prefix:
`curl -v  -H "Host: httpbin.org" {INGRESS_GATEWAY}/httpbin/get`

In order to integrate the adapter, follow the [instructions for routing traffic](../../../README.md#routing-service-traffic-through-the-adapter)

You will need to customise the `Rule` to target the intended ServiceEntry.
In the example given above, this could mean setting the match to the following:
`match: request.host == "httpbin.org"`