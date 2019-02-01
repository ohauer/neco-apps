Neco Ops
========

[![CircleCI](https://circleci.com/gh/cybozu-go/neco-ops.svg?style=svg)](https://circleci.com/gh/cybozu-go/neco-ops)

This repository contains GitOps resources for Neco. It mostly contains Kubernetes deployment resources.

Requirements
------------

- [Kubernetes][]
- [Argo CD][]
- [Kustomize][]

CI/CD
-----

See [docs/cicd.md](docs/cicd.md)

Kubernetes Manifest development
-------------------------------

See [docs/manifests.md](docs/manifests.md)

Deployment procedure
--------------------

See [docs/deploy.md](docs/deploy.md)

License
-------

MIT

[Kubernetes]: https://kubernetes.io/
[Kustomize]: https://github.com/kubernetes-sigs/kustomize
[Argo CD]: https://github.com/argoproj/argo-cd
[Alertmanager]: https://prometheus.io/docs/alerting/alertmanager/
[Ginkgo]: https://github.com/onsi/ginkgo
