local loki = import 'loki/loki.libsonnet';

loki {
  _config+:: {
    namespace: 'logging',

    storage_backend: 's3',
    s3_access_key: '${AWS_ACCESS_KEY_ID}',
    s3_secret_access_key: '${AWS_SECRET_ACCESS_KEY}',
    s3_address: '${BUCKET_HOST}',
    s3_bucket_name: '${BUCKET_NAME}',
    s3_path_style: true,

    boltdb_shipper_shared_store: 's3',

    ingester_pvc_class: 'ceph-ssd-block',

    querier_pvc_class: 'ceph-ssd-block',
    // current index size is ~500MiB per day.
    // by default 10Gi, querier becames disk full if searchs are executed for 20 days.
    // I assume 100Gi is sufficient, it allows us to run query for 200 days.
    querier_pvc_size: '100Gi',

    compactor_pvc_class: 'ceph-ssd-block',

    commonArgs+: {
      'config.expand-env': 'true',
    },

    replication_factor: 3,

    loki+: {
      auth_enabled: false,

      chunk_store_config+: {
        chunk_cache_config+: {
          memcached_client+: {
            host: 'memcached-old.logging.svc.cluster.local'
          },
        },
      },

      frontend+: {
        tail_proxy_url: 'http://querier-old.logging.svc:3100'
      },

      frontend_worker+: {
        frontend_address: 'query-frontend-old.logging.svc.cluster.local:9095'
      },

      ingester+: {
        lifecycler+: {
          ring+: {
            kvstore: std.mergePatch(super.kvstore, {
              store: 'memberlist',
              consul: null
            }),
          },
        },
      },

      distributor+: {
        ring+: {
          kvstore: std.mergePatch(super.kvstore, {
            store: 'memberlist',
            consul: null
          }),
        },
      },

      memberlist+: {
        abort_if_cluster_join_fails: false,
        bind_port: 7946,
        join_members: ['loki-gossip-ring-old.logging.svc:7946'],
        retransmit_factor: 2,
        gossip_interval: '5s',
        stream_timeout: '5s'
      },

      query_range+: {
        results_cache+: {
          cache+: {
            memcached_client+: {
              host: 'memcached-frontend-old.logging.svc.cluster.local'
            },
          },
        },
      },

      schema_config+: {
        configs: [
          x {
            object_store: 's3',
            index+: {
              prefix: 'index_'
            },
          }
          for x in super.configs
        ],
      },

      storage_config+: {
        index_queries_cache_config+: {
          memcached_client+: {
            host: 'memcached-index-queries-old.logging.svc.cluster.local'
          },
        },
      },

      limits_config+: {
        # In default, its value is 10.
        # loki-canary can not use tail API due to the limit.
        # We are not sure what the appropriate value is.
        max_concurrent_tail_requests: 1000
      },
    },
  },

  _images+:: {
    memcached: 'quay.io/cybozu/memcached',
    memcachedExporter: 'quay.io/cybozu/memcached-exporter',
    loki: 'quay.io/cybozu/loki'
  },

  compactor_args+:: {
    'config.expand-env': 'true',
  },
}
