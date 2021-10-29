local utility = import '../utility.libsonnet';
local base_template = import 'base/main.libsonnet';
function(settings)
  utility.prefix_file_names('base', base_template(settings))
