function(name, tenant) [
  {
    apiVersion: 'cattage.cybozu.io/v1beta1',
    kind: 'Tenant',
    metadata: {
      name: name,
    },
    spec: tenant,
  },
]
