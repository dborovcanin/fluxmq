# FluxMQ Production Readiness Plan

## Next Up: P0 — Config/Docs Sync, Bugs, and Test Gaps

### P0a — Fix Example Configs (user-facing, quick win)

Example configs use old `plain:` naming but code expects `v3:`/`v5:` for TCP and WebSocket listeners.

- [ ] `examples/config.yaml` — update `plain:` → `v3:`/`v5:`
- [ ] `examples/no-cluster.yaml` — same
- [ ] `examples/tls-server.yaml` — same
- [ ] `web/content/docs/reference/configuration-reference.md` — update to match code

### P0b — Bug Fixes

**Critical:**
- [ ] Add context timeouts to AMQP 0.9.1 cleanup paths (`amqp/broker/channel.go:1341-1375`, `amqp/broker/connection.go:482-512`) — `context.Background()` with no timeout can block shutdown indefinitely
- [ ] Add context timeouts to AMQP 1.0 link attach/detach (`amqp1/broker/link.go:196, 210, 218, 288`) — same issue
- [ ] Clarify Raft transport stubs (`cluster/transport.go:664-682`) — `AppendEntries`, `RequestVote`, `InstallSnapshot` always return failure; determine if dead code or blocking cluster functionality

**High:**
- [ ] Add TTL/eviction to auth identity cache (`broker/authcallout`) — `sync.Map` grows unbounded
- [ ] Log cross-protocol delivery errors instead of silently dropping them (`CrossDeliverFunc` is fire-and-forget)
- [ ] Replace panic in `RefCountedBuffer.Release()` (`mqtt/refbuffer.go:77`) with graceful error handling
- [ ] Remove hardcoded 3s sleep in leader campaign (`cluster/etcd.go:636`) — replace with backoff or remove

**Medium:**
- [ ] Protect AMQP 0.9.1 pending publish state (`pendingMethod`, `pendingHeader`, `pendingBody`) — not synchronized, potential race during multi-frame publish assembly
- [ ] Webhook circuit breaker has no half-open retry state — failed endpoints are permanently dead until manual reload
- [ ] Event hooks are synchronous in publish hot path — slow webhook degrades broker throughput

### P0c — Test Coverage Gaps

**High-risk untested packages:**
- [ ] `amqp1/connection.go` (259 LOC) — protocol header exchange, frame read/write, error conditions
- [ ] `queue/storage/topic.go` (128 LOC) — topic-to-queue routing index, concurrent access
- [ ] `server/otel/metrics.go` (239 LOC) — metric recording, instrumentation init
- [ ] `storage/pool.go` (70 LOC) — message pool lifecycle, concurrent access
- [ ] `server/http/server.go` (195 LOC) — HTTP bridge `/publish` and `/health` endpoints

**Weak coverage to expand:**
- [ ] `amqp/broker` — 3 test files for 2.5k LOC, needs more coverage
- [ ] `queue/` — missing core queue logic tests

---

## P1 — Security Hardening

- Add "production profile" config example with TLS/mTLS/auth callout enabled
- Add admin API protection (mTLS and/or auth middleware) while preserving local-dev h2c profile
- Add OTLP TLS options to remove current exporter `WithInsecure()` dependency

## Future: Hot Reload Scope Expansion

The reload framework (`reload/`, `config/diff.go`) supports extending the set of runtime-safe fields. Candidates for future promotion from restart-required to runtime-safe:

- **Webhook tuning**: workers, queue size, endpoints — requires drain logic for old worker pool
- **Session config**: MaxOfflineQueueSize, DefaultExpiryInterval — affects existing sessions
- **Auth protocol toggles**: needs policy for in-flight connections
- **AsyncFanOut / FanOutWorkers**: goroutine pool lifecycle management

Each requires careful analysis of the impact on active connections before promotion.

## Future: Cluster-wide Config Orchestration

Hot reload currently applies to one node at a time. Cluster-wide orchestration (rolling reload across nodes, config consistency checks) is a later phase.
