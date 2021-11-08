function(team, namespace) [
  {
    '$patch': 'delete',
    apiVersion: 'v1',
    kind: 'Namespace',
    metadata: {
      name: namespace,
    },
  },
  {
    '$patch': 'delete',
    apiVersion: 'rbac.authorization.k8s.io/v1',
    kind: 'RoleBinding',
    metadata: {
      name: team + '-role-binding',
      namespace: namespace,
    },
  },
]
