## CustomResource Generator

This program produces sample CustomResource definitions for [handler](https://istio.io/docs/concepts/policies-and-telemetry/#handlers),
 [instance](https://istio.io/docs/concepts/policies-and-telemetry/#instances)
 and [rule](https://istio.io/docs/concepts/policies-and-telemetry/#rules) CustomResources.

The manifests produced are tested against the integration test suite in the gRPC adapter code.

### Usage

The program accepts a number of flags which are documented in the table below:

| Option               | Description                                                                     | Required| Default      |
|----------------------|---------------------------------------------------------------------------------|---------|--------------|
|    `-h`, `--help`    |  Produces help output for available options                                     |   No    |              |
|    `--name`          |  Unique name for this (url,token) pair                                          |   Yes   |              |
|    `-n`,`--namespace`|  Namespace to generate templates for                                            |   No    | istio-system |
|    `-t`,`--token`    |  3scale access token                                                            |   Yes   |              |
|    `-u`,`--url`      |  3scale Admin Portal URL                                                        |   Yes   |              |
|    `--backend-url`   |  3scale Backend URL. If set, overrides the value read from system configuration |   No    |              |
|    `--service`       |  3scale Service ID. If set, generated config will apply to this service only    |   No    |              |
|    `--auth`          |  3scale authentication pattern to specify (1=Api Key, 2=App Id/App Key, 3=OIDC) |   No    | Hybrid       |
|    `-o`,`--output`   |  File to save produced manifests to                                             |   No    | STDOUT       |
|    `--version`       |  Outputs the CLI version (and exits right away)                                 |   No    |              |

### Example

This example will generate generic templates, allowing the token,url pair to be shared by multiple services as a single handler 
> 3scale-gen-config --name=admin-credentials --url="https://myorg-admin.3scale.net:443" --token="[redacted]"

This example will generate the templates with the service ID embedded in the handler:
> 3scale-gen-config --url="https://myorg-admin.3scale.net" --name="my-unique-id" --service="123456789" --token="[redacted]"


