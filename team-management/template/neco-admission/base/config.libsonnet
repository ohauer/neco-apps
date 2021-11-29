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
          repository: 'https://github.com/cybozu-private/csa-apps.git',
          projects: [
            'csa',
          ],
        },
        {
          repository: 'https://github.com/cybozu-private/neco-apps-secret.git',
          projects: [
            'default',
          ],
        },
        {
          repository: 'https://github.com/cybozu-private/neco-tenant-apps.git',
          projects: [
            'tenant-apps',
          ],
        },
        {
          repositoryPrefix: 'https://github.com/cybozu-private',
          projects: std.filter(function(x) x != 'csa', utility.get_teams(settings) + [
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
