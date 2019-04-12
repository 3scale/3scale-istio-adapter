## CustomResource Generator

This program produces sample CustomResource definitions for [handler](https://istio.io/docs/concepts/policies-and-telemetry/#handlers),
 [instance](https://istio.io/docs/concepts/policies-and-telemetry/#instances)
 and [rule](https://istio.io/docs/concepts/policies-and-telemetry/#rules) CustomResources.

The manifests produced are tested against the integration test suite in the gRPC adapter code and support multiple services,
by generating a unique UID per service.

### Usage

The program accepts a number of flags which are documented in the table below:

| Option               | Description                                                                    | Required| Default |
|----------------------|--------------------------------------------------------------------------------|---------|---------|
|    `-h`, `--help`    |  Produces help output for available options                                    |   No    |         |
|    `-t`, `--token`   |  3scale access token                                                           |   Yes   |         |
|    `-u`, `--url`     |  3scale Admin Portal URL                                                       |   Yes   |         |
|    `-s`, `--service` |  3scale API/Service ID                                                         |   Yes   |         |
|    `--uid`           |  Unique UID for resource names. Derived from URL and service if unset          |   No    |         |
|    `--fixup`         |  Try to automatically fix validation errors. Autoenabled if --uid unset        |   No    | False   |
|    `--auth`          |  3scale authentication pattern to specify (1=Api Key, 2=App Id/App Key/3=OIDC) |   No    | Hybrid  |
|    `-o`, `--output`  |  File to save produced manifests to                                            |   No    | STDOUT  |
|    `-v`              |  Outputs the CLI version (and exits right away)                                |   No    |         |

### Example

This example will generate the templates just from the URL, service and personal
token:
> 3scale-gen-config --url="https://myorg-admin.3scale.net:443" --service="123456789" --token="[redacted]"

This example will generate the templates just like above but with a specific
unique ID, which otherwise would have been derived from the URL:
> 3scale-gen-config --url="https://myorg-admin.3scale.net" --uid="my-unique-id" --service="123456789" --token="[redacted]"

The previous example might have failed to pass validation if the given unique ID
did contain some special characters, so you might want to use the fixup mode to
have the tool autocorrect those:
> 3scale-gen-config --url="https://myorg-admin.3scale.net" --uid="my-unique-id" --fixup --service="123456789" --token="[redacted]"
