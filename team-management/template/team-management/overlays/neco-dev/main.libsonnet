local utility = import '../../../utility.libsonnet';
local kustomization_template = import 'kustomization.libsonnet';
local tenant_template = import 'tenant-template.libsonnet';
local get_tenant_files = function(settings, tenant)
  std.foldl(function(x, y) x { ['tenant-' + tenant + '.yaml']: tenant_template(tenant) }, tenant, {});
function(settings)
  local tenants = utility.get_tenants(settings);
  local tenant_files = std.map(function(tenant) get_tenant_files(settings, tenant), tenants);
  utility.union_map([{
    'kustomization.yaml': kustomization_template(settings.repositories, tenants),
  }] + tenant_files)
