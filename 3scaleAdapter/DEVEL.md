

## Devel

### Build Mixer server and client.


Get the istio sources:

```
export ISTIO=$GOPATH/src/istio.io/
mkdir -p $ISTIO
git clone https://github.com/istio/istio $ISTIO/istio
```

Compile `mixc` and `mixs`:

```
pushd $ISTIO/istio
make mixs && make mixc
```

Make sure you have the `$GOPATH/bin/` in your `$PATH` var.

Now you should be able to use mixc / mixs: 

```
mixc version
mixs version
```

### Get your 3scale account

You can signup for a trial account here: https://www.3scale.net/signup/

Or you can deploy Redhat 3scale API Management on-premises.

You will need to write down:

  * Admin portal URL, for example https://istiodevel-admin.3scale.net
  * Access Token, you can find/create your access token in "Personal Settings/Tokens" section or https://istiodevel-admin.3scale.net/p/admin/user/access_tokens#service-tokens
  * Service ID, you can find the service ID in the API section, as the "ID for API calls is XXXXXXXXXX"
  * User_key: You will find this key in the integration page.

### Run a local instance of the adapter.

Get the adapter:

```
go get github.com/3scale/istio-integration/3scaleAdapter
```

Download deps:

```
cd $GOPATH/src/github.com/3scale/istio-integration/3scaleAdapter
dep ensure -v 
```

Modify the testdata with you 3scale account information:

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
   service_id: "XXXXXXXXXXXX"
   system_url: "https://XXXXXX-admin.3scale.net/"
  (...)
```

Run the adapter:

```
go run cmd/main.go 3333
```

### Run the adapter in a container

Use the provided Makefile:

```
make test
```

### Run Mixer

Start `mixs`:

```
mixs server --configStoreURL=fs://$GOPATH/src/github.com/3scale/istio-integration/3scaleAdapter/testdata
```

### Test the adapter! 

Run `mixc` with the desired request.path: 

```
mixc check -s request.path="/thepath?user_key=XXXXXXXXXXXXXXXXXXXXXXX"
```

With this, you should be able to simulate the istio -> mixer -> adapter -> 3scale path.  

