# Cloud Build Pipelines

This directory contains the Cloud Build YAML files used by the CI/CD system in the
census31-eq-ci-terraform project.

Two pipelines are defined:The cloud build files are invoked by triggers during
pr-build.yaml — PR validation pipeline
merge-push-to-gar.yaml — merge-to-main build and publish pipeline

## 1. PR Build Pipeline — pr-build.yaml
Purpose
a. Runs on every pull request.
b. Validates code quality but does not publish Docker images.

Steps

a. Build Docker image (local only)

## 2. Merge Build & Push Pipeline — merge-push-to-gar.yaml
Purpose
a. Runs on push to main.
b. Builds and pushes the Docker image to Artifact Registry.

Steps

a. Build Docker image

b. Push to GAR

## Note

$PROJECT_ID is passed forom the trigger

No gcloud auth is required inside Cloud Build

IAM must be applied to the service account

