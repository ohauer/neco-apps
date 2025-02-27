function(team, namespaces) [
  {
    apiVersion: 'argoproj.io/v1alpha1',
    kind: 'AppProject',
    metadata: {
      name: team,
      namespace: 'argocd',
    },
    spec: {
      sourceRepos: [
        '*',
      ],
      destinations: std.set([
        {
          namespace: 'sandbox',
          server: '*',
        },
        {
          namespace: 'dev-*',
          server: '*',
        },
      ] + std.map(function(x) {
        namespace: x,
        server: '*',
      }, std.filter(function(x) !std.startsWith(x, 'dev-'), namespaces)), function(x) x.namespace),
      namespaceResourceBlacklist: [
        {
          group: '',
          kind: 'ResourceQuota',
        },
        {
          group: '',
          kind: 'LimitRange',
        },
        {
          group: 'networking.k8s.io',
          kind: 'NetworkPolicy',
        },
      ],
      orphanedResources: {
        warn: false,
      },
      roles: [
        {
          name: 'admin',
          groups: std.set([
            'cybozu-private:' + team,
          ] + if team != 'csa' then [
            'cybozu-private:maneki',
          ] else []),
          policies: [
            std.strReplace('p, proj:<team>:admin, applications, *, <team>/*, allow', '<team>', team),
          ],
        },
      ],
    },
  },
]
