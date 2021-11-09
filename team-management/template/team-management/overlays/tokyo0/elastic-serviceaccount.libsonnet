function(namespaces)
  std.map(function(x) {
    '$patch': 'delete',
    apiVersion: 'v1',
    kind: 'ServiceAccount',
    metadata: {
      name: 'elastic',
      namespace: x,
    },
  }, namespaces)
