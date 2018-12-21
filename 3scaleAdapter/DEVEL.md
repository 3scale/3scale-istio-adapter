# Development and testing

This document provides some instructions that may be helpfully when contributing changes and testing the adapter locally.

You will need a working Go environment to test and contribute to this project.
This project uses `dep` for dependency management. Follow the [installation instructions](https://golang.github.io/dep/docs/installation.html) for your operating system.

  * [Testing the adapter](#testing-the-adapter)
    * [Running tests](#running-tests)
    * [Running tests against real data](#running-tests-against-real-data)
      * [Build Mixer server and client](#build-mixer-server-and-client)
      * [Get your 3scale account](#get-your-3scale-account)
      * [Run a local instance of the adapter](#run-a-local-instance-of-the-adapter)
      * [Run Mixer](#run-mixer)
      * [Test the adapter](#test-the-adapter)
  * [Creating a debuggable adapter](#creating-a-debuggable-adapter)
  * [Making changes to configuration](#making-changes-to-configuration)

## Testing the adapter

### Running tests

Running `make unit` and `make integration` will run the unit tests and integration tests respectively. The adapters integration test uses the testing framework
provided by Istio to create an in-memory `mixer server` and therefore does not require any external dependencies. Appending `_coverage` to either of the `make` test
targets generates coverage reports.

The integration test above creates test servers to simulate responses from 3scale. However testing can be done using real data by following instructions in the next section.

__________________________________

### Running tests against real data

Requirements:
1. Existing 3scale account
1. Istio source code.


#### Build Mixer server and client

Get the istio sources:

```
export ISTIO=$GOPATH/src/istio.io/
mkdir -p $ISTIO
git clone https://github.com/istio/istio $ISTIO/istio
```

Compile `mixc` and `mixs`:

```
pushd $ISTIO/istio
make mixs DEBUG=1
make mixc
```

Make sure you have the `$GOPATH/bin/` in your `$PATH` var.

Now you should be able to use `mixc`/`mixs`:

```
mixc version
mixs version
```

#### Get your 3scale account

You can signup for a trial account here: https://www.3scale.net/signup/

Or you can deploy Redhat 3scale API Management on-premises.

You will need to write down:

  * Admin portal URL, for example https://istiodevel-admin.3scale.net
  * Access Token, you can find/create your access token in "Personal Settings/Tokens" section or https://istiodevel-admin.3scale.net/p/admin/user/access_tokens#service-tokens
  * Service ID, you can find the service ID in the API section, as the "ID for API calls is XXXXXXXXXX"
  * User_key: You will find this key in the integration page.

#### Run a local instance of the adapter

Build the adapter:

```
go get github.com/3scale/istio-integration/3scaleAdapter
cd $GOPATH/src/github.com/3scale/istio-integration/3scaleAdapter
make build
```


Modify the `testdata` with your 3scale account information:

```
vi testdata/threescale-adapter-config.yaml

----
# handler for adapter threescale
apiVersion: "config.istio.io/v1alpha2"
kind: handler
metadata:
 name: threescalehandler
 namespace: istio-system
spec:
 adapter: threescale
 params:
   access_token: "XXXXXXXXXXXXXXXXXXXXXX"
   service_id: "XXXXXXXXXXXX"
   system_url: "https://XXXXXX-admin.3scale.net/"
 connection:
   address: "[::]:3333"
```


Run the adapter locally...:

```
THREESCALE_LISTEN_ADDR=3333 go run cmd/main.go
```

Or in a container using the provided `make` target:
```
make docker-test
```

#### Run Mixer
If you followed the previous steps, `mixs` will have been built with debugging enabled, making it possible to attach a debugger to the process if required. 
Run `mixs server -h` to see the various flags that can be set for mixer.

Start `mixs` with the `testdata` configuration. 
 
```
make run-mixer-server
```

#### Test the adapter

Run `mixc` and pass the desired or required attributes.

```
mixc check -s request.path="/thepath?api_key=XXXXXXXXXXXXXXXXXXXXXXX" \
    --stringmap_attributes destination.labels=service-mesh.3scale.net:true
```

With this, you should be able to simulate the istio -> mixer -> adapter -> 3scale path.

__________________________________

## Creating a debuggable adapter

During development, it may be useful to step through adapter code while it's running within a cluster.
To do this you will need to build a specific version of the adapter image.

This guide assumes you have an OpenShift cluster running with istio installed in the `istio-system` namespace
and the 3scale adapter has already been deployed into that project

Run the following to create the image:
```
make debug-image REGISTRY=$(whoami) IMAGE=3scaleadapter TAG=debug
```

The debugger listens on port 40000 and we need to patch the service, run the following:
```bash
oc patch svc -n istio-system threescaleistioadapter \
   --patch='{"spec":{"ports":[{"name":"debug", "port":40000,"targetPort":40000}]}}'
```

Next, we need to patch the deployment with the image we built above:
```bash
export THREESCALE_DEBUG_ADAPTER=$(whoami)/3scaleistioadapter:debug
docker push ${THREESCALE_DEBUG_ADAPTER}
oc patch deployment -n istio-system 3scale-istio-adapter \
   --patch='{"spec":{"template":{"spec":{"containers":[{"name": "3scale-istio-adapter", "image":"'${THREESCALE_DEBUG_ADAPTER}'"}]}}}}'
```

Now, we need to get the Pod name and do some port forwarding:
```bash
POD_NAME="$(oc get po -n istio-system -o jsonpath='{.items[?(@.metadata.labels.app=="3scale-istio-adapter")].metadata.name}')"
oc port-forward ${POD_NAME} 40000 -n istio-system
```

Connect a remote debugger to `localhost:40000` and the adapter will begin to listen on `3333` as normal.

__________________________________

## Making changes to configuration

This adapter integrates with the Istio Mixer via gRPC. This model is referred to as an
[Out Of Process or OOP adapter](https://github.com/istio/istio/wiki/Mixer-Out-Of-Process-Adapter-Dev-Guide).

The project already contains the necessary generated files, templates and manifests, however there may at some
point be a need to extend or modify those. To do so, follow the [OOP adapter walk-through](https://github.com/istio/istio/wiki/Mixer-Out-Of-Process-Adapter-Walkthrough)
and copy the required changes to this code base. This generally relates to the files within the `config` directory.

__________________________________
