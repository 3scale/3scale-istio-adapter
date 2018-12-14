# 3scale Istio Mixer gRPC Adapter

An [out of process gRPC Adapter](https://github.com/istio/istio/wiki/Mixer-Out-Of-Process-Adapter-Dev-Guide) which integrates 3scale with Istio

* [Overview](#overview)
* [Prerequisites](#prerequisites)
* [Deploy the adapter](#deploy-the-adapter)
* [Customise and create the adapter configuration](#customise-and-create-the-adapter-configuration)
* [Routing service traffic through the adapter](#routing-service-traffic-through-the-adapter)
* [Authenticating requests](#authenticating-requests)
* [Adapter metrics](#adapter-metrics)
* [Development and contributing](#development-and-contributing)

## Overview

*This project is currently in alpha and is not yet supported. Provided templates and configuration may change*

When running Istio in a Kubernetes or OpenShift cluster, this adapter allows a user to label their service
running within the mesh, and have that service integrated with the [3scale Api Management solution](https://www.3scale.net/).

## Prerequisites

1. Istio version 1.0.5+
1. A working [3scale account](https://www.3scale.net/signup) (SaaS or On-Premises)
1. `kubectl` or `oc` command line tool

## Deploy the adapter

A Kubernetes deployment and service manifest are located in the `deploy` directory.
To deploy the adapter to a Kubernetes/OpenShift cluster, within the `istio-system` project, run

```bash
oc create -f deploy/
```

## Customise and create the adapter configuration

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
 name: threescalehandler
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

In order to drive traffic for your service through the adapter and be managed by 3scale, we need to match the rule
`destination.labels["service-mesh.3scale.net"] == "true"` we previously created in the configuration, in the `kind: rule` resource.

To do so, we need to add a label to the PodTemplateSpec on the Deployment of the target workload. 
For example, if we have a `productpage` service, whose Pod is managed by the `productpage-v1` deployment, 
by adding the above label under `spec.template.labels` in `productpage-v1`, we can have the adapter authorise requests to this service.

## Authenticating requests

Now that the we have [configured the service to be managed by 3scale](#routing-service-traffic-through-the-adapter)

TODO - Describe the various supported authentication methods

## Adapter metrics

The adapter, by default reports various Prometheus metrics which are exposed on port `8080` at the `/metrics` endpoint.
These allow some insight into how the interactions between the adapter and 3scale are performing. The service is labelled
to be automatically discovered and scraped by Prometheus.


## Development and contributing

Check the [DEVEL.md](DEVEL.md) for more info on how to hack/test this adapter.
