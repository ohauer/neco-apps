local utility = import '../../../../utility.libsonnet';
local kustomization_template = import 'kustomization.libsonnet';
local app_template = import 'template-apps.libsonnet';
local generate_app_content = function(settings, app, overlay)
  local info = utility.get_app(settings, app);
  // TODO: revert here when app has been switched completely.
  // Workaround for switching app with overlays.
  local repo = if overlay == 'stage0' && info.repo == 'https://github.com/garoon-private/garoon-apps.git' then 'https://github.com/cybozu-private/garoon-apps.git' else info.repo;
  app_template(app, repo, overlay, info.destinations[overlay]);
function(settings, overlay)
  local apps = utility.get_destination_apps(settings, overlay);
  {
    'kustomization.yaml': kustomization_template(apps),
  } + utility.union_map(std.map(function(x) { [x + '.yaml']: generate_app_content(settings, x, overlay) }, apps))
