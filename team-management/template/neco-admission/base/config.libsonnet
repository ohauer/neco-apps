local utility = import '../../utility.libsonnet';
function(settings) [{
  ArgoCDApplicationValidator: {
    rules: std.set(
      [
        {
          repository: 'https://github.com/cybozu-go/neco-apps.git',
          projects: [
            'default',
          ],
        },
        {
          repository: 'https://prometheus-community.github.io/helm-charts',
          projects: [
            'default',
          ],
        },
        {
          repositoryPrefix: 'https://github.com/cybozu-private',
          projects: std.set(utility.get_teams(settings) + [
            'default',
            'tenant-apps',
            'tenant-app-of-apps',
          ]),
        },
        {
          repositoryPrefix: 'https://github.com/garoon-private',
          projects: [
            'garoon',
            'maneki',
            'tenant-app-of-apps',
          ],
        },
        {
          repositoryPrefix: 'https://github.com/kintone-private',
          projects: [
            'kintone-neco',
            'maneki',
            'tenant-app-of-apps',
          ],
        },
        {
          repository: 'https://topolvm.github.io/pvc-autoresizer',
          projects: [
            'default',
          ],
        },
        {
          repository: 'https://topolvm.github.io/topolvm',
          projects: [
            'default',
          ],
        },
        {
          repository: 'https://cybozu-go.github.io/accurate',
          projects: [
            'default',
          ],
        },
        {
          repository: 'https://cybozu-go.github.io/moco',
          projects: [
            'default',
          ],
        },
      ],
      function(x) if std.objectHas(x, 'repositoryPrefix') then 'A' + x.repositoryPrefix else 'B' + x.repository
    ),
  },
}]
