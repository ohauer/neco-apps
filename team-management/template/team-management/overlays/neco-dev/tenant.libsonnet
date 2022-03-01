function(name) [
  {
    apiVersion: 'cattage.cybozu.io/v1beta1',
    kind: 'Tenant',
    metadata: {
      name: name,
    },
    spec: {
      argocd: {
        repositories: ['*'],
      },
    },
  },
]
