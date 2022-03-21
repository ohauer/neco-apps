function(repositories, tenants) [{
  apiVersion: 'kustomize.config.k8s.io/v1beta1',
  kind: 'Kustomization',
  resources: [
    '../stage0',
  ],
  patchesStrategicMerge: std.map(function(x) 'tenant-' + x + '.yaml', tenants),
}]
