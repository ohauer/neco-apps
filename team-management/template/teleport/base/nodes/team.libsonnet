function(team) [{
  apiVersion: 'apps/v1',
  kind: 'StatefulSet',
  metadata: {
    name: 'node-' + team,
    namespace: 'teleport',
    labels: {
      'app.kubernetes.io/name': 'teleport',
      'app.kubernetes.io/component': 'node',
    },
    annotations: {
      'argocd.argoproj.io/sync-wave': '1',
    },
  },
  spec: {
    selector: {
      matchLabels: {
        'app.kubernetes.io/name': 'teleport',
        'app.kubernetes.io/component': 'node',
        'teleport-node': team,
      },
    },
    replicas: 1,
    serviceName: 'node-' + team,
    template: {
      metadata: {
        labels: {
          'app.kubernetes.io/name': 'teleport',
          'app.kubernetes.io/component': 'node',
          'teleport-node': team,
        },
        annotations: {
          'prometheus.io/port': '3020',
        },
      },
      spec: {
        automountServiceAccountToken: true,
        containers: [
          {
            name: 'node-' + team,
            image: 'quay.io/cybozu/teleport-node',
            args: [
              '--roles=node',
              '--labels=team=' + team,
              '--diag-addr=0.0.0.0:3020',
            ],
            livenessProbe: {
              httpGet: {
                port: 3020,
                path: '/healthz',
              },
              initialDelaySeconds: 5,
              periodSeconds: 5,
            },
            ports: [
              {
                name: 'metrics',
                containerPort: 3020,
              },
            ],
            volumeMounts: [
              {
                mountPath: '/etc/teleport',
                name: 'teleport-node-secret',
                readOnly: true,
              },
              {
                mountPath: '/var/lib/teleport',
                name: 'teleport-storage',
              },
              {
                mountPath: '/home/cybozu',
                name: 'home-dir',
              },
            ],
            resources: {
              requests: {
                memory: '256Mi',
              },
            },
          },
        ],
        securityContext: {
          runAsNonRoot: true,
          runAsUser: 10000,
        },
        volumes: [
          {
            name: 'teleport-node-secret',
            secret: {
              secretName: 'teleport-node-secret-20211130',
            },
          },
          {
            name: 'teleport-storage',
            emptyDir: {},
          },
          {
            name: 'home-dir',
            ephemeral: {
              volumeClaimTemplate: {
                spec: {
                  accessModes: [
                    'ReadWriteOnce',
                  ],
                  storageClassName: 'topolvm-provisioner',
                  resources: {
                    requests: {
                      storage: '10Gi',
                    },
                  },
                },
              },
            },
          },
        ],
        serviceAccountName: 'node-' + team,
      },
    },
  },
}]
