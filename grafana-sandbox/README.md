grafana-sandbox
===============

The sandbox Grafana has a special handling.

- It is deployed only to stage0 environment.
  - On other environments, `grafana-sandbox` Argo CD Application does not exist.
- It is **not tested** in CI because the initialization of Grafana MySQL takes toooooo much time on gcp/placemat environment.
  - To test manually on dctest environment, apply the output manifest of `kustomize build grafana-sandbox/overlays/gcp`.
