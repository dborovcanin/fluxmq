---
title: Configuration Reference
description: Strict FluxMQ v1 configuration schema
---

# Configuration reference

FluxMQ v1 accepts one strict YAML document. Every file must declare
`version: 1`; unknown, duplicate, legacy, and misplaced keys are rejected with
their YAML path.

Validate configuration without starting a broker:

```console
fluxmq config validate --config /etc/fluxmq/config.yaml
```

Naming a missing file is an error. Starting without `--config` is different: it
starts a loopback-only, in-memory development broker and logs a warning.

## Minimal file

```yaml
version: 1

listeners:
  mqtt:
    - address: ":1883"
      transport: tcp
      versions: ["3.1.1", "5.0"]
  amqp091: []
  amqp1: []

storage:
  type: badger
  data_dir: /var/lib/fluxmq
```

## Stable listeners

`listeners.mqtt`, `listeners.amqp091`, and `listeners.amqp1` are lists. Every
entry has `address`; all limits and timeouts below have normalized defaults.

MQTT entries:

| Key | Values/default |
| --- | --- |
| `transport` | Required: `tcp` or `websocket`. |
| `versions` | Required non-empty subset of `"3.1.1"`, `"5.0"`. Both versions are auto-detected on one listener. |
| `path` | WebSocket path, default `/mqtt`; invalid for TCP. |
| `allowed_origins` | WebSocket origin allowlist; invalid for TCP. |
| `max_connections` | Default `10000`; `0` selects the default. |
| `read_timeout` | Default `60s`. |
| `write_timeout` | Default `60s`. |
| `tls` | Optional TLS mapping described below. |

AMQP 0.9.1 entries select `auth: external|local`. Local authentication requires
`tls.client_ca_file` and principals under `auth.local_principals`. AMQP 0.9.1
and AMQP 1.0 use `max_connections: 10000` and `handshake_timeout: 10s` by
default.

There are no `plain`, `tls`, `mtls`, `local`, `internal`, or `service` slots or
aliases. Add another list entry when another address or policy is required.

### Listener TLS

```yaml
tls:
  cert_file: /run/secrets/server-cert
  key_file: /run/secrets/server-key
  client_ca_file: /run/secrets/client-ca # optional; makes this mTLS
  min_version: "1.2"
  cipher_suites: []
```

`cert_file` and `key_file` are mandatory when `tls` is present. Adding
`client_ca_file` requires and verifies a client certificate.

## Control plane and telemetry

Admin, health, and telemetry are independent top-level sections:

```yaml
admin:
  address: "127.0.0.1:8082"

health:
  enabled: true
  address: "127.0.0.1:8081"

telemetry:
  endpoint: "127.0.0.1:4317"
  service_name: fluxmq
  service_version: "1.0.0"
  metrics_enabled: false
  traces_enabled: false
  trace_sample_rate: 0.1
  insecure: false
  ca_file: ""
  cert_file: ""
  key_file: ""

shutdown_timeout: 30s
```

## Storage

```yaml
storage:
  type: badger       # memory | badger
  data_dir: /var/lib/fluxmq
  badger_sync_writes: true
  queue_recover_on_startup: false
```

`data_dir` is required for Badger and for cluster mode. FluxMQ derives internal
broker, cluster, and experimental queue-Raft paths below it.

## Static cluster

The presence of a non-empty `cluster.members` enables clustering. Every node
uses the same file:

```yaml
cluster:
  members:
    node1: node1.internal
    node2: node2.internal
    node3: node3.internal
  ports:
    etcd_peer: 2380
    transport: 7948
  tls:
    ca_file: /run/secrets/cluster-ca
    cert_file: /run/secrets/cluster-cert
    key_file: /run/secrets/cluster-key
```

Select the process-local member with `--node-id`; if omitted,
`FLUXMQ_NODE_ID` is used. The flag takes precedence, and the selected ID must
exist in `members`.

The broker derives embedded-etcd membership and advertised peer URLs, broker
transport peers, and local data paths. The embedded-etcd client endpoint is
loopback-only. The one cluster identity provides mTLS for embedded-etcd peer and
broker transport traffic. Plaintext requires the explicit development override
`cluster.allow_insecure: true`.

Membership is static for v1. Changing the member map against existing data
fails startup. Transport batching and retained-payload thresholds are internal
and not configurable.

## Experimental features

HTTP and CoAP bridges live only behind explicit gates:

