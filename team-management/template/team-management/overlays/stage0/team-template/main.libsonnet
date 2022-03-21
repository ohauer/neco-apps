local utility = import '../../../../utility.libsonnet';
local namespace_template = import 'namespace.libsonnet';
local project_template = import 'project.libsonnet';
function(settings, team)
  local namespaces = utility.get_team_namespaces(settings, team);
  {
    'project.yaml': project_template(team),
  } + std.foldl(function(x, y) x { [y + '.yaml']: namespace_template(settings, team, y) }, namespaces, {})
