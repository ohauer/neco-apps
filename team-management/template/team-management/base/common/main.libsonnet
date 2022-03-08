local utility = import '../../../utility.libsonnet';
local project_template = import 'project.libsonnet';
function(settings)
  local teams = utility.get_teams(settings);
  local tenants = utility.get_tenants(settings);
  {
    'project.yaml': project_template(settings.apps, std.sort(teams + tenants)),
  }
