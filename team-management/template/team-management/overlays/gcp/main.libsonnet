local kustomization_template = import 'kustomization.libsonnet';
function() {
  'kustomization.yaml': kustomization_template(),
}
