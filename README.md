# EQ Launcher

This project was copied from [go-launch-a-survey](https://github.com/ONSdigital/go-launch-a-survey) and should be used for v3 of runner.

## Building and Running

Install Go and ensure that your `GOPATH` env variable is set (usually it's `~/go`).

```go
go get
go build
./census31-eq-questionnaire-launcher

go run launch.go (Does both the build and run cmd above)
```

Open `http://localhost:8000/`

## Docker

The Docker image can be built using the following command, providing the required target platform architecture as required

```shell
docker buildx build --platform [ linux/amd64 | linux/arm64 ] --no-cache -t census31-eq-questionnaire-launcher:latest .
```

You can then run the image using `SURVEY_RUNNER_SCHEMA_URL` to point it at an instance of survey runner.

```shell
docker run -e SURVEY_RUNNER_SCHEMA_URL=http://localhost:5000 -it -p 8000:8000 census31-eq-questionnaire-launcher:latest
```

The syntax for this will be slightly different on Mac

```shell
docker run -e SURVEY_RUNNER_SCHEMA_URL=http://host.docker.internal:5000 -it -p 8000:8000 census31-eq-questionnaire-launcher:latest
```

You should then be able to access go launcher at `localhost:8000`

## Cloud Run Deployment

The deployment configuration is kept alongside the application so changes to the
service can be reviewed with the code. [skaffold.yaml](skaffold.yaml) declares
the `launcher-image` artifact and uses [deploy/service.yaml](deploy/service.yaml)
as the Cloud Run service manifest. The Cloud Build trigger builds and pushes the
Docker image, then creates a Cloud Deploy release that supplies its image
reference for `launcher-image`.

The service manifest defines ingress, VPC egress, the runtime service account,
scaling, resource limits, concurrency, port 8000, and `SURVEY_RUNNER_URL`.
Values marked `from-param` are placeholders filled by Cloud Deploy target
parameters for the deployment environment; the empty strings in the manifest
are not runtime defaults.

The `checkov.io/skip1` annotation suppresses `CKV_K8S_21` only for this
service because Cloud Run does not use Kubernetes namespaces.

## Run Quick-Launch

If the schema specifies a `schema_name` field, that will be used as the schema_name claim. If not, the filename from the URL (before `.`) will be used.

Run Questionnaire Launcher

```text
scripts/run_app.sh
```

Now run Go launcher and navigate to "localhost:8000/quick-launch?schema_url=" passing the url of the JSON

```text
e.g."http://localhost:8000/quick-launch?schema_url=http://localhost:7777/1_0001.json"
```

The optional query parameter `version` can be added to the quick launch url which allows for the launch payload structure to be specified. If the parameter is not set then the default launch payload structure `v2` will be used.
Documentation on the `v2` structure is in [ons-schema-definitions](https://github.com/ONSdigital/ons-schema-definitions/blob/v3/docs/rm_to_eq_runner_payload_v2.rst)

```text
e.g."http://localhost:8000/quick-launch?schema_url=http://localhost:7777/1_0001.json&version=v1"
```

## Commands for Formatting & Linting

We use [Megalinter](https://megalinter.io/latest/mega-linter-runner/) to maintain our code by running various linters over the different file types we have. This is run against PRs using the `mega-linter` GitHub action but can also be run locally. To run the linter locally you can run:

```shell
make megalint
```

This command will run all the linters enabled in the `mega-linter.yml` config file in the root of the repo against the all the files in the repo and report back any issues. This is run via docker and may take some time to run first time.
We also have another command which will also run Megalinter locally but this one will attempt to fix any issues it can rather than just report them.

```shell
make megalint-apply
```

Its also possible to just run the golang linting locally, to run this you will need to have
[golangci-lint](https://golangci-lint.run/welcome/install/#local-installation) installed.

To install with Homebrew run:

```shell
brew install golangci-lint
```

To install with Conda run:

```shell
conda install conda-forge::golangci-lint
```

To lint the go files run:

```shell
make lint-go
```

This will run `golangci-lint` which is also configured to run `revive`, the config for this will
also be in the golangci.yml

To format the go files run:

```shell
make format-go
```

## Design System

To update the design system version, you need to update the version within the CDN link, they are present in both template files ([layout](templates/layout.html:11) and [launch](templates/launch.html:381))

## Notes

- There are no unit tests yet
- JWT spec based on [ons-schema-definitions](https://github.com/ONSdigital/census31-eq-questionnaire-runner-interface-definitions/blob/main/docs/jwt_profile.md)

## Settings

| Environment Variable           | Meaning                                                             | Default                                                                |
|--------------------------------|---------------------------------------------------------------------|------------------------------------------------------------------------|
| GO_LAUNCH_A_SURVEY_LISTEN_HOST | Host address to listen on                                           | 0.0.0.0                                                                |
| GO_LAUNCH_A_SURVEY_LISTEN_PORT | Host port to listen on                                              | 8000                                                                   |
| SURVEY_RUNNER_URL              | URL of Questionnaire Runner to re-direct to when launching a survey | `http://localhost:5000`                                                |
| SURVEY_REGISTER_URL            | URL of eq-survey-register to load schema list from                  | `http://localhost:8080`                                                |
| JWT_ENCRYPTION_KEY_PATH        | Path to the JWT Encryption Key (PEM format)                         | jwt-test-keys/sdc-user-authentication-encryption-sr-public-key.pem     |
| JWT_SIGNING_KEY_PATH           | Path to the JWT Signing Key (PEM format)                            | jwt-test-keys/sdc-user-authentication-signing-launcher-private-key.pem |
