# 3scale Istio Mixer gRPC Adapter (PoC)

This document is a WIP

## Warning

*  This is not supported. It's a proof of concept, not ready for production use.

## Requirements

* Istio 1.0pre
* A working 3scale account (SaaS or On-Premises)

## How to deploy **WIP**


Modify the handler configuration: `istio/threescale-adapter-config.yaml` with 
your 3scale configuration.

```
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
   address: "threescaleistioadapter:3333"
```

then deploy the 3scale Adapter POD and required configuration:

```
oc project istio-system
oc create -f ./openshift
oc create -f ./istio 
```

### Other

Check the [DEVEL.md](DEVEL.md) for more info on how to hack/test this adapter.
