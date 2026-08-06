---
title: Listeners and Control Endpoints
description: Configure stable protocol listeners, admin, health, and telemetry
---

# Listeners and control endpoints

FluxMQ v1 uses typed listener lists. One MQTT TCP listener can accept MQTT
3.1.1 and 5.0 by auto-detecting the protocol header:

```yaml
version: 1
listeners:
  mqtt:
    - address: ":1883"
      transport: tcp
      versions: ["3.1.1", "5.0"]
  amqp091:
    - address: ":5682"
      auth: external
  amqp1:
    - address: ":5672"
```

Limits and timeouts have defaults. Add an optional `tls` mapping for TLS; add
`client_ca_file` inside it for mTLS. AMQP 0.9.1 local-principal listeners use
`auth: local` and require mTLS.

Admin, health, telemetry, and shutdown are not listener slots:

```yaml
admin:
  address: "127.0.0.1:8082"
health:
  enabled: true
  address: "127.0.0.1:8081"
telemetry:
  metrics_enabled: false
  endpoint: "127.0.0.1:4317"
shutdown_timeout: 30s
```

HTTP and CoAP bridge listeners are experimental and must be configured below
`experimental.http` or `experimental.coap` with `enabled: true`.

See the [configuration reference](/docs/reference/configuration-reference) for
all listener fields and defaults.
