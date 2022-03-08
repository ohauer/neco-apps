local utility = import '../../utility.libsonnet';
local common_template = import 'common/main.libsonnet';
local kustomization_template = import 'kustomization.libsonnet';
local team_template = import 'team-template/main.libsonnet';
local tenant_template = import 'tenant-template.libsonnet';
local get_tenant_files = function(settings, tenant)
  std.foldl(function(x, y) x { ['tenant-' + tenant + '.yaml']: tenant_template(tenant, settings.tenants[tenant]) }, tenant, {});
function(settings)
  local teams = utility.get_teams(settings);
  local tenants = utility.get_tenants(settings);
  local common_files = utility.prefix_file_names('common', common_template(settings));
  local team_files = [utility.prefix_file_names(x, team_template(settings, x)) for x in teams];
  local tenant_files = std.map(function(tenant) get_tenant_files(settings, tenant), tenants);
  utility.union_map([common_files] + team_files + tenant_files + [{
    'kustomization.yaml': kustomization_template(teams, tenants),
  }])
