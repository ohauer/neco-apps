function(team) [{
  version: 'v3',
  kind: 'role',
  metadata: {
    name: team,
  },
  spec: {
    allow: {
      app_labels: if team == 'csa' then {
        team: [team, 'neco'],
      } else {
        team: team,
      },
      kubernetes_groups: [
        team,
      ],
      logins: [
        'cybozu',
      ],
      node_labels: {
        team: team,
      },
      rules: [],
    },
    deny: {
      logins: null,
    },
    options: {
      cert_format: 'standard',
      forward_agent: true,
      max_session_ttl: '30h0m0s',
      port_forwarding: true,
    },
  },
}]
