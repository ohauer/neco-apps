How to maintain neco-apps
=========================

- [accurate](#accurate)
- [argocd](#argocd)
- [bmc-reverse-proxy](#bmc-reverse-proxy)
- [cert-manager](#cert-manager)
- [customer-egress (Squid and unbound)](#customer-egress-squid-and-unbound)
- [elastic (ECK)](#elastic-eck)
- [external-dns](#external-dns)
- [kube-metrics-adapter](#kube-metrics-adapter)
- [ingress (Contour & Envoy)](#ingress-contour--envoy)
- [local-pv-provisioner](#local-pv-provisioner)
- [logging](#logging)
  - [loki, loki-canary](#loki-loki-canary)
  - [promtail](#promtail)
  - [consul](#consul)
- [machines-endpoints](#machines-endpoints)
- [metallb](#metallb)
- [moco](#moco)
- [monitoring](#monitoring)
  - [pushgateway](#pushgateway)
  - [promtool](#promtool)
  - [kube-state-metrics](#kube-state-metrics)
  - [grafana-operator](#grafana-operator)
  - [Grafana](#grafana)
  - [heartbeat](#heartbeat)
  - [victoriametrics-operator](#victoriametrics-operator)
  - [VictoriaMetrics, Alertmanager](#victoriametrics-alertmanager)
- [neco-admission](#neco-admission)
- [network-policy (Calico)](#network-policy-calico)
- [pod-security-admission](#pod-security-admission)
- [prometheus-adapter](#prometheus-adapter)
- [pvc-autoresizer](#pvc-autoresizer)
- [rook](#rook)
  - [ceph](#ceph)
- [s3gw](#s3gw)
- [sealed-secrets](#sealed-secrets)
- [teleport](#teleport)
- [topolvm](#topolvm)

## accurate

Check [releases](https://github.com/cybozu-go/accurate/releases) for changes.

Update the manifest as follows:

```console
$ make update-accurate
$ git diff
```

## argocd

1. Check [releases](https://github.com/argoproj/argo-cd/releases) for changes.
2. Check [upgrading overview](https://github.com/argoproj/argo-cd/blob/master/docs/operator-manual/upgrading/overview.md) when upgrading major or minor version.
3. Run the following command and check the diff.

   ```console
   $ make update-argocd
   $ git diff
   ```

4. Update `KUSTOMIZE_VERSION` in `test/Makefile`.

## bmc-reverse-proxy

Update image tags as follows,

```console
$ make update-bmc-reverse-proxy
$ git diff
```

## cert-manager

Check [the upgrading section](https://cert-manager.io/docs/installation/upgrading/) in the official website.

```console
$ make update-cert-manager
$ git diff
```

## customer-egress (Squid and unbound)

customer-egress contains Squid and unbound containers.

Update the manifests as follows:

```console
$ make update-customer-egress
$ git diff
```

## elastic (ECK)

Check the [Release Notes](https://www.elastic.co/guide/en/cloud-on-k8s/current/eck-release-notes.html) and [Upgrade ECK](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-upgrading-eck.html) on the official website.

Update the upstream manifests as follows:

```console
$ make update-eck
$ git diff
```

Check the difference, and adjust our patches to the new manifests.

## external-dns

Read the following document and fix manifests as necessary.

https://github.com/kubernetes-sigs/external-dns/blob/vX.Y.Z/docs/tutorials/coredns.md

Update the manifests as follows:

```console
$ make update-external-dns
$ git diff
```

## kube-metrics-adapter

Check [releases](https://github.com/zalando-incubator/kube-metrics-adapter/releases).

Update the manifests as follows:

```console
$ make setup   # install/updaet Helm; if a newer Helm is required, update HELM_VERSION in Makefile first
$ make update-kube-metrics-adapter
$ git diff kube-metrics-adapter
```

## ingress (Contour & Envoy)

Check the [upgrading guide](https://projectcontour.io/resources/upgrading/) in the official website.

Check diffs of projectcontour/contour files as follows:

```console
$ git clone https://github.com/projectcontour/contour
$ cd contour
$ git checkout vX.Y.Z
$ git diff vA.B.C...vX.Y.Z examples/contour
```

Then, import YAML manifests as follows:

```console
$ cd $GOPATH/src/github.com/cybozu-go/neco-apps
$ rm ./ingress/base/contour/*
$ cp $GOPATH/src/github.com/projectcontour/contour/examples/contour/*.yaml ./ingress/base/contour/
```

Check diffs of contour and envoy deployments as follows:

```console
$ diff -u ingress/base/contour/03-contour.yaml ingress/base/template/deployment-contour.yaml
$ diff -u ingress/base/contour/03-envoy.yaml ingress/base/template/deployment-envoy.yaml
```

Note that:
- We do not use contour's certificate issuance feature, but use cert-manager to issue certificates required for gRPC.
- We change Envoy manifest from DaemonSet to Deployment.
  - We do not create `envoy` service account, and therefore `serviceAccountName: envoy` is removed from Envoy Deployment.
  - We replace or add probes with our custom one bundled in our Envoy container image.
- Not all manifests inherit the upstream. Please check `kustomization.yaml` which manifest inherits or not.
  - If the manifest in the upstream is usable as is, use it from `ingress/base/kustomization.yaml`.
  - If the manifest needs modification:
    - If the manifest is for a cluster-wide resource, put a modified version in the `common` directory.
    - If the manifest is for a namespaced resource, put a template in the `template` directory and apply patches.

## local-pv-provisioner

Update image tags as follows,

```console
$ make update-local-pv-provisioner
$ git diff
```

## logging

### loki, loki-canary

Check [loki releases](https://github.com/grafana/loki/releases).

Check [k8s-alpha](https://github.com/jsonnet-libs/k8s-alpha/) jsonnet library to find the appropriate value for `JSONNET_LIBS_K8S_ALPHA_VERSION` in Makefile.
If the library supports the currently used Kubernetes version, i.e. if the repository contains a directory with the name of the current Kubernetes version, then use that version as the value.
If not, use the latest Kubernetes version that the library supports.
In both cases, update `JSONNET_LIBS_K8S_ALPHA_VERSION` in Makefile if necessary.

Update the manifests as follows:

```console
$ make setup   # install/update Tanka and jsonnet-builder; if a newer Tanka is required, update TANKA_VERSION in Makefile first
$ make update-logging-loki
$ git diff logging
```

### promtail

Promtail is an agent for Loki.
It is published in Loki repository.
Check [loki releases](https://github.com/grafana/loki/releases) for the changes of Promtail.

There is no official kubernetes manifests for promtail.
Generate manifests from the Helm charts and check the changes as follows.

```
LOGGING_DIR=$GOPATH/src/github.com/cybozu-go/neco-apps/logging
helm repo add grafana https://grafana.github.io/helm-charts
helm search repo -l grafana | grep grafana/promtail
# Example output with a header line:
#   NAME                            CHART VERSION   APP VERSION     DESCRIPTION
#   grafana/promtail                3.5.1           2.2.1           Promtail is an agent which ships the contents o...
#   grafana/promtail                3.5.0           2.2.0           Promtail is an agent which ships the contents o...

# Choose the latest `CHART VERSION` which matches the target Loki's `APP VERSION` and set the value like below.
PROMTAIL_CHART_VERSION=X.Y.Z
helm template logging --namespace=logging grafana/promtail --version=${PROMTAIL_CHART_VERSION} > ${LOGGING_DIR}/base/promtail/upstream/promtail.yaml
cd ${LOGGING_DIR}
git diff
```

Check the difference between the existing manifest and the new manifest, and update the kustomization patch.

In upstream, loki and promtail settings are stored in secret resource. The configuration is now written in configmap, so decode the secret and compare the settings.

```console
$ yq eval '.stringData."promtail.yaml" | select(.)' logging/base/promtail/upstream/promtail.yaml > /tmp/promtail.yaml
$ diff -u logging/base/promtail/config/promtail.yaml /tmp/promtail.yaml

# diff of .client.url is intentional
```

Update the image tag as follows.

```console
$ make update-logging-promtail
$ git diff
```

### consul

Generate manifests from the Helm charts and check the changes as follows.

```
LOGGING_DIR=$GOPATH/src/github.com/cybozu-go/neco-apps/logging
helm repo add hashicorp https://helm.releases.hashicorp.com
helm search repo hashicorp/consul
helm template logging --namespace=logging hashicorp/consul -f ${LOGGING_DIR}/base/consul/values.yaml > ${LOGGING_DIR}/base/consul/upstream/consul.yaml
cd ${LOGGING_DIR}
git diff
```

Update the image tag as follows.

```console
$ make update-logging-consul
$ git diff
```

## machines-endpoints

`machines-endpoints` are used in `monitoring` and `bmc-reverse-proxy`.
Update their CronJobs as follows:

```console
$ make update-machines-endpoints
$ git diff
```

## metallb

Check [releases](https://github.com/metallb/metallb/releases)

Update the manifests as follows

```console
$ make update-metallb
$ git diff
```

## moco

Check [releases](https://github.com/cybozu-go/moco/releases) for changes.

Update the manifest as follows:

```console
$ make update-moco
$ git diff
```

## monitoring

### pushgateway

Check [releases](https://github.com/prometheus/pushgateway/releases) for changes.

Update the image tag as follows.

```console
$ make update-pushgateway
$ git diff
```

If you find that the tests for Pushgateway fail due to the stale manifests, then update the manifests.
There is no official kubernetes manifests for pushgateway.
We generate manifests from the Helm charts for reference as follows, and create/update simplified manifests in `monitoring/base/pushgateway`.

```
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm search repo -l prometheus-community
# use the latest version
helm template prom prometheus-community/prometheus --version=x.y.z > prom.yaml
```

### promtool

Update `PROMTOOL_VERSION` in `test/Makefile`.

### kube-state-metrics

Check the manifests in [examples/standard](https://github.com/kubernetes/kube-state-metrics/tree/master/examples/standard) directory.

Update the manifest as follows:

```console
$ make update-kube-state-metrics
$ git diff
```

### grafana-operator

Check [releases](https://github.com/integr8ly/grafana-operator/releases)

Update the manifest as follows:

```console
$ make update-grafana-operator
$ git diff
```

This make target also updates grafana_plugins_init.

### Grafana

Run the following command.

```console
$ make update-grafana
```

### heartbeat

Update the manifest as follows:

```console
$ make update-heartbeat
$ git diff
```

### victoriametrics-operator

Check [releases](https://github.com/VictoriaMetrics/operator/releases)

Update the manifest as follows:

```console
$ make update-victoriametrics-operator
$ git diff
```

### VictoriaMetrics, Alertmanager

Update the manifest as follows:

```console
$ make update-victoriametrics
$ git diff
```

## neco-admission

Update the manifest as follows:

```console
$ make update-neco-admission
$ git diff
```

## network-policy (Calico)

Check [the release notes](https://docs.projectcalico.org/release-notes/).

Update the manifest as follows:

```console
$ make update-calico
$ git diff
```

## pod-security-admission

Update the manifest as follows:

```console
$ make update-pod-security-admission
$ git diff
```

## prometheus-adapter

Check [releases](https://github.com/kubernetes-sigs/prometheus-adapter/releases).

Check the latest Helm chart for prometheus-adapter on https://github.com/prometheus-community/helm-charts .
For example, `prometheus-adapter-2.12.1` is the latest release as of Feb. 28th, 2021.

Update the Helm chart as follows:

```console
$ make update-prometheus-adapter CHART_VERSION=2.12.1
$ git diff
```

## pvc-autoresizer

Check [the CHANGELOG](https://github.com/topolvm/pvc-autoresizer/blob/main/CHANGELOG.md).

Update the manifest as follows:

```console
$ make update-pvc-autoresizer
$ git diff
```

## rook

*Caution:* Rook and Ceph require two phase update.
First phase, update Rook solely. Second phase, update Ceph and Rook image based on new Ceph image.

Read [this document](https://github.com/rook/rook/blob/master/Documentation/ceph-upgrade.md) before. Note that you should choose the appropriate release version.

Get upstream helm chart:

```console
$ cd $GOPATH/src/github.com/rook
$ git clone https://github.com/rook/rook
$ cd rook
$ ROOK_VERSION=X.Y.Z
$ git checkout v$ROOK_VERSION
$ ls $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base/upstream/chart
$ rm -rf $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base/upstream/chart
$ cp -a cluster/charts/rook-ceph $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base/upstream/chart
```

Download Helm used in Rook. Follow `HELM_VERSION` in the upstream configuration.

```console
# Check the Helm version, in rook repo directory downloaded above
$ cat $GOPATH/src/github.com/rook/rook/build/makelib/helm.mk | grep ^HELM_VERSION
$ HELM_VERSION=X.Y.Z
$ mkdir -p $GOPATH/src/github.com/cybozu-go/neco-apps/rook/bin
$ curl -sSLf https://get.helm.sh/helm-v$HELM_VERSION-linux-amd64.tar.gz | tar -C $GOPATH/src/github.com/cybozu-go/neco-apps/rook/bin linux-amd64/helm --strip-components 1 -xzf -
```

Update rook/base/values*.yaml if necessary.

Regenerate base resource yaml.  
Note: Check the number of yaml files.

```console
$ cd $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base
$ for i in clusterrole resources; do
    ../bin/helm template upstream/chart -f values.yaml -s templates/${i}.yaml > common/${i}.yaml
  done
$ for t in hdd ssd; do
    for i in deployment role rolebinding serviceaccount; do
      ../bin/helm template upstream/chart -f values.yaml -f values-${t}.yaml -s templates/${i}.yaml --namespace ceph-${t} > ceph-${t}/${i}.yaml
    done
    ../bin/helm template upstream/chart -f values.yaml -f values-${t}.yaml -s templates/clusterrolebinding.yaml --namespace ceph-${t} > ceph-${t}/clusterrolebinding/clusterrolebinding.yaml
  done
```

Then check the diffs by `git diff`.

Update manifest for Ceph toolbox.
Assume `rook/rook` is updated in the above procedure.

```console
$ cd $GOPATH/src/github.com/cybozu-go/neco-apps/
$ cp $GOPATH/src/github.com/rook/rook/cluster/examples/kubernetes/ceph/toolbox.yaml rook/base/upstream/
```

Update rook/**/kustomization.yaml if necessary.

### ceph

*Read Rook Caution!*

Read [this document](https://github.com/rook/rook/blob/master/Documentation/ceph-upgrade.md) first.

Update `spec.cephVersion.image` field in CephCluster CR.

- rook/base/ceph-hdd/cluster.yaml
- rook/base/ceph-ssd/cluster.yaml

## s3gw

Update image tags as follows,

```console
$ make update-s3gw
$ git diff
```

## sealed-secrets

Check the [release notes](https://github.com/bitnami-labs/sealed-secrets/blob/master/RELEASE-NOTES.md).

Update the manifest as follows:

```console
$ make update-sealed-secrets
$ git diff
```

## teleport

The teleport manifests in neco-apps is created from scratch.
Please check changes in [CHANGELOG.md](https://github.com/gravitational/teleport/blob/master/CHANGELOG.md) on github, the update of the upstream [Helm chart](https://github.com/gravitational/teleport/tree/master/examples/chart/teleport) and backport it.

```console
$ git clone https://github.com/gravitational/teleport
$ cd teleport
$ git diff vx.y.z...vX.Y.Z examples/chart/teleport
```

- Update `newTag` in `team-management/template/teleport/base/kustomization.libsonnet` and regenerate files.
- Updating teleport-node can also be done in this way. (Update `team-management/template/teleport/base/nodes/kustomization.libsonnet`)

```console
$ cd ./team-management/template
$ make setup
$ make teleport
```
- Update `TELEPORT_VERSION` in `test/Makefile`.

## topolvm

Check [releases](https://github.com/topolvm/topolvm/releases) for changes.

Update the manifest as follows:

```console
$ make update-topolvm
$ git diff
```
