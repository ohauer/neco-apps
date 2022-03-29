# logging-small

Loki with RGW as a backend cannot be referred to when Ceph stops. So Loki with TopoLVM as a backend (loki-small) is run so that logs of Ceph can be referred to when Ceph stops.

Loki-small is set up as a different instance from the existing Loki. However, the logs can be referred to by specifying `loki-small` as the data source in the existing Grafana.

## Spec

- Logs of pods in the namespaces matching `ceph-.*` are collected.
- The kernel logs from journals are collected.
- Logs are retained for a week.
- Loki-small is deployed in the monolithic mode. This is because it is used only when Ceph stops and the availability is not very important. The log data persistency is guaranteed by the existing Loki.
- The storage backend to store log data is TopoLVM, not Ceph.
