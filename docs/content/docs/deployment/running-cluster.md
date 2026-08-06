---
title: Running a Cluster
description: Run the secure static three-node v1 example
---

# Running a cluster

FluxMQ v1 uses static membership and one shared YAML manifest. The Docker
example provides three brokers on a bridge network with identical container
ports, different `--node-id` values, and separate persistent volumes.

Provision a CA and one cluster identity whose certificate is valid for all
member hostnames, then set:

```console
export FLUXMQ_CLUSTER_CA_FILE=/secure/cluster-ca.pem
export FLUXMQ_CLUSTER_CERT_FILE=/secure/cluster-cert.pem
export FLUXMQ_CLUSTER_KEY_FILE=/secure/cluster-key.pem
```

Validate the shared file and render Compose before startup:

```console
./build/fluxmq config validate \
  --config deployments/cluster/config/cluster.yaml \
  --node-id node1
docker compose -f deployments/cluster/docker-compose.yaml config --quiet
```

Start and stop the cluster:

```console
make docker-cluster-up
make docker-cluster-down
```

The same manifest is mounted into every broker. `--node-id` takes precedence
over `FLUXMQ_NODE_ID` and must name a key in `cluster.members`.

Size the cluster before it holds data. Membership is fixed for the life of a
node's data directory, so adding or removing a node in place is not supported in
v1 — see [cluster membership](/docs/configuration/clustering) for why the
restart fails and what the options are.

The embedded-etcd peer and broker transport ports are `2380` and `7948` inside
every container. FluxMQ derives advertised endpoints and peer maps from the
member hostnames. Its embedded-etcd client endpoint remains on loopback.

Cluster mTLS is mandatory. `cluster.allow_insecure: true` exists only for
explicit development use. Membership is static: changing the member map
against a populated volume fails startup; use the original map or fresh data.

Queue Raft is not part of the stable cluster contract. It remains disabled
below `experimental.queue_raft` and derives any experimental peers from the
same member map.
