local utility = import '../../../utility.libsonnet';
local role_template = import 'team-role.libsonnet';
function(teams)
  utility.union_map(std.map(function(x) { [x + '-role.yaml']: role_template(x) }, teams))
