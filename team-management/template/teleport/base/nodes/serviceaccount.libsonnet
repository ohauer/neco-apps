function(teams) std.map(function(x) {
  apiVersion: 'v1',
  kind: 'ServiceAccount',
  metadata: {
    name: 'node-' + x,
    namespace: 'teleport',
    labels: {
      'app.kubernetes.io/name': 'teleport',
    },
  },
}, teams)
