local utility = import '../../../utility.libsonnet';
local kustomization_template = import 'kustomization.libsonnet';
local namespace_template = import 'namespace.libsonnet';
local tenant_template = import 'tenant-template.libsonnet';
local get_team_devns = function(settings, team)
  std.filter(function(x) std.startsWith(x, 'dev-'), utility.get_team_namespaces(settings, team));
local get_team_files = function(team, dev_namespaces)
  std.foldl(function(x, y) x { [y + '.yaml']: namespace_template(team, y) }, dev_namespaces, {});
local get_tenant_files = function(settings, tenant)
  std.foldl(function(x, y) x { ['tenant-' + tenant + '.yaml']: tenant_template(tenant, settings.tenants[tenant]) }, tenant, {});
function(settings)
  local all_teams_namespaces = utility.get_all_namespaces(settings);
  local all_dev_namespaces = std.filter(function(x) std.startsWith(x, 'dev-'), all_teams_namespaces);
  local teams = utility.get_teams(settings);
  local tenants = utility.get_tenants(settings);
  local team_files = std.map(function(team) get_team_files(team, get_team_devns(settings, team)), teams);
  local tenant_files = std.map(function(tenant) get_tenant_files(settings, tenant), tenants);
  utility.union_map([{
    'kustomization.yaml': kustomization_template(teams, all_dev_namespaces, tenants),
  }] + team_files + tenant_files)
