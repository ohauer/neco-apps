function(name, tenant) [
  {
    apiVersion: 'cattage.cybozu.io/v1beta1',
    kind: 'Tenant',
    metadata: {
      name: name,
    },
    spec: {
      rootNamespaces: std.filter(function(x) !std.startsWith(x.name, 'dev-'), tenant.rootNamespaces),
    },
  },
]
