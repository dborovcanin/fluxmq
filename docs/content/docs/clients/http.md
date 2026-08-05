---
title: HTTP
description: Publish messages over the HTTP bridge
---

# HTTP

**Last Updated:** 2026-02-05

FluxMQ includes an HTTP publish bridge.

## Enable

Set `experimental.http.enabled: true` and add a listener below
`experimental.http.listeners`. Add its optional `tls` mapping for TLS or mTLS.

## Publish

`POST /publish` with JSON body:

```json
{"topic":"sensors/temp","payload":"MjIuNQ==","qos":1,"retain":false}
```

`payload` is base64-encoded in JSON.

Example:

```bash
curl -sS -X POST http://localhost:8080/publish \
  -H 'Content-Type: application/json' \
  -d '{"topic":"sensors/temp","payload":"MjIuNQ==","qos":1,"retain":false}'
```

## Health

`GET /health` returns a simple status payload.

## Learn More

- [Publishing messages](/messaging/publishing-messages)
- [Server configuration](/configuration/server)
