## CustomResource Generator

This program produces sample CustomResource definitions for [handler](https://istio.io/docs/concepts/policies-and-telemetry/#handlers),
 [instance](https://istio.io/docs/concepts/policies-and-telemetry/#instances) 
 and [rule](https://istio.io/docs/concepts/policies-and-telemetry/#rules) CustomResources.

The manifests produced are tested against the integration test suite in the gRPC adapter code and support multiple services,
by generating a unique UID per service.

### Usage

The program accepts a number of flags which are documented in the table below:

| Option               | Description                                                             | Required| Default |
|----------------------|-------------------------------------------------------------------------|---------|---------|
|    `-h`, `--help`    |  Produces help output for available options                             |   No    |         |
|    `-t`, `--token`   |  3scale access token                                                    |   Yes   |         |
|    `-u`, `--url`     |  3scale Admin Portal URL                                                |   Yes   |         |
|    `-s`, `--service` |  3scale API/Service ID                                                  |   Yes   |         |
|    `--uid`           |  Unique UID for resource names. Derived from URL and service  if unset  |   No    |         |
|    `--fixup`         |  Try to automatically fix validation errors                             |   No    | False   |
|    `--auth`          |  3scale authentication pattern to specify (1=Api Key, 2=App Id/App Key) |   No    | Hybrid  |
|    `-o`, `--output`  |  File to save produced manifests to                                     |   No    | STDOUT  |
|    `-v`              |  Outputs the CLI version                                                |   No    |         |