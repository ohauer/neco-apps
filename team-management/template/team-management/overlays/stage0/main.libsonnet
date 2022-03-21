local utility = import '../../../utility.libsonnet';
local kustomization_template = import 'kustomization.libsonnet';
local team_template = import 'team-template/main.libsonnet';
function(settings)
  local teams = utility.get_migrating_teams(settings);
  local team_files = [utility.prefix_file_names(x, team_template(settings, x)) for x in teams];
  utility.union_map(team_files + [{
    'kustomization.yaml': kustomization_template(settings, teams),
  }])
