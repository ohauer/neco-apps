local utility = import '../../utility.libsonnet';
local common_template = import 'common/main.libsonnet';
local kustomization_template = import 'kustomization.libsonnet';
local team_template = import 'team-template/main.libsonnet';
function(settings)
  local teams = utility.get_teams(settings);
  local common_files = utility.prefix_file_names('common', common_template(settings));
  local team_files = [utility.prefix_file_names(x, team_template(settings, x)) for x in teams];
  utility.union_map([common_files] + team_files + [{
    'kustomization.yaml': kustomization_template(teams),
  }])
