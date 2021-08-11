local utility = import '../../../utility.libsonnet';
function(settings, team, namespace) [
  local labels = utility.get_team_namespace_labels(settings, team, namespace);
  {
    apiVersion: 'v1',
    kind: 'Namespace',
    metadata: {
      name: namespace,
      [if std.length(labels) > 0 then 'labels']: labels,
    },
  },
  {
    apiVersion: 'rbac.authorization.k8s.io/v1',
    kind: 'RoleBinding',
    metadata: {
      name: team + '-role-binding',
      namespace: namespace,
    },
    roleRef: {
      apiGroup: 'rbac.authorization.k8s.io',
      kind: 'ClusterRole',
      name: 'admin',
    },
    subjects: std.set([
      {
        kind: 'Group',
        name: team,
        apiGroup: 'rbac.authorization.k8s.io',
      },
      {
        kind: 'Group',
        name: 'maneki',
        apiGroup: 'rbac.authorization.k8s.io',
      },
      {
        kind: 'ServiceAccount',
        name: 'node-maneki',
        namespace: 'teleport',
      },
      {
        kind: 'ServiceAccount',
        name: 'node-' + team,
        namespace: 'teleport',
      },
    ], function(x) x.kind + x.name),
  },
]
