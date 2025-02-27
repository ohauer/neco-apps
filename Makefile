# Makefile to update manifests

HELM_VERSION = 3.8.0
TANKA_VERSION = 0.15.1
JSONNET_LIBS_K8S_ALPHA_VERSION = 1.20
YQ_VERSION = 4.24.5

KUSTOMIZE = test/bin/kustomize

.PHONY: all
all:
	@echo Read docs/maintenance.md for the usage

.PHONY: test-generate
test-generate:
	$(MAKE) update-rook

.PHONY: update-accurate
update-accurate:
	$(call get-latest-helm,accurate,https://cybozu-go.github.io/accurate)
	yq eval -i '.spec.source.targetRevision = "$(latest_helm)"' argocd-config/base/accurate.yaml

.PHONY: update-argocd
update-argocd:
	$(call get-latest-tag,argocd)
	curl -sSLf -o argocd/base/upstream/install.yaml \
		https://raw.githubusercontent.com/argoproj/argo-cd/$(call upstream-tag,$(latest_tag))/manifests/install.yaml
	curl -sSLf -o argocd/base/upstream/notifications-catalog.yaml \
		https://raw.githubusercontent.com/argoproj/argo-cd/$(call upstream-tag,$(latest_tag))/notifications_catalog/install.yaml
	sed -i -E '/name:.*argocd$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' argocd/base/kustomization.yaml
	sed -i -e 's/ARGOCD_VERSION *:=.*/ARGOCD_VERSION := $(subst v,,$(call upstream-tag,$(latest_tag)))/' test/Makefile
	$(call get-latest-tag,dex)
	sed -i -E '/name:.*dex$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' argocd/base/kustomization.yaml
	$(call get-latest-tag,redis)
	sed -i -E '/name:.*redis$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' argocd/base/kustomization.yaml

.PHONY: update-bmc-reverse-proxy
update-bmc-reverse-proxy:
	$(call get-latest-tag,bmc-reverse-proxy)
	sed -i -E 's,image: quay.io/cybozu/bmc-reverse-proxy:.*$$,image: quay.io/cybozu/bmc-reverse-proxy:$(latest_tag),' bmc-reverse-proxy/base/bmc-reverse-proxy/deployment.yaml

.PHYNY: update-cadvisor
update-cadvisor:
	$(call get-latest-tag,cadvisor)
	rm -rf /tmp/cadvisor
	cd /tmp; git clone --depth 1 -b $(call upstream-tag,$(latest_tag)) https://github.com/google/cadvisor
	rm -rf monitoring/base/cadvisor/upstream/*
	cp -r /tmp/cadvisor/deploy/kubernetes/base monitoring/base/cadvisor/upstream
	cp -r /tmp/cadvisor/deploy/kubernetes/overlays monitoring/base/cadvisor/upstream
	rm -rf /tmp/cadvisor
	sed -i -E '/newName:.*cadvisor$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' monitoring/base/kustomization.yaml

.PHONY: update-cattage
update-cattage:
	$(call get-latest-helm,cattage,https://cybozu-go.github.io/cattage)
	yq eval -i '.spec.source.targetRevision = "$(latest_helm)"' argocd-config/base/cattage.yaml

.PHONY: update-cert-manager
update-cert-manager:
	$(call get-latest-tag,cert-manager)
	curl -sSLf -o cert-manager/base/upstream/cert-manager.yaml \
		https://github.com/jetstack/cert-manager/releases/download/$(call upstream-tag,$(latest_tag))/cert-manager.yaml
	sed -i -E 's/newTag:.*$$/newTag: $(latest_tag)/' cert-manager/base/kustomization.yaml

.PHONY: update-customer-egress
update-customer-egress:
	curl -sSLf -o customer-egress/base/neco/squid.yaml \
		https://raw.githubusercontent.com/cybozu-go/neco/release/etc/squid.yml
	sed -e 's/internet-egress/customer-egress/g' \
		-e 's,{{ .squid }},quay.io/cybozu/squid,g' \
		-e 's,{{ index . "cke-unbound" }},quay.io/cybozu/unbound,g' \
		-e '/nodePort: 30128/d' customer-egress/base/neco/squid.yaml > customer-egress/base/squid.yaml
	$(call get-latest-tag,squid)
	sed -i -E '/name:.*squid$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' customer-egress/base/kustomization.yaml
	$(call get-latest-tag,unbound)
	sed -i -E '/name:.*unbound$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' customer-egress/base/kustomization.yaml

.PHONY: update-eck
update-eck:
	$(call get-latest-gh,elastic/cloud-on-k8s)
	curl -sSLf -o elastic/base/upstream/crds.yaml https://download.elastic.co/downloads/eck/$(latest_gh)/crds.yaml
	curl -sSLf -o elastic/base/upstream/operator.yaml https://download.elastic.co/downloads/eck/$(latest_gh)/operator.yaml

.PHONY: update-external-dns
update-external-dns:
	$(call get-latest-tag,external-dns)
	curl -sSLf -o external-dns/base/upstream/crd.yaml \
		https://raw.githubusercontent.com/kubernetes-sigs/external-dns/$(call upstream-tag,$(latest_tag))/docs/contributing/crd-source/crd-manifest.yaml
	sed -i -E 's,quay.io/cybozu/external-dns:.*$$,quay.io/cybozu/external-dns:$(latest_tag),' external-dns/base/deployment.yaml

.PHONY: update-grafana-operator
update-grafana-operator:
	$(call get-latest-tag,grafana-operator)
	rm -rf /tmp/grafana-operator
	cd /tmp; git clone --depth 1 -b $(call upstream-tag,$(latest_tag)) https://github.com/grafana-operator/grafana-operator
	rm -rf monitoring/base/grafana-operator/upstream/*
	mkdir -p monitoring/base/grafana-operator/upstream/cluster_roles
	mkdir -p monitoring/base/grafana-operator/upstream/manifests
	cp -r /tmp/grafana-operator/deploy/cluster_roles/* monitoring/base/grafana-operator/upstream/cluster_roles
	cp -r /tmp/grafana-operator/deploy/manifests/latest/* monitoring/base/grafana-operator/upstream/manifests
	rm -rf /tmp/grafana-operator
	sed -i -E '/newName:.*grafana-operator$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' monitoring/base/kustomization.yaml
	$(call get-latest-tag,grafana_plugins_init)
	sed -i -E 's/grafana-plugins-init-container-tag=.*$$/grafana-plugins-init-container-tag=$(latest_tag)/' monitoring/base/grafana-operator/deployment.yaml

.PHONY: update-grafana
update-grafana:
	$(call get-latest-tag,grafana)
	sed -i -E 's/grafana-image-tag=.*$$/grafana-image-tag=$(latest_tag)/' monitoring/base/grafana-operator/deployment.yaml
	sed -i -E 's,quay.io/cybozu/grafana:.*$$,quay.io/cybozu/grafana:$(latest_tag),' grafana-sandbox/overlays/common/grafana.yaml

.PHONY: update-heartbeat
update-heartbeat:
	$(call get-latest-tag,heartbeat)
	sed -i -E '/name:.*heartbeat$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' monitoring/base/kustomization.yaml

.PHONY: update-hubble
update-hubble:
	$(call get-latest-tag,hubble-ui-frontend)
	sed -i -E '/repository:.*hubble-ui-frontend$$/!b;n;s/tag:.*$$/tag: $(latest_tag)/' hubble/base/values.yaml
	$(call get-latest-tag,hubble-ui-backend)
	sed -i -E '/repository:.*hubble-ui-backend$$/!b;n;s/tag:.*$$/tag: $(latest_tag)/' hubble/base/values.yaml
	helm repo add cilium https://helm.cilium.io/ >/dev/null
	helm repo update >/dev/null
	helm template cilium cilium/cilium \
		--version $(shell curl -sSf https://raw.githubusercontent.com/cybozu-go/neco/release/artifacts.go | awk '/"cilium"/ {match($$6, /[0-9.]+/); print substr($$6,RSTART,RLENGTH)}' | cut -d \. -f 1,2,3) \
		--namespace=kube-system \
		--values hubble/base/values.yaml > hubble/base/upstream/hubble.yaml

.PHONY: update-kube-metrics-adapter
update-kube-metrics-adapter:
	$(call get-latest-tag,kube-metrics-adapter)
	rm -rf /tmp/kube-metrics-adapter
	cd /tmp; git clone -b $(call upstream-tag,$(latest_tag)) --depth 1 https://github.com/zalando-incubator/kube-metrics-adapter
	helm template \
		--set namespace=kube-metrics-adapter \
		--set enableExternalMetricsApi=true \
		--set service.internalPort=6443 \
		--set replicas=2 \
		/tmp/kube-metrics-adapter/docs/helm > kube-metrics-adapter/base/upstream/manifest.yaml
	rm -rf /tmp/kube-metrics-adapter
	sed -i 's/newTag: .*/newTag: $(latest_tag)/' kube-metrics-adapter/base/kustomization.yaml

.PHONY: update-kube-state-metrics
update-kube-state-metrics:
	$(call get-latest-tag,kube-state-metrics)
	rm -rf /tmp/kube-state-metrics
	cd /tmp; git clone --depth 1 -b $(call upstream-tag,$(latest_tag)) https://github.com/kubernetes/kube-state-metrics
	rm -f monitoring/base/kube-state-metrics/*
	cp /tmp/kube-state-metrics/examples/standard/* monitoring/base/kube-state-metrics
	rm -rf /tmp/kube-state-metrics
	sed -i -E '/newName:.*kube-state-metrics$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' monitoring/base/kustomization.yaml

.PHONY: update-kube-storage-version-migrator
update-kube-storage-version-migrator:
	$(call get-latest-tag,storage-version-migration-migrator)
	rm -rf /tmp/kube-storage-version-migrator
	cd /tmp; git clone --depth 1 -b $(call upstream-tag,$(latest_tag)) https://github.com/kubernetes-sigs/kube-storage-version-migrator
	$(MAKE) -C /tmp/kube-storage-version-migrator local-manifests REGISTRY=quay.io/cybozu NAMESPACE=kube-storage-version-migrator VERSION=$(latest_tag)
	rm -f kube-storage-version-migrator/base/upstream/*
	cp /tmp/kube-storage-version-migrator/manifests.local/*.yaml kube-storage-version-migrator/base/upstream/
	rm -rf /tmp/kube-storage-version-migrator

.PHONY: update-local-pv-provisioner
update-local-pv-provisioner:
	$(call get-latest-tag,local-pv-provisioner)
	sed -i -E 's,image: quay.io/cybozu/local-pv-provisioner:.*$$,image: quay.io/cybozu/local-pv-provisioner:$(latest_tag),' local-pv-provisioner/base/daemonset.yaml

.PHONY: update-logging-loki
update-logging-loki:
	$(call get-latest-tag,loki)
	rm -rf /tmp/loki
	mkdir /tmp/loki
	cd /tmp/loki; \
	tk init && \
	tk env add environments/loki --namespace=logging && \
	tk env add environments/loki-canary --namespace=logging && \
	jb install github.com/grafana/loki/production/ksonnet/loki@$(call upstream-tag,$(latest_tag)) && \
	jb install github.com/grafana/loki/production/ksonnet/loki-canary@$(call upstream-tag,$(latest_tag)) && \
	jb install github.com/jsonnet-libs/k8s-alpha/$(JSONNET_LIBS_K8S_ALPHA_VERSION) && \
	echo "import 'github.com/jsonnet-libs/k8s-alpha/$(JSONNET_LIBS_K8S_ALPHA_VERSION)/main.libsonnet'" > lib/k.libsonnet

	cp logging/base/loki/upstream/main.jsonnet /tmp/loki/environments/loki/main.jsonnet
	cp logging/base/loki-canary/main.jsonnet /tmp/loki/environments/loki-canary/main.jsonnet
	rm -rf logging/base/loki/upstream/generated/* logging/base/loki-canary/upstream/*
	cd /tmp/loki && \
	tk export $(shell pwd)/logging/base/loki/upstream/generated environments/loki/ -t '!.*/consul(-sidekick)?' && \
	tk export $(shell pwd)/logging/base/loki-canary/upstream/ environments/loki-canary/

	sed -i -E '/name:.*loki$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' logging/base/loki*/kustomization.yaml

	$(call get-latest-tag,memcached)
	sed -i -E '/name:.*memcached$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' logging/base/loki/kustomization.yaml
	$(call get-latest-tag,memcached-exporter)
	sed -i -E '/name:.*memcached-exporter$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' logging/base/loki/kustomization.yaml

.PHONY: update-logging-promtail
update-logging-promtail: $(KUSTOMIZE)
	$(call get-latest-tag,promtail)
	$(call get-latest-helm-by-app,grafana,https://grafana.github.io/helm-charts,grafana/promtail,$(shell echo $(latest_tag)|sed -E 's/\.[0-9]+$$//'))
	yq eval -i '.helmCharts[0].version = "$(latest_helm)"' logging/base/promtail/upstream/kustomization.yaml
	sed -i -E 's/tag:.*$$/tag: $(latest_tag)/' logging/base/promtail/upstream/values.yaml
	$(KUSTOMIZE) build --enable-helm logging/base/promtail/upstream > logging/base/promtail/promtail.yaml

.PHONY: diff-logging-promtail-config
diff-logging-promtail-config:
	d=$$(mktemp -d); \
	version=$$(yq eval '.helmCharts[0].version' logging/base/promtail/upstream/kustomization.yaml); \
	helm template logging --namespace=logging grafana/promtail --version=$$version | yq eval '.stringData."promtail.yaml" | select(.)' - > $$d/orig.yaml && \
	yq eval '.stringData."promtail.yaml" | select(.)' logging/base/promtail/promtail.yaml > $$d/my.yaml && \
	(diff -u $$d/orig.yaml $$d/my.yaml || true) && \
	rm -r $$d

.PHONY: update-loki-small
update-loki-small: $(KUSTOMIZE)
	$(call get-latest-tag,loki)
	$(call get-latest-helm-by-app,grafana,https://grafana.github.io/helm-charts,grafana/loki,v$(shell echo $(latest_tag)|sed -E 's/\.[0-9]+$$//'))
	yq eval -i '.helmCharts[0].version = "$(latest_helm)"' logging-small/base/loki/kustomization.yaml
	sed -i -E 's/tag:.*$$/tag: $(latest_tag)/' logging-small/base/loki/values.yaml
	$(KUSTOMIZE) build --enable-helm logging-small/base/loki > logging-small/base/loki.yaml

.PHONY: diff-promtail-small-config
diff-promtail-small-config:
	d=$$(mktemp -d); \
	version=$$(yq eval '.helmCharts[0].version' logging-small/base/promtail/kustomization.yaml); \
	helm template logging-small --namespace=logging-small grafana/promtail --version=$$version | yq eval '.stringData."promtail.yaml" | select(.)' - > $$d/orig.yaml && \
	yq eval '.stringData."promtail.yaml" | select(.)' logging-small/base/promtail.yaml > $$d/my.yaml && \
	(diff -u $$d/orig.yaml $$d/my.yaml || true) && \
	rm -r $$d

.PHONY: update-promtail-small
update-promtail-small: $(KUSTOMIZE)
	$(call get-latest-tag,promtail)
	$(call get-latest-helm-by-app,grafana,https://grafana.github.io/helm-charts,grafana/promtail,$(shell echo $(latest_tag)|sed -E 's/\.[0-9]+$$//'))
	yq eval -i '.helmCharts[0].version = "$(latest_helm)"' logging-small/base/promtail/kustomization.yaml
	sed -i -E 's/tag:.*$$/tag: $(latest_tag)/' logging-small/base/promtail/values.yaml
	$(KUSTOMIZE) build --enable-helm logging-small/base/promtail > logging-small/base/promtail.yaml

.PHONY: update-machines-endpoints
update-machines-endpoints:
	$(call get-latest-tag,machines-endpoints)
	sed -i -E 's,image: quay.io/cybozu/machines-endpoints:.*$$,image: quay.io/cybozu/machines-endpoints:$(latest_tag),' bmc-reverse-proxy/base/machines-endpoints/cronjob.yaml
	sed -i -E 's,image: quay.io/cybozu/machines-endpoints:.*$$,image: quay.io/cybozu/machines-endpoints:$(latest_tag),' monitoring/base/machines-endpoints/cronjob.yaml

.PHONY: update-meows
update-meows:
	$(call get-latest-gh,cybozu-go/meows)
	rm -rf /tmp/meows
	cd /tmp; git clone --depth 1 -b "$(latest_gh)" https://github.com/cybozu-go/meows.git
	rm -rf meows/overlays/gcp/upstream/*
	cp -r /tmp/meows/config/* meows/overlays/gcp/upstream
	rm -rf /tmp/meows
	sed -i -E '/name:.*meows-controller$$/!b;n;s/newTag:.*$$/newTag: $(patsubst v%,%,$(latest_gh))/' meows/overlays/gcp/kustomization.yaml
	$(call get-latest-tag,meows-dctest-runner)
	sed -i -E 's,quay.io/cybozu/meows-dctest-runner:.*$$,quay.io/cybozu/meows-dctest-runner:$(latest_tag),' meows/overlays/stage0/runnerpool.yaml
	sed -i -E 's,quay.io/cybozu/meows-dctest-runner:.*$$,quay.io/cybozu/meows-dctest-runner:$(latest_tag),' meows/overlays/stage0/neco-apps-runner.yaml
	$(call get-latest-tag,meows-neco-runner)
	sed -i -E 's,quay.io/cybozu/meows-neco-runner:.*$$,quay.io/cybozu/meows-neco-runner:$(latest_tag),' meows/overlays/gcp/deployment.yaml

.PHONY: update-metallb
update-metallb:
	$(call get-latest-tag,metallb)
	rm -rf /tmp/metallb
	cd /tmp; git clone --depth 1 -b $(call upstream-tag,$(latest_tag)) https://github.com/metallb/metallb
	rm -f metallb/base/upstream/*
	cp /tmp/metallb/manifests/*.yaml metallb/base/upstream
	rm -rf /tmp/metallb
	sed -i -E 's/newTag:.*$$/newTag: $(latest_tag)/' metallb/base/kustomization.yaml

.PHONY: update-moco
update-moco:
	$(call get-latest-helm,moco,https://cybozu-go.github.io/moco)
	yq eval -i '.spec.source.targetRevision = "$(latest_helm)"' argocd-config/base/moco.yaml

.PHONY: update-neco-admission
update-neco-admission:
	$(call get-latest-tag,neco-admission)
	curl -sSLf -o neco-admission/base/upstream/manifests.yaml \
		https://raw.githubusercontent.com/cybozu/neco-containers/main/admission/config/webhook/manifests.yaml
	sed -i -E 's/newTag:.*$$/newTag: $(latest_tag)/' neco-admission/base/kustomization.yaml

.PHONY: update-prometheus-adapter
update-prometheus-adapter:
	$(call get-latest-tag,prometheus-adapter)
	sed -i -E \
		-e 's/^(          tag:).*$$/\1 $(latest_tag)/' \
		-e 's/^(    targetRevision:).*$$/\1 $(CHART_VERSION)/' \
		argocd-config/base/prometheus-adapter.yaml
	rm -rf /tmp/prometheus-adapter

.PHONY: update-pod-security-admission
update-pod-security-admission:
	$(call get-latest-gh,cybozu-go/pod-security-admission)
	curl -sSLf -o pod-security-admission/base/upstream/install.yaml \
		https://github.com/cybozu-go/pod-security-admission/releases/download/$(latest_gh)/install.yaml

.PHONY: update-pushgateway
update-pushgateway:
	$(call get-latest-tag,pushgateway)
	sed -i -E '/name:.*pushgateway$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' monitoring/base/kustomization.yaml

.PHONY: update-pvc-autoresizer
update-pvc-autoresizer:
	sed -i -E \
		-e 's/^(  version:).*$$/\1 $(CHART_VERSION)/' \
		pvc-autoresizer/base/kustomization.yaml

ROOK_IMAGE_VERSION := $(shell awk '/ROOK_IMAGE:/ {print $$2}' rook/base/VERSIONS)
ROOK_CHAET_VERSION := $(shell awk '/ROOK_CHART:/ {print $$2}' rook/base/VERSIONS)
.PHONY: update-rook
update-rook:
	sed -i -E \
		-e 's/^(  tag:).*$$/\1 $(ROOK_IMAGE_VERSION)/' \
		rook/base/values.yaml
	curl -L --output rook/base/toolbox/toolbox.yaml \
		https://raw.githubusercontent.com/rook/rook/v$$(echo $(ROOK_IMAGE_VERSION) | cut -d "." -f 1-3)/deploy/examples/toolbox.yaml
	sed -i -E \
		-e 's/^(    newTag:).*$$/\1 $(ROOK_IMAGE_VERSION)/' \
		rook/base/toolbox/kustomization.yaml
	sed -i -E \
		-e 's/(image: quay.io\/cybozu\/rook:).*$$/\1$(ROOK_IMAGE_VERSION)/' \
		rook/base/ceph-extra-exporter/deployment.yaml
	for t in common	ceph-object-store ceph-object-store-clusterrolebinding \
		ceph-poc ceph-poc-clusterrolebinding \
		ceph-ssd ceph-ssd-clusterrolebinding; do \
		sed -i -E \
			-e 's/^(  version:).*$$/\1 v$(ROOK_CHAET_VERSION)/' \
			rook/base/$$t/kustomization.yaml; \
		cp rook/base/values.yaml rook/base/$$t/values.yaml; \
		$(KUSTOMIZE) build --enable-helm rook/base/$$t > rook/base/$$t.yaml; \
	done
	mv rook/base/ceph-poc*.yaml rook/overlays/stage0/ceph-poc/

.PHONY: update-s3gw
update-s3gw:
	$(call get-latest-tag,s3gw)
	sed -i -E 's,quay.io/cybozu/s3gw:.*$$,quay.io/cybozu/s3gw:$(latest_tag),' session-log/base/s3gw.yaml

.PHONY: update-sealed-secrets
update-sealed-secrets:
	$(call get-latest-tag,sealed-secrets)
	curl -sSLf -o sealed-secrets/base/upstream/controller.yaml \
		https://github.com/bitnami-labs/sealed-secrets/releases/download/$(call upstream-tag,$(latest_tag))/controller.yaml
	sed -i -E 's/newTag:.*$$/newTag: $(latest_tag)/' sealed-secrets/base/kustomization.yaml

.PHONY: update-tenet
update-tenet:
	$(call get-latest-helm,tenet,https://cybozu-go.github.io/tenet)
	yq eval -i '.spec.source.targetRevision = "$(latest_helm)"' argocd-config/base/tenet.yaml

.PHONY: update-topolvm
update-topolvm:
	sed -i -E \
		-e 's/^(  version:).*$$/\1 $(CHART_VERSION)/' \
		topolvm/base/kustomization.yaml

.PHONY: update-victoriametrics-operator
update-victoriametrics-operator:
	$(call get-latest-tag,victoriametrics-operator)
	rm -rf /tmp/operator
	cd /tmp; git clone --depth 1 -b $(call upstream-tag,$(latest_tag)) https://github.com/VictoriaMetrics/operator
	rm -rf monitoring/base/victoriametrics/upstream/*
	cp -r /tmp/operator/config/crd /tmp/operator/config/rbac monitoring/base/victoriametrics/upstream/
	rm -rf /tmp/operator
	sed -i -E 's,quay.io/cybozu/victoriametrics-operator:.*$$,quay.io/cybozu/victoriametrics-operator:$(latest_tag),' monitoring/base/victoriametrics/operator.yaml

.PHONY: update-victoriametrics
update-victoriametrics:
	$(call get-latest-tag,victoriametrics-vmalert)
	sed -i -E '/name: VM_VMALERTDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,victoriametrics-vmagent)
	sed -i -E '/name: VM_VMAGENTDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,victoriametrics-vmsingle)
	sed -i -E '/name: VM_VMSINGLEDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,victoriametrics-vmselect)
	sed -i -E '/name: VM_VMCLUSTERDEFAULT_VMSELECTDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,victoriametrics-vmstorage)
	sed -i -E '/name: VM_VMCLUSTERDEFAULT_VMSTORAGEDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,victoriametrics-vminsert)
	sed -i -E '/name: VM_VMCLUSTERDEFAULT_VMINSERTDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,alertmanager)
	sed -i -E '/name: VM_VMALERTMANAGER_ALERTMANAGERVERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,configmap-reload)
	sed -i -E 's,quay.io/cybozu/configmap-reload:.*$$,quay.io/cybozu/configmap-reload:$(latest_tag),' monitoring/base/victoriametrics/operator.yaml monitoring/base/victoriametrics/vmalertmanager-largeset.yaml monitoring/base/victoriametrics/vmalertmanager-smallset.yaml
	$(call get-latest-tag,prometheus-config-reloader)
	sed -i -E 's,quay.io/cybozu/prometheus-config-reloader:.*$$,quay.io/cybozu/prometheus-config-reloader:$(latest_tag),' monitoring/base/victoriametrics/operator.yaml

# usage: get-latest-tag NAME
define get-latest-tag
$(eval latest_tag := $(shell curl -sSf https://quay.io/api/v1/repository/cybozu/$1/tag/ | jq -r '.tags[] | .name' | awk '/.*\..*\./ {print $$1; exit}'))
endef

# usage: upstream-tag 1.2.3.4
define upstream-tag
$(shell echo $1 | sed -E 's/^(.*)\.[[:digit:]]+$$/v\1/')
endef

# usage get-latest-gh OWNER/REPO
define get-latest-gh
$(eval latest_gh := $(shell curl -sSf https://api.github.com/repos/$1/releases/latest | jq -r '.tag_name'))
endef

# usage get-latest-helm REPO URL
define get-latest-helm
$(eval latest_helm := $(shell helm repo add $1 $2 >/dev/null; helm repo update >/dev/null; helm search repo $1 -o json | jq -r .[0].version))
endef

# usage get-latest-helm REPO URL KEYWORD APP_VERSION
define get-latest-helm-by-app
$(eval latest_helm := $(shell helm repo add $1 $2 >/dev/null; helm repo update >/dev/null; helm search repo $3 -l -o json | jq -r 'map(select(.app_version=="$4"))[0]'.version))
endef

.PHONY: setup
setup:
	# helm
	curl -sSLf -o /tmp/helm.tgz https://get.helm.sh/helm-v$(HELM_VERSION)-linux-amd64.tar.gz
	mkdir -p $$(go env GOPATH)/bin
	tar --strip-components=1 -C $$(go env GOPATH)/bin -xzf /tmp/helm.tgz linux-amd64/helm
	rm -f /tmp/helm.tgz

	# tanka
	./bin/curl-github -sSLf -o $$(go env GOPATH)/bin/tk https://github.com/grafana/tanka/releases/download/v$(TANKA_VERSION)/tk-linux-amd64
	chmod +x $$(go env GOPATH)/bin/tk

	# jb
	go install github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb@latest

	# yq
	./bin/curl-github -sSLf -o /tmp/yq.tar.gz https://github.com/mikefarah/yq/releases/download/v$(YQ_VERSION)/yq_linux_amd64.tar.gz
	tar --strip-components=1 -C $$(go env GOPATH)/bin -xzf /tmp/yq.tar.gz
	mv $$(go env GOPATH)/bin/yq_linux_amd64 $$(go env GOPATH)/bin/yq
	rm -f /tmp/yq.tar.gz
