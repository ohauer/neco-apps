local utility = import '../../../utility.libsonnet';
local nodes_template = import 'nodes/main.libsonnet';
local kustomization_template = import 'kustomization.libsonnet';
local utility = import '../../../utility.libsonnet';
function(teams)
  utility.prefix_file_names('nodes', nodes_template(teams)) + {
    'kustomization.yaml': kustomization_template(teams),
  }
