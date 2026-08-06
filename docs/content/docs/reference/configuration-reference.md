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
  enabled: false
  endpoint: "127.0.0.1:4317"
  service_name: fluxmq
  service_version: "1.0.0"
  metrics_enabled: true
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
