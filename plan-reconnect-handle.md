# Plan: Server-Initiated Consumer Cancellation

## Context

When the queue manager removes a consumer due to stale heartbeat timeout, the AMQP 0.9.1 client is never notified. The TCP connection and AMQP channel stay open; the delivery channel just goes silent. The client cannot distinguish "no messages" from "consumer was reaped." `AutoReconnect` never fires.

FluxMQ already advertises `consumer_cancel_notify: true` in the AMQP handshake (`amqp/broker/connection.go:139`) and the `BasicCancel` frame is fully implemented in the codec (`amqp/codec/methods.go:1423-1476`). The `amqp091-go` client library supports `NotifyCancel`. We just need to wire them together.

## Approach

Add a callback on the queue manager that fires when consumers are removed by the stale cleanup loop. The AMQP broker registers this callback and sends `basic.cancel` frames to affected clients. The FluxMQ AMQP client listens for these cancellations and triggers reconnection.

## Changes

### 1. `queue/manager.go` — Add callback to Config, invoke on cleanup

- Add `OnConsumerRemoved func(queueName, groupID string, consumerIDs []string)` field to `Config` struct (around line 90)
- In `cleanupStaleConsumers()` (line 1483), after the existing log at line 1500, invoke the callback with the removed consumer IDs

### 2. `amqp/broker/channel.go` — Server-initiated cancel

- Add `cancelConsumerByQueue(queueName, groupID string)` method
- Finds consumers matching queue+group, removes from channel's map, sends `BasicCancel{ConsumerTag: tag, NoWait: true}` frame
- Does NOT call `qm.Unsubscribe` (consumer already removed by cleanup)

### 3. `amqp/broker/connection.go` — Delegate to channels

- Add `cancelConsumerByQueue(queueName, groupID string)` method
- Iterates all channels, delegates to each

### 4. `amqp/broker/broker.go` — Broker-level API

- Add `CancelConsumer(clientID, queueName, groupID string)` — extracts connID, looks up connection, delegates
- Add `CancelConsumers(queueName, groupID string, clientIDs []string)` — iterates consumer IDs, filters AMQP091 clients, delegates

### 5. `cmd/main.go` — Wire the callback

- Before `queue.NewManager` (line 502), set:
  ```go
  queueCfg.OnConsumerRemoved = func(queueName, groupID string, consumerIDs []string) {
      amqp091Broker.CancelConsumers(queueName, groupID, consumerIDs)
  }
  ```

### 6. `client/amqp/options.go` — Add user callback

- Add `OnConsumerCancelled func(consumerTag string)` to `Options`
- Add `SetOnConsumerCancelled` setter

### 7. `client/amqp/client.go` — Listen for server cancellations

- Add `watchCancelNotifications(subCh *amqp091.Channel)` — uses `subCh.NotifyCancel()`, calls `OnConsumerCancelled` callback, then triggers `handleDisconnect` which leads to `startReconnect` → `resubscribeAll`
- Call `watchCancelNotifications` from `connectOnce()` after sub channel creation

### 8. Tests

- **`amqp/broker/channel_test.go`**: `cancelConsumerByQueue` removes consumer, writes `BasicCancel` frame, skips non-matching consumers
- **`queue/manager_test.go`**: `OnConsumerRemoved` fires with correct args when stale consumers are cleaned up
- **`client/amqp/client_api_test.go`**: `OnConsumerCancelled` callback fires; reconnect triggers on cancel notification

## Implementation Order

1. Channel → Connection → Broker (steps 2-4, bottom-up)
2. Queue manager callback (step 1)
3. Wiring (step 5)
4. Client-side (steps 6-7)
5. Tests (step 8)

## Verification

1. `go build ./...`
2. `go test -race ./queue/...`
3. `go test -race ./amqp/broker/...`
4. `go test -race ./client/amqp/...`
5. Manual: start FluxMQ, connect supermq, wait for consumer timeout → client should log cancel and reconnect
