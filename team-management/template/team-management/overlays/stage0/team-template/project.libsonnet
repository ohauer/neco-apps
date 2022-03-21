function(team) [
  {
    '$patch': 'delete',
    apiVersion: 'argoproj.io/v1alpha1',
    kind: 'AppProject',
    metadata: {
      name: team,
      namespace: 'argocd',
    },
  },
]
