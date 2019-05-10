# 3scale Istio Mixer gRPC Adapter

An [out of process gRPC Adapter](https://github.com/istio/istio/wiki/Mixer-Out-Of-Process-Adapter-Dev-Guide) which integrates 3scale with Istio

* [Overview](#overview)
* [Prerequisites](#prerequisites)
* [Enabling Policies](#enabling-policies)
* [Deploy the adapter](#deploy-the-adapter)
* [Customise adapter manifests and create the resources](#customise-adapter-manifests-and-create-the-resources)
* [Routing service traffic through the adapter](#routing-service-traffic-through-the-adapter)
* [Authenticating requests](#authenticating-requests)
  * [Applying Patterns](#applying-patterns)
    * [API Key Pattern](#api-key-pattern)
    * [Application ID Pattern](#application-id-pattern)
    * [OpenID Connect Pattern](#openid-connect-pattern)
    * [Hybrid](#hybrid-pattern)
* [Generating sample CustomResources](#generating-sample-custom-resources)
* [Adapter metrics](#adapter-metrics)
* [Development and contributing](#development-and-contributing)

## Overview

*This project is currently in alpha and is not yet supported. Provided templates and configuration may change*

When running Istio in a Kubernetes or OpenShift cluster, this adapter allows a user to label their service
running within the mesh, and have that service integrated with the [3scale Api Management solution](https://www.3scale.net/).

## Prerequisites

1. Istio version 1.1 with [policies enabled](#enabling-policies)
1. A working [3scale account](https://www.3scale.net/signup) (SaaS or On-Premises)
1. `kubectl` or `oc` command line tool

## Enabling Policies

As of Istio 1.1.0 GA, policies are now disabled by default. This impacts the 3scale adapter and policies need to be re-enabled
in order for the adapter to receive traffic.

Follow [these instructions](https://istio.io/docs/tasks/policy-enforcement/enabling-policy/) to enable policies.

## Deploy the adapter

A Kubernetes deployment and service manifest are located in the `deploy` directory.
To deploy the adapter to a Kubernetes/OpenShift cluster, within the `istio-system` project, run

```bash
oc create -f deploy/
```

## Configuring the adapter

See [the adapter configuration options](cmd/server/README.md) to understand the default behaviour of the adapter, and how to modify it.

## Customise adapter manifests and create the resources

The required CustomResources are located in the `istio` directory. These samples can be used
to configure requests to your services via 3scale.

Pay particular attention to the `kind: handler` resource in the file `istio/threescale-adapter-config.yaml`.
This will need to be updated with your own 3scale credentials.

Modify the handler configuration: `istio/threescale-adapter-config.yaml` with
your 3scale configuration.

```yaml
# handler for adapter Threescale
apiVersion: "config.istio.io/v1alpha2"
kind: handler
metadata:
 name: threescale
 namespace: istio-system
spec:
 adapter: threescale
 params:
   service_id: "XXXXXXXXXXXX"
   system_url: "https://XXXXXX-admin.3scale.net/"
   access_token: "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
 connection:
   address: "threescale-istio-adapter:3333"
```

Create the required resources:

```bash
oc create -f istio/ -n istio-system
```

## Routing service traffic through the adapter

The following statements assume that configuration has been generated using the tool provided. Custom modifications
to the match condition in the rule are possible, but explanation of any or all combinations is outside of the scope of
this document. If interested, check out the [upstream documentation](https://istio.io) 

In order to drive traffic for your service through the adapter and be managed by 3scale, we need to match the rule
`destination.labels["service-mesh.3scale.net/credentials"] == "threescale"` we previously created in the configuration, in the `kind: rule` resource.

Integration of a service requires that the above label be added to PodTemplateSpec on the Deployment of the target workload.
The value, `threescale`, refers to the name of the generated handler. This handler will store the access token required to call 3scale.

When the request reaches the adapter, the adapter needs to know the how the service maps to the an API on 3scale.
This can be provided in one of two ways:

1. As a label on the workload (recommended)
1. Hardcoded in the handler as `service_id`

To pass the service ID to the adapter via the instance at request time, add the following label to the workload:

`destination.labels["service-mesh.3scale.net/service-id"] == "replace-me"`

Your 3scale administrator should be able to provide you with both the required credentials name and the service ID.

## Authenticating requests

Now that the we have [configured the service to be managed by 3scale](#routing-service-traffic-through-the-adapter) we can decide how requests should be authenticated.
Currently there are two supported mechanisms:
1. The API Key authentication pattern
2. The Application ID, Application Key (optional) pair authentication pattern

You can read more detailed information about these patterns and their behaviour in the [3scale documentation](https://access.redhat.com/documentation/en-us/red_hat_3scale_api_management/2.4/html/api_authentication/authentication-patterns).


### Applying Patterns

When you have decided what pattern best fits your needs, you can modify the `instance` CustomResource to configure this behaviour.
You can also decide if authentication credentials should be read from headers or query parameters, or allow both.

It is important to note that when specifying values from headers, Istio expects they key to be lower case.
So for example if you want to send a header as `X-User-Key`, this must be referenced in the configuration as `request.headers["x-user-key"]`.


#### API Key Pattern
To use the *API Key authentication pattern*, you should use the `user` value on the `subject` field like so:

```yaml
apiVersion: "config.istio.io/v1alpha2"
kind: instance
metadata:
  name: threescale-authorization
  namespace: istio-system
spec:
  template: threescale-authorization
  params:
    subject:
      user: request.query_params["user_key"] | request.headers["x-user-key"] | ""
    action:
      path: request.url_path
      method: request.method | "get"
      service: destination.labels["service-mesh.3scale.net/service-id"] | ""
```

This configuration will examine the `user_key` query parameter, followed by the `x-user-key` header in search of the api key. As mentioned, this can be restricted to one or the other by removing that particular attribute.
The order can be changed to determine precedence.

If you would like for the adapter to examine a different, for example query parameter than `user_key`, you would simply change `[user_key]` to `[foo]`. The same pattern applies to the headers.

#### Application ID Pattern
To use the *Application ID authentication pattern*, you should use the `properties` value on the `subject` field to set `app_id`, and **optionally** `app_key`.

Manipulation of this object can be done in using the methods described previously.
An example configuration is shown below.

```yaml
apiVersion: "config.istio.io/v1alpha2"
kind: instance
metadata:
  name: threescale-authorization
  namespace: istio-system
spec:
  template: threescale-authorization
  params:
    subject:
        app_id: request.query_params["app_id"] | request.headers["x-app-id"] | ""
        app_key: request.query_params["app_key"] | request.headers["x-app-key"] | ""
    action:
      path: request.url_path
      method: request.method | "get"
      service: destination.labels["service-mesh.3scale.net/service-id"] | ""
```

#### OpenID Connect Pattern
To use the *OpenID Connect authentication pattern*, you should use the `properties` value on the `subject` field to set `client_id`, and **optionally** `app_key`.

Manipulation of this object can be done in using the methods described previously.
An example configuration is shown below. Here the client identifier (application ID) is parsed from the JWT under the label "azp". Modify this as desired.

```yaml
apiVersion: "config.istio.io/v1alpha2"
kind: instance
metadata:
  name: threescale-authorization
  namespace: istio-system
spec:
  template: threescale-authorization
  params:
    subject:
        app_key: request.query_params["app_key"] | request.headers["x-app-key"] | ""
        client_id: request.auth.claims["azp"] | ""
    action:
      path: request.url_path
      method: request.method | "get"
      service: destination.labels["service-mesh.3scale.net/service-id"] | ""
```

For this integration to work correctly. OpenID configuration must still be done in 3scale in order for the client to be created in the IdP.
For the service the user wants to protect, they should create [end-user authentication](https://istio.io/help/ops/security/end-user-auth/) in the same namespace as that service.
The JWT should then be passed in the `Authorization` header of the request.

In the sample `Policy` defined below, replace `issuer` and `jwksUri` as appropriate.

```yaml
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: jwt-example
  namespace: bookinfo
spec:
  origins:
    - jwt:
        issuer: >-
          http://keycloak-keycloak.34.242.107.254.nip.io/auth/realms/3scale-keycloak
        jwksUri: >-
          http://keycloak-keycloak.34.242.107.254.nip.io/auth/realms/3scale-keycloak/protocol/openid-connect/certs
  principalBinding: USE_ORIGIN
  targets:
    - name: productpage
```

#### Hybrid Pattern

Finally, you may decide to not enforce a particular authentication method but accept any valid credentials for either pattern. In that case, you can do a hybrid configuration where the user key pattern will be preferred if both are provided:
```yaml
apiVersion: "config.istio.io/v1alpha2"
kind: instance
metadata:
  name: threescale-authorization
  namespace: istio-system
spec:
  template: threescale-authorization
  params:
    subject:
      user: request.query_params["user_key"] | request.headers["x-user-key"] | ""
      properties:
        app_id: request.query_params["app_id"] | request.headers["x-app-id"] | ""
        app_key: request.query_params["app_key"] | request.headers["x-app-key"] | ""
    action:
      path: request.url_path
      method: request.method | "get"
      service: destination.labels["service-mesh.3scale.net/service-id"] | ""

```

## Generating sample custom resources

The adapter embeds a tool which allows generation of the `handler`,`instance` and `rule` CR's.
More detail can be found in the tools [documentation](cmd/cli/README.md)

To generate these manifests from a deployed adapter, run the following:

```bash
oc exec -n istio-system $(oc get po -n istio-system -o jsonpath='{.items[?(@.metadata.labels.app=="3scale-istio-adapter")].metadata.name}') \
-it -- ./3scale-config-gen \
--url="https://replace-me.3scale.net:443" --name="example-name" --token="access-token"
```

This will produce some sample output to the terminal. Edit these samples if required and create the objects using `oc create` command.


Update the workload (target service deployment's Pod Spec) with the required annotations:

```bash
export CREDENTIALS_NAME="replace-me"
export SERVICE_ID="replace-me"
export DEPLOYMENT="replace-me"
patch="$(oc get deployment "${DEPLOYMENT}" --template='{"spec":{"template":{"metadata":{"labels":{ {{ range $k,$v := .spec.template.metadata.labels }}"{{ $k }}":"{{ $v }}",{{ end }}"service-mesh.3scale.net/service-id":"'"${SERVICE_ID}"'","service-mesh.3scale.net/credentials":"'"${CREDENTIALS_NAME}"'"}}}}}' )"
oc patch deployment "${DEPLOYMENT}" --patch ''"${patch}"''
```

## Adapter metrics

The adapter, by default reports various Prometheus metrics which are exposed on port `8080` at the `/metrics` endpoint.
These allow some insight into how the interactions between the adapter and 3scale are performing. The service is labelled
to be automatically discovered and scraped by Prometheus.


## Development and contributing

Check the [DEVEL.md](DEVEL.md) for more info on how to hack/test this adapter.
