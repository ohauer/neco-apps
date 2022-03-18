function(name, tenant) [
  {
    apiVersion: 'cattage.cybozu.io/v1beta1',
    kind: 'Tenant',
    metadata: {
      name: name,
    },
    spec: {
      argocd: tenant.argocd,
      rootNamespaces: tenant.rootNamespaces,
      delegates: if std.objectHas(tenant, 'delegates') then tenant.delegates else [],
    },
  },
]
