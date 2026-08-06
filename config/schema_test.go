// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// v1Schema is the frozen set of YAML keys the v1 configuration accepts, as
// dotted paths from the document root. Sequence elements are addressed with
// [], and a map whose keys an operator chooses is addressed with {}.
//
// Several sections are decoded straight into runtime structs, so a Go field
// rename there silently renames a configuration key. This test is the thing
// that stops that: adding a key is an additive change and only needs a line
// here, but renaming or removing one breaks every deployed configuration and
// should fail loudly.
var v1Schema = []string{
	"admin.address",
	"auth.external.identity_cache_size",
	"auth.external.identity_cache_ttl",
	"auth.external.protocols{}",
	"auth.external.timeout",
	"auth.external.transport",
	"auth.external.url",
	"auth.local_principals[].certificate_uri_san",
	"auth.local_principals[].current_secret_file",
	"auth.local_principals[].name",
	"auth.local_principals[].permissions.publish[].exchange",
	"auth.local_principals[].permissions.publish[].routing_key",
	"auth.local_principals[].permissions.publish[].routing_key_prefix",
	"auth.local_principals[].permissions.subscribe[]",
	"auth.local_principals[].previous_secret_file",
	"auth.local_principals[].role",
	"broker.async_fan_out",
	"broker.fan_out_workers",
	"broker.max_message_size",
	"broker.max_qos",
	"broker.max_retained_messages",
	"broker.max_retries",
	"broker.retry_interval",
	"cluster.allow_insecure",
	"cluster.members{}",
	"cluster.ports.etcd_peer",
	"cluster.ports.transport",
	"cluster.tls.ca_file",
	"cluster.tls.cert_file",
	"cluster.tls.key_file",
	"experimental.coap.enabled",
	"experimental.coap.listeners[].address",
	"experimental.coap.listeners[].tls.cert_file",
	"experimental.coap.listeners[].tls.cipher_suites[]",
	"experimental.coap.listeners[].tls.client_ca_file",
	"experimental.coap.listeners[].tls.key_file",
	"experimental.coap.listeners[].tls.min_version",
	"experimental.http.enabled",
	"experimental.http.listeners[].address",
	"experimental.http.listeners[].tls.cert_file",
	"experimental.http.listeners[].tls.cipher_suites[]",
	"experimental.http.listeners[].tls.client_ca_file",
	"experimental.http.listeners[].tls.key_file",
	"experimental.http.listeners[].tls.min_version",
	"experimental.queue_raft.ack_timeout",
	"experimental.queue_raft.auto_provision_groups",
	"experimental.queue_raft.distribution_mode",
	"experimental.queue_raft.election_timeout",
	"experimental.queue_raft.enabled",
	"experimental.queue_raft.groups{}.ack_timeout",
	"experimental.queue_raft.groups{}.bind_addr",
	"experimental.queue_raft.groups{}.data_dir",
	"experimental.queue_raft.groups{}.election_timeout",
	"experimental.queue_raft.groups{}.enabled",
	"experimental.queue_raft.groups{}.heartbeat_timeout",
	"experimental.queue_raft.groups{}.min_in_sync_replicas",
	"experimental.queue_raft.groups{}.peers{}",
	"experimental.queue_raft.groups{}.replication_factor",
	"experimental.queue_raft.groups{}.snapshot_interval",
	"experimental.queue_raft.groups{}.snapshot_threshold",
	"experimental.queue_raft.groups{}.sync_mode",
	"experimental.queue_raft.heartbeat_timeout",
	"experimental.queue_raft.min_in_sync_replicas",
	"experimental.queue_raft.port",
	"experimental.queue_raft.replication_factor",
	"experimental.queue_raft.snapshot_interval",
	"experimental.queue_raft.snapshot_threshold",
	"experimental.queue_raft.sync_mode",
	"experimental.queue_raft.write_policy",
	"health.address",
	"health.enabled",
	"hooks.events{}",
	"hooks.fail_mode",
	"hooks.protocols{}",
	"hooks.timeout",
	"hooks.transport",
	"hooks.url",
	"listeners.amqp091[].address",
	"listeners.amqp091[].auth",
	"listeners.amqp091[].handshake_timeout",
	"listeners.amqp091[].max_connections",
	"listeners.amqp091[].tls.cert_file",
	"listeners.amqp091[].tls.cipher_suites[]",
	"listeners.amqp091[].tls.client_ca_file",
	"listeners.amqp091[].tls.key_file",
	"listeners.amqp091[].tls.min_version",
	"listeners.amqp1[].address",
	"listeners.amqp1[].handshake_timeout",
	"listeners.amqp1[].max_connections",
	"listeners.amqp1[].tls.cert_file",
	"listeners.amqp1[].tls.cipher_suites[]",
	"listeners.amqp1[].tls.client_ca_file",
	"listeners.amqp1[].tls.key_file",
	"listeners.amqp1[].tls.min_version",
	"listeners.mqtt[].address",
	"listeners.mqtt[].allowed_origins[]",
	"listeners.mqtt[].max_connections",
	"listeners.mqtt[].path",
	"listeners.mqtt[].read_timeout",
	"listeners.mqtt[].tls.cert_file",
	"listeners.mqtt[].tls.cipher_suites[]",
	"listeners.mqtt[].tls.client_ca_file",
	"listeners.mqtt[].tls.key_file",
	"listeners.mqtt[].tls.min_version",
	"listeners.mqtt[].transport",
	"listeners.mqtt[].versions[]",
	"listeners.mqtt[].write_timeout",
	"log.format",
	"log.level",
	"queue_manager.auto_commit_interval",
	"queue_manager.capture_drain_timeout",
	"queue_manager.capture_queue_depth",
	"queue_manager.capture_workers",
	"queues[].dlq.enabled",
	"queues[].dlq.topic",
	"queues[].limits.max_depth",
	"queues[].limits.max_message_size",
	"queues[].limits.message_ttl",
	"queues[].name",
	"queues[].primary_group",
	"queues[].replication.ack_timeout",
	"queues[].replication.election_timeout",
	"queues[].replication.enabled",
	"queues[].replication.group",
	"queues[].replication.heartbeat_timeout",
	"queues[].replication.min_in_sync_replicas",
	"queues[].replication.mode",
	"queues[].replication.replication_factor",
	"queues[].replication.snapshot_interval",
	"queues[].replication.snapshot_threshold",
	"queues[].reserved",
	"queues[].retention.max_age",
	"queues[].retention.max_length_bytes",
	"queues[].retention.max_length_messages",
	"queues[].retry.initial_backoff",
	"queues[].retry.max_backoff",
	"queues[].retry.max_retries",
	"queues[].retry.multiplier",
	"queues[].topics[]",
	"queues[].type",
	"ratelimit.connection.burst",
	"ratelimit.connection.cleanup_interval",
	"ratelimit.connection.enabled",
	"ratelimit.connection.rate",
	"ratelimit.enabled",
	"ratelimit.message.burst",
	"ratelimit.message.enabled",
	"ratelimit.message.rate",
	"ratelimit.subscribe.burst",
	"ratelimit.subscribe.enabled",
	"ratelimit.subscribe.rate",
	"session.default_expiry_interval",
	"session.disconnect_on_full",
	"session.inflight_overflow",
	"session.max_inflight_messages",
	"session.max_offline_queue_size",
	"session.max_send_queue_size",
	"session.max_sessions",
	"session.offline_queue_policy",
	"session.pending_queue_size",
	"shutdown_timeout",
	"storage.data_dir",
	"storage.queue_recover_on_startup",
	"storage.badger_sync_writes",
	"storage.type",
	"telemetry.ca_file",
	"telemetry.cert_file",
	"telemetry.enabled",
	"telemetry.endpoint",
	"telemetry.insecure",
	"telemetry.key_file",
	"telemetry.metrics_enabled",
	"telemetry.service_name",
	"telemetry.service_version",
	"telemetry.trace_sample_rate",
	"telemetry.traces_enabled",
	"version",
	"webhook.defaults.circuit_breaker.failure_threshold",
	"webhook.defaults.circuit_breaker.reset_timeout",
	"webhook.defaults.retry.initial_interval",
	"webhook.defaults.retry.max_attempts",
	"webhook.defaults.retry.max_interval",
	"webhook.defaults.retry.multiplier",
	"webhook.defaults.timeout",
	"webhook.drop_policy",
	"webhook.enabled",
	"webhook.endpoints[].events[]",
	"webhook.endpoints[].headers{}",
	"webhook.endpoints[].name",
	"webhook.endpoints[].retry.initial_interval",
	"webhook.endpoints[].retry.max_attempts",
	"webhook.endpoints[].retry.max_interval",
	"webhook.endpoints[].retry.multiplier",
	"webhook.endpoints[].timeout",
	"webhook.endpoints[].topic_filters[]",
	"webhook.endpoints[].type",
	"webhook.endpoints[].url",
	"webhook.include_payload",
	"webhook.queue_size",
	"webhook.shutdown_timeout",
	"webhook.workers",
}

