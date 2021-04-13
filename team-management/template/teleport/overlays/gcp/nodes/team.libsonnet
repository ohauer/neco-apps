function(team) [
  (if team != 'maneki' then { '$patch': 'delete' } else {})
  + {
    apiVersion: 'apps/v1',
    kind: 'StatefulSet',
    metadata: {
      name: 'node-' + team,
      namespace: 'teleport',
    },
  } + (if team == 'maneki' then
         {
           spec: {
             template: {
               spec: {
                 containers: [
                   {
                     name: 'node-maneki',
                     args: [
                       '--roles=node',
                       '--labels=team=maneki',
                       '--diag-addr=0.0.0.0:3020',
                       '--insecure',
                     ],
                   },
                 ],
               },
             },
           },
         } else {}),
]
