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
  * [End-to-end walk-through](#end-to-end-walk-through)
    * [Deploying OpenShift with Istio](#deploying-openshift-with-istio)
    * [Create sample application](#create-sample-application)
    * [Create an Api on 3scale](#create-an-api-on-3scale)
    * [Generate the custom resources](#generate-the-custom-resources)
    * [Integrating your OpenShift service](#integrating-your-openshift-service)
    * [Testing Integration](#testing-integration)

## Testing the adapter

### Running tests

Running `make unit` and `make integration` will run the unit tests and integration tests respectively. The adapters integration test uses the testing framework
provided by Istio to create an in-memory `mixer server` and therefore does not require any external dependencies. Appending `_coverage` to either of the `make` test
targets generates coverage reports.

The integration test above creates test servers to simulate responses from 3scale. However testing can be done using real data by following instructions in the next section.

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
go get github.com/3scale/3scale-istio-adapter
cd $GOPATH/src/github.com/3scale/3scale-istio-adapter
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

## Creating a debuggable adapter

During development, it may be useful to step through adapter code while it's running within a cluster.
To do this you will need to build a specific version of the adapter image.

This guide assumes you have an OpenShift cluster running with istio installed in the `istio-system` namespace
and the 3scale adapter has already been deployed into that project

Run the following to create the image:
```
make debug-image REGISTRY=$(whoami) IMAGE=3scale-istio-adapter TAG=debug
```

The debugger listens on port 40000 and we need to patch the service, run the following:
```bash
oc patch svc -n istio-system threescaleistioadapter \
   --patch='{"spec":{"ports":[{"name":"debug", "port":40000,"targetPort":40000}]}}'
```

Next, we need to patch the deployment with the image we built above:
```bash
export THREESCALE_DEBUG_ADAPTER=$(whoami)/3scale-istio-adapter:debug
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

## Making changes to configuration

This adapter integrates with the Istio Mixer via gRPC. This model is referred to as an
[Out Of Process or OOP adapter](https://github.com/istio/istio/wiki/Mixer-Out-Of-Process-Adapter-Dev-Guide).

The project already contains the necessary generated files, templates and manifests.
This generally relates to the files within the `config` directory.

To extend or make changes here, read the [OOP adapter walk-through](https://github.com/istio/istio/wiki/Mixer-Out-Of-Process-Adapter-Walkthrough)
and update the `go generate` commands in this code base as required.

Once the changes have been made, run `make generate-config`.
Copy the required generated files in `$ISTIO/mixer/adapter/3scale-istio-adapter/config`
into the `testdata` directory in this repository. Run `make test`.

Assuming a successful test run, copy the required generated files to `config`.
Build the adapter image with these changes and verify the functionality.

## Creating a release

There is a `make` target to help with creating a release. It requires `VERSION=vx.y.z` as an argument. Please follow [Semantic Versioning](https://github.com/semver/semver/blob/master/semver.md)

The target will do the following:

1. Update the dependencies
1. Generate the Deployment definition with correct container image
1. Build the Docker image with the specified version as a tag
1. Push said image
1. Ask the user to commit the changes

When you have committed and reviewed the changes push to github and create a PR for review.

After the PR is approved and merged to master, checkout and pull the changes to master and run `make tag-release VERSION=vx.y.z`. 
Push the tag to the remote repo. Create a release based on this tag with the appropriate changelog.



## End-to-end walk-through

The guide will walk you through the creation of the required OpenShift cluster and the deployment of
the 3scale adapter and its' integration with a 3scale API.

### Deploying OpenShift with Istio

The upstream [Maistra project](https://github.com/Maistra/) contains a fork of Origin which we will use for this walk-through.
Follow the [Maistra documentation](https://maistra.io/docs/install/) to install and Istio as required. The all-in-one setup is the
likely the quickest to get started with. When creating the `Installation` custom resource, ensure 3scale is enabled:

```yaml
  threeScale:
    enabled: true
```

Alternatively, you can tweak as required, and run [this script for CentOS/RHEL based installation from the ground up](https://gist.github.com/unleashed/dea133398805fb2daef391ff960d0479)

### Create sample application

The Istio project uses the following [bookinfo application](https://istio.io/docs/examples/bookinfo/) for demonstration and testing purposes.
It consists of a set of microservices integrated within the service mesh.

The Maistra project also ships this example app and we will use that for the walk-through. Log into the OpenShift cluster we created
in the last step as a `cluster-admin` and run the [following script](https://gist.github.com/unleashed/5f1aaeed880474fc9eb1861f0c6444e8).
Wait for all the Pods in the `bookinfo` namespace to become ready.

### Create an API on 3scale

We are assuming that an active 3scale account exists at this point. If not, [go create one](https://www.3scale.net/signup/).

Create the following 3scale resources:
1. Create an API
2. Create an Application Plan
3. Create an Application

Note down the following:

* The service ID
* The system URL
* The access token

Set the integration method to Istio. For this example, we are going to use the API Key authentication pattern, so scroll down and select that option.
Create a Mapping Rule for this service in 3scale with `GET` verb and `/productpage` pattern. Create some limits if desired.

### Generate the custom resources

Follow the instructions to generate the sample resources [here](README.md#generating-sample-custom-resources). THis will print the sample
YAML to your terminal. As well as a unique identifier. Note down this `UID` for the next section.
Save this to a file and edit as required. You may want to look at changing the location of the credentials or the
API Key label etc. For more details see [the instructions in main documentation](README.md#api-key-pattern)

Once you are happy with your changes. Run `oc create -f` on the modified file.

### Integrating your OpenShift service

Now that the rules are configured and the adapter is deployed we want to use the `productpage-v1` deployment in the
`bookinfo` project to be managed by 3scale for example purposes.

Run `istiooc edit deploy productpage-v1 -n bookinfo`
Ensure the following block exists under `.spec.template.metadata`

```yaml
      labels:
        app: productpage
        version: v1
        service-mesh.3scale.net: 'true'
        service-mesh.3scale.net/uid: 'replace-with-uid-from-previous-section'
```

Alternatively, follow the [main documentation](README.md#generating-sample-custom-resources), we provide a command there to patch the deployment.
This process is also [documented in more detail](README.md#routing-service-traffic-through-the-adapter)

### Testing Integration

Next we need to test the integration worked as expected. Lets export the ingress gateway as an environment variable for convenience

```bash
export GW=$(istiooc get route istio-ingressgateway -n istio-system -o go-template='http://{{ .spec.host }}')
```

Now lets call the service we have integrated without any authentication:

```bash
curl ${GW}/productpage
```

As expected you should see an error that include the following text: `PERMISSION_DENIED`

Next, lets add a fake/incorrect `user_key`:

```bash
curl ${GW}/productpage?user_key=intruder
```

Again we see an error similar to `PERMISSION_DENIED` with some additional information

Now lets add our correct `user_key`
```bash
curl ${GW}/productpage?user_key=XXX_REPLACE_ME_XXX
```

At this point we should see the request allowed through and you can get all the book information you desire :)

Make repeated calls to verify the limits that were set and ensure they are enforced. Verify the hits act as expected and analytics are reported.
