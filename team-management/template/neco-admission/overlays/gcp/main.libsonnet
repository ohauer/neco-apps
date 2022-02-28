local config_template = import '../../config.libsonnet';
function(settings)
  {
    'config.yaml': config_template(settings, [
      {
        repositoryPrefix: 'https://github.com/cybozu-go/neco-apps',
        projects: [
          'my-team',
        ],
      },
    ]),
  }
