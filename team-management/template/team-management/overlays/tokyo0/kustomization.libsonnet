function(teams, all_dev_namespaces) [{
  apiVersion: 'kustomize.config.k8s.io/v1beta1',
  kind: 'Kustomization',
  resources: [
    '../../base',
  ],
  patchesStrategicMerge: std.map(function(x) x + '.yaml', all_dev_namespaces),
}]
