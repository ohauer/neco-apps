local utility = import '../utility.libsonnet';
local base_template = import 'base/main.libsonnet';
local neco_dev_template = import 'overlays/neco-dev/main.libsonnet';
local osaka0_template = import 'overlays/osaka0/main.libsonnet';
local tokyo0_template = import 'overlays/tokyo0/main.libsonnet';
function(settings)
  local base_files = utility.prefix_file_names('base', base_template(settings));
  local neco_dev_files = utility.prefix_file_names('overlays/neco-dev', neco_dev_template(settings));
  local osaka0_files = utility.prefix_file_names('overlays/osaka0', osaka0_template());
  local tokyo0_files = utility.prefix_file_names('overlays/tokyo0', tokyo0_template(settings));
  utility.union_map([base_files] + [neco_dev_files] + [osaka0_files] + [tokyo0_files])