```yaml
experimental:
  http:
    enabled: true
    listeners:
      - address: ":8080"
  coap:
    enabled: false
    listeners: []
```

Queue Raft lives under `experimental.queue_raft`; it is disabled by default,
derives peers from `cluster.members`, and is outside the v1 compatibility
contract. Queue replication settings are rejected unless this gate is enabled.

## Unchanged sections

The `auth`, `hooks`, `webhook`, `queue_manager`, `queues`, `ratelimit`, `session`,
`broker`, and `log` sections retain their existing field structure, but they are
now decoded strictly like the rest of the document.

FluxMQ v1 does not provide aliases, pre-v1 compatibility decoding, environment
interpolation, includes, overlays, or profiles.

## Every key

The table below is generated from the loader's own document types, so it lists
exactly what a v1 file may contain. Omitting a key takes the default shown;
`required` means the file will not load without it, and `—` means the key has
no value until you set one.

<!-- BEGIN GENERATED KEYS -->

| Key | Type | Default |
| --- | --- | --- |
| `admin.address` | string | `127.0.0.1:8082` |
| `auth.external.identity_cache_size` | integer | — |
| `auth.external.identity_cache_ttl` | duration | — |
| `auth.external.protocols{}` | boolean | — |
| `auth.external.timeout` | duration | `0s` |
| `auth.external.transport` | string | — |
| `auth.external.url` | string | — |
| `auth.local_principals[].certificate_uri_san` | string | — |
| `auth.local_principals[].current_secret_file` | string | required |
| `auth.local_principals[].name` | string | required |
| `auth.local_principals[].permissions.publish[].exchange` | string | — |
| `auth.local_principals[].permissions.publish[].routing_key_prefix` | string | — |
| `auth.local_principals[].permissions.publish[].routing_key` | string | — |
| `auth.local_principals[].permissions.subscribe[]` | string | — |
| `auth.local_principals[].previous_secret_file` | string | — |
| `auth.local_principals[].role` | string | — |
| `broker.async_fan_out` | boolean | `false` |
| `broker.fan_out_workers` | integer | `0` |
| `broker.max_message_size` | integer | `1048576` |
| `broker.max_qos` | integer | `2` |
| `broker.max_retained_messages` | integer | `10000` |
| `broker.max_retries` | integer | `0` |
| `broker.retry_interval` | duration | `20s` |
| `cluster.allow_insecure` | boolean | — |
| `cluster.members{}` | string | — |
| `cluster.ports.etcd_peer` | integer | `2380` |
| `cluster.ports.transport` | integer | `7948` |
| `cluster.tls.ca_file` | string | — |
| `cluster.tls.cert_file` | string | — |
| `cluster.tls.key_file` | string | — |
| `experimental.coap.enabled` | boolean | `false` |
| `experimental.coap.listeners[].address` | string | required |
| `experimental.coap.listeners[].tls.cert_file` | string | — |
| `experimental.coap.listeners[].tls.cipher_suites[]` | string | — |
| `experimental.coap.listeners[].tls.client_ca_file` | string | — |
| `experimental.coap.listeners[].tls.key_file` | string | — |
| `experimental.coap.listeners[].tls.min_version` | string | — |
| `experimental.http.enabled` | boolean | `false` |
| `experimental.http.listeners[].address` | string | required |
| `experimental.http.listeners[].tls.cert_file` | string | — |
| `experimental.http.listeners[].tls.cipher_suites[]` | string | — |
| `experimental.http.listeners[].tls.client_ca_file` | string | — |
| `experimental.http.listeners[].tls.key_file` | string | — |
| `experimental.http.listeners[].tls.min_version` | string | — |
| `experimental.queue_raft.ack_timeout` | duration | `5s` |
| `experimental.queue_raft.auto_provision_groups` | boolean | `true` |
| `experimental.queue_raft.distribution_mode` | string | `replicate` |
| `experimental.queue_raft.election_timeout` | duration | `3s` |
| `experimental.queue_raft.enabled` | boolean | `false` |
| `experimental.queue_raft.groups{}.ack_timeout` | duration | — |
| `experimental.queue_raft.groups{}.bind_addr` | string | — |
| `experimental.queue_raft.groups{}.data_dir` | string | — |
| `experimental.queue_raft.groups{}.election_timeout` | duration | — |
| `experimental.queue_raft.groups{}.enabled` | boolean | — |
| `experimental.queue_raft.groups{}.heartbeat_timeout` | duration | — |
| `experimental.queue_raft.groups{}.min_in_sync_replicas` | integer | — |
| `experimental.queue_raft.groups{}.peers{}` | string | — |
| `experimental.queue_raft.groups{}.replication_factor` | integer | — |
| `experimental.queue_raft.groups{}.snapshot_interval` | duration | — |
| `experimental.queue_raft.groups{}.snapshot_threshold` | integer | — |
| `experimental.queue_raft.groups{}.sync_mode` | boolean | — |
| `experimental.queue_raft.heartbeat_timeout` | duration | `1s` |
| `experimental.queue_raft.min_in_sync_replicas` | integer | `2` |
| `experimental.queue_raft.port` | integer | `7100` |
| `experimental.queue_raft.replication_factor` | integer | `3` |
| `experimental.queue_raft.snapshot_interval` | duration | `5m0s` |
| `experimental.queue_raft.snapshot_threshold` | integer | `8192` |
| `experimental.queue_raft.sync_mode` | boolean | `true` |
| `experimental.queue_raft.write_policy` | string | `forward` |
| `health.address` | string | `127.0.0.1:8081` |
| `health.enabled` | boolean | `true` |
| `hooks.events{}` | boolean | — |
| `hooks.fail_mode` | string | — |
| `hooks.protocols{}` | boolean | — |
| `hooks.timeout` | duration | — |
| `hooks.transport` | string | — |
| `hooks.url` | string | — |
| `listeners.amqp091[].address` | string | required |
| `listeners.amqp091[].auth` | string | required |
| `listeners.amqp091[].handshake_timeout` | duration | `10s` |
| `listeners.amqp091[].max_connections` | integer | `10000` |
| `listeners.amqp091[].tls.cert_file` | string | — |
| `listeners.amqp091[].tls.cipher_suites[]` | string | — |
| `listeners.amqp091[].tls.client_ca_file` | string | — |
| `listeners.amqp091[].tls.key_file` | string | — |
| `listeners.amqp091[].tls.min_version` | string | — |
| `listeners.amqp1[].address` | string | required |
| `listeners.amqp1[].handshake_timeout` | duration | `10s` |
| `listeners.amqp1[].max_connections` | integer | `10000` |
| `listeners.amqp1[].tls.cert_file` | string | — |
| `listeners.amqp1[].tls.cipher_suites[]` | string | — |
| `listeners.amqp1[].tls.client_ca_file` | string | — |
| `listeners.amqp1[].tls.key_file` | string | — |
| `listeners.amqp1[].tls.min_version` | string | — |
| `listeners.mqtt[].address` | string | required |
| `listeners.mqtt[].allowed_origins[]` | string | — |
| `listeners.mqtt[].max_connections` | integer | `10000` |
| `listeners.mqtt[].path` | string | `/mqtt` for websocket |
| `listeners.mqtt[].read_timeout` | duration | `1m0s` |
| `listeners.mqtt[].tls.cert_file` | string | — |
| `listeners.mqtt[].tls.cipher_suites[]` | string | — |
| `listeners.mqtt[].tls.client_ca_file` | string | — |
| `listeners.mqtt[].tls.key_file` | string | — |
| `listeners.mqtt[].tls.min_version` | string | — |
| `listeners.mqtt[].transport` | string | — |
| `listeners.mqtt[].versions[]` | string | — |
| `listeners.mqtt[].write_timeout` | duration | `1m0s` |
| `log.format` | string | `text` |
| `log.level` | string | `info` |
| `queue_manager.auto_commit_interval` | duration | `5s` |
| `queue_manager.capture_drain_timeout` | duration | — |
| `queue_manager.capture_queue_depth` | integer | — |
| `queue_manager.capture_workers` | integer | — |
| `queues[].dlq.enabled` | boolean | — |
| `queues[].dlq.topic` | string | — |
| `queues[].limits.max_depth` | integer | — |
| `queues[].limits.max_message_size` | integer | — |
| `queues[].limits.message_ttl` | duration | — |
| `queues[].name` | string | required |
| `queues[].primary_group` | string | — |
| `queues[].replication.ack_timeout` | duration | — |
| `queues[].replication.election_timeout` | duration | — |
| `queues[].replication.enabled` | boolean | — |
| `queues[].replication.group` | string | — |
| `queues[].replication.heartbeat_timeout` | duration | — |
| `queues[].replication.min_in_sync_replicas` | integer | — |
| `queues[].replication.mode` | string | — |
| `queues[].replication.replication_factor` | integer | — |
| `queues[].replication.snapshot_interval` | duration | — |
| `queues[].replication.snapshot_threshold` | integer | — |
| `queues[].reserved` | boolean | — |
| `queues[].retention.max_age` | duration | — |
| `queues[].retention.max_length_bytes` | integer | — |
| `queues[].retention.max_length_messages` | integer | — |
| `queues[].retry.initial_backoff` | duration | — |
| `queues[].retry.max_backoff` | duration | — |
| `queues[].retry.max_retries` | integer | — |
| `queues[].retry.multiplier` | number | — |
| `queues[].topics[]` | string | — |
| `queues[].type` | string | — |
| `ratelimit.connection.burst` | integer | `20` |
| `ratelimit.connection.cleanup_interval` | duration | `5m0s` |
| `ratelimit.connection.enabled` | boolean | `true` |
| `ratelimit.connection.rate` | number | `1.6666666666666667` |
| `ratelimit.enabled` | boolean | `false` |
| `ratelimit.message.burst` | integer | `100` |
| `ratelimit.message.enabled` | boolean | `true` |
| `ratelimit.message.rate` | number | `1000` |
| `ratelimit.subscribe.burst` | integer | `10` |
| `ratelimit.subscribe.enabled` | boolean | `true` |
| `ratelimit.subscribe.rate` | number | `100` |
| `session.default_expiry_interval` | integer | `300` |
| `session.disconnect_on_full` | boolean | `false` |
| `session.inflight_overflow` | string | `backpressure` |
| `session.max_inflight_messages` | integer | `256` |
| `session.max_offline_queue_size` | integer | `1000` |
| `session.max_send_queue_size` | integer | `0` |
| `session.max_sessions` | integer | `10000` |
| `session.offline_queue_policy` | string | `evict` |
| `session.pending_queue_size` | integer | `1000` |
| `shutdown_timeout` | duration | `30s` |
| `storage.badger_sync_writes` | boolean | `false` |
| `storage.data_dir` | string | — |
| `storage.queue_recover_on_startup` | boolean | `false` |
| `storage.type` | string | required |
| `telemetry.ca_file` | string | — |
| `telemetry.cert_file` | string | — |
| `telemetry.endpoint` | string | `127.0.0.1:4317` |
| `telemetry.insecure` | boolean | `false` |
| `telemetry.key_file` | string | — |
| `telemetry.metrics_enabled` | boolean | `false` |
| `telemetry.service_name` | string | `fluxmq` |
| `telemetry.service_version` | string | `1.0.0` |
| `telemetry.trace_sample_rate` | number | `0.1` |
| `telemetry.traces_enabled` | boolean | `false` |
| `version` | integer | required |
| `webhook.defaults.circuit_breaker.failure_threshold` | integer | `5` |
| `webhook.defaults.circuit_breaker.reset_timeout` | duration | `1m0s` |
| `webhook.defaults.retry.initial_interval` | duration | `1s` |
| `webhook.defaults.retry.max_attempts` | integer | `3` |
| `webhook.defaults.retry.max_interval` | duration | `30s` |
| `webhook.defaults.retry.multiplier` | number | `2` |
| `webhook.defaults.timeout` | duration | `5s` |
| `webhook.drop_policy` | string | `oldest` |
| `webhook.enabled` | boolean | `false` |
| `webhook.endpoints[].events[]` | string | — |
| `webhook.endpoints[].headers{}` | string | — |
| `webhook.endpoints[].name` | string | — |
| `webhook.endpoints[].retry.initial_interval` | duration | — |
| `webhook.endpoints[].retry.max_attempts` | integer | — |
| `webhook.endpoints[].retry.max_interval` | duration | — |
| `webhook.endpoints[].retry.multiplier` | number | — |
| `webhook.endpoints[].timeout` | duration | — |
| `webhook.endpoints[].topic_filters[]` | string | — |
| `webhook.endpoints[].type` | string | — |
| `webhook.endpoints[].url` | string | — |
| `webhook.include_payload` | boolean | `false` |
| `webhook.queue_size` | integer | `10000` |
| `webhook.shutdown_timeout` | duration | `30s` |
| `webhook.workers` | integer | `5` |

<!-- END GENERATED KEYS -->
