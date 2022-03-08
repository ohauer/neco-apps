function(repositories, tenants) [{
  apiVersion: 'kustomize.config.k8s.io/v1beta1',
  kind: 'Kustomization',
  resources: [
    '../../base',
  ],
  patchesStrategicMerge: std.map(function(x) 'tenant-' + x + '.yaml', tenants),
}]
