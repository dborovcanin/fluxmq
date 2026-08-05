---
title: CLI Reference
description: Command-line flags for starting FluxMQ
---

# CLI Reference

**Last Updated:** 2026-02-05

## fluxmq

```bash
./build/fluxmq [--config /path/to/config.yaml] [--node-id member-id]
./build/fluxmq config validate --config /path/to/config.yaml [--node-id member-id]
```

### Flags

- `--config` Path to a v1 YAML configuration file. If omitted, FluxMQ starts a
  loopback-only, in-memory development broker and prints a warning. An explicitly
  named missing file is an error.
- `--node-id` Local member selected from `cluster.members`. It overrides
  `FLUXMQ_NODE_ID`.

## Examples

```bash
./build/fluxmq
./build/fluxmq --config examples/config.yaml
./build/fluxmq config validate --config examples/production.yaml
```
