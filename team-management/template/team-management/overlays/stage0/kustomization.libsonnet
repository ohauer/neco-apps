local utility = import '../../../utility.libsonnet';
function(settings, teams) [{
  apiVersion: 'kustomize.config.k8s.io/v1beta1',
  kind: 'Kustomization',
  resources: std.set([
    '../../base',
  ]),
  patchesStrategicMerge: std.flattenArrays(std.map(function(x) ['./' + x + '/' + y + '.yaml' for y in utility.get_team_namespaces(settings, x)] + ['./' + x + '/project.yaml'], teams)),
}]
