---
title: Running in Production
description: Production checklist covering performance, scaling, observability, and operational safety
---

# Running in Production

**Last Updated:** 2026-02-05

Production readiness is workload-dependent. Use the benchmark suites in `benchmarks/` and validate on your target hardware and network before making production commitments.

## Benchmarking

- See `benchmarks/README.md` for available benchmarks and how to run them.
- Run benchmarks on the same class of hardware you plan to deploy.
- Capture results with `-benchmem` and keep baselines in version control if needed.

## Practical Tuning Levers

- `max_connections` on every entry in `listeners.*`: protect the broker from excess concurrent connections. Counted on accepted sockets, so a peer that connects without completing a handshake still consumes quota
- `read_timeout` and `write_timeout` on MQTT listener entries: evict peers that stall before a session starts, or that stop reading afterwards. Both default to `60s`
- `session.max_sessions`: cap active MQTT sessions
- `broker.max_message_size`: limit payload size, and with it the memory a peer can make the broker buffer before it is authenticated
- `session.max_offline_queue_size` and `session.max_inflight_messages`: control per-client memory usage
- `session.max_send_queue_size` and `session.disconnect_on_full`: tune slow-subscriber behavior under fan-out
- `queues.*.limits`: bound queue depth, message size, and TTL

## Durability

- `storage.queue_recover_on_startup` is `false` by default, which means a corrupted
  log segment fails startup rather than being silently truncated. Keep it that
  way: it turns silent data loss into a startup error you can act on. Enable it
  deliberately, after taking a backup, when you have decided to discard the
  damaged tail. See [Storage](/reference/configuration-reference).

## OS and Runtime Considerations

- Ensure file descriptor limits are high enough for your target connection count.
- Tune TCP keepalive and timeouts based on your workload (IoT vs low-latency systems).
- Measure with production-like TLS settings if TLS is enabled in production.
- For Go runtime and GC tuning (`GOGC`, `GOMEMLIMIT`), see [Performance Tuning](/deployment/performance-tuning).

## Cluster Scaling

- Clustering spreads sessions and subscriptions across nodes but requires etcd coordination.
- Use multiple nodes to improve availability and distribute connections.
- Queue replication is optional and configured via `cluster.raft`.

## Security Basics

- Prefer listener `tls` mappings; add `client_ca_file` for mTLS.
- If you enable inter-broker transport TLS, configure `cluster.transport.tls_*`.
- Use `ratelimit.*` to protect against connection and message floods.

## Observability

- Enable OpenTelemetry metrics via `telemetry.metrics_enabled` and `telemetry.endpoint`. The exporter starts when either signal is enabled.
- Consider enabling traces only for debugging (`telemetry.traces_enabled`).

For configuration details, see [Configuration reference](/reference/configuration-reference).
