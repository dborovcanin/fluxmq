---
title: Storage
description: Configure FluxMQ memory or Badger storage
---

# Storage

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
  sync_writes: true
  recover_on_startup: false
```

- `type` is `memory` or `badger`.
- `data_dir` is required for Badger and cluster mode.
- `sync_writes` requests synchronous Badger writes.
- `recover_on_startup` enables explicit startup recovery.

FluxMQ derives implementation-specific broker and per-node cluster directories
under `storage.data_dir`; those paths are not part of the public schema.