// TestV1SchemaIsFrozen fails whenever the accepted key set drifts from the list
// above. Adding a key is compatible and only needs a new line here; renaming or
// removing one is not, and the diff below is the warning.
func TestV1SchemaIsFrozen(t *testing.T) {
	got := collectSchemaPaths(reflect.TypeFor[document](), "")
	sort.Strings(got)

	want := slices.Clone(v1Schema)
	sort.Strings(want)

	added := difference(got, want)
	removed := difference(want, got)

	for _, path := range added {
		t.Errorf("configuration key %q is not in the frozen v1 schema; add it to v1Schema if the addition is intended", path)
	}
	for _, path := range removed {
		t.Errorf("frozen v1 configuration key %q no longer exists; renaming or removing a key breaks deployed configurations", path)
	}
}

// collectSchemaPaths walks the document types the way the strict decoder does,
// yielding a dotted path per accepted scalar key.
func collectSchemaPaths(typ reflect.Type, prefix string) []string {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.Struct:
		var paths []string
		for i := range typ.NumField() {
			field := typ.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := strings.Split(field.Tag.Get("yaml"), ",")[0]
			if tag == "-" {
				continue
			}
			if tag == "" {
				tag = strings.ToLower(field.Name)
			}
			paths = append(paths, collectSchemaPaths(field.Type, joinSchemaPath(prefix, tag))...)
		}
		return paths
	case reflect.Slice, reflect.Array:
		return collectSchemaPaths(typ.Elem(), prefix+"[]")
	case reflect.Map:
		return collectSchemaPaths(typ.Elem(), prefix+"{}")
	default:
		return []string{prefix}
	}
}

func joinSchemaPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func difference(from, remove []string) []string {
	excluded := make(map[string]struct{}, len(remove))
	for _, value := range remove {
		excluded[value] = struct{}{}
	}
	var result []string
	for _, value := range from {
		if _, skip := excluded[value]; !skip {
			result = append(result, value)
		}
	}
	return result
}

// A runtime-only struct must not carry yaml tags: the tag reads as a
// configuration key that no file can actually set, and the next reader will
// believe it. Everything the document reaches is covered by the frozen schema
// above, so anything listed here is derived rather than configured.
func TestDerivedRuntimeStructsCarryNoYAMLTags(t *testing.T) {
	derived := []any{
		ClusterConfig{}, EtcdConfig{}, TransportConfig{}, RaftConfig{}, StorageConfig{},
	}
	for _, value := range derived {
		typ := reflect.TypeOf(value)
		t.Run(typ.Name(), func(t *testing.T) {
			for i := range typ.NumField() {
				field := typ.Field(i)
				if tag, ok := field.Tag.Lookup("yaml"); ok && tag != "-" {
					t.Errorf("%s.%s carries yaml tag %q but is derived, not decoded", typ.Name(), field.Name, tag)
				}
			}
		})
	}
}

// The pinned schema is only meaningful if it matches what the loader really
// accepts, so spot-check a representative key from each decoding style.
func TestV1SchemaMatchesTheDecoder(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "runtime struct section", body: minimalV1 + "broker:\n  max_qos: 1\n"},
		{name: "document section", body: minimalV1 + "health:\n  enabled: false\n"},
		{name: "operator-keyed map", body: minimalV1 + "hooks:\n  protocols:\n    mqtt: true\n"},
		{name: "sequence element", body: minimalV1 + "queues:\n  - name: q\n    topics: [\"m/#\"]\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := loadTestYAMLError(t, test.body, LoadOptions{}); err != nil {
				t.Fatalf("a key listed in v1Schema was rejected by the loader: %v", err)
			}
		})
	}

	unknown := minimalV1 + "broker:\n  max_qos_typo: 1\n"
	_, err := loadTestYAMLError(t, unknown, LoadOptions{})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("loader accepted a key absent from the schema: %v", err)
	}
}
