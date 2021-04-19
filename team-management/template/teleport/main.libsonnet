local utility = import '../utility.libsonnet';
local base_template = import 'base/main.libsonnet';
local gcp_template = import 'overlays/gcp/main.libsonnet';
function(teams)
  utility.prefix_file_names('base', base_template(teams)) +
  utility.prefix_file_names('overlays/gcp', gcp_template(teams))
