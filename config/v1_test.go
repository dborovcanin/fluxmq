// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

const minimalV1 = `version: 1
listeners:
  mqtt:
    - address: "127.0.0.1:1883"
      transport: tcp
      versions: ["3.1.1", "5.0"]
  amqp091: []
  amqp1: []
storage:
  type: memory
`

func TestLoadV1MinimalAndDeterministicDefaults(t *testing.T) {
	first := loadTestYAML(t, minimalV1, LoadOptions{})
	second := loadTestYAML(t, minimalV1, LoadOptions{})
	if !reflect.DeepEqual(first, second) {
		t.Fatal("normalization is not deterministic")
	}
	if first.Version != VersionV1 || first.Development {
		t.Fatalf("unexpected mode: version=%d development=%v", first.Version, first.Development)
	}
	if got := first.Listeners.MQTT[0]; got.ProtocolMode() != ProtocolModeAuto || got.MaxConnections != 10000 || got.ReadTimeout != 60*time.Second {
		t.Fatalf("unexpected normalized listener defaults: %+v", got)
	}
	if first.Cluster.Enabled {
		t.Fatal("omitted cluster section enabled clustering")
	}
	if first.Cluster.Raft.DistributionMode != "forward" {
		t.Fatalf("queue distribution mode = %q, want forward", first.Cluster.Raft.DistributionMode)
	}
}

func TestLoadV1StrictPaths(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "version required", body: strings.Replace(minimalV1, "version: 1\n", "", 1), want: "version is required"},
		{name: "version exact", body: strings.Replace(minimalV1, "version: 1", "version: 2", 1), want: "version must be 1"},
		{name: "old server key", body: minimalV1 + "server:\n  tcp: {}\n", want: "server: unknown field"},
		{name: "unknown nested key", body: strings.Replace(minimalV1, "transport: tcp", "transprot: tcp", 1), want: "listeners.mqtt[0].transprot: unknown field"},
		{name: "duplicate nested key", body: strings.Replace(minimalV1, "      transport: tcp\n", "      transport: tcp\n      transport: websocket\n", 1), want: "listeners.mqtt[0].transport: duplicate field"},
		{name: "misplaced listener key", body: "version: 1\nlisteners:\n  mqtt:\n    address: \":1883\"\n  amqp091: []\n  amqp1: []\n", want: "listeners.mqtt: must be a sequence"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadTestYAMLError(t, test.body, LoadOptions{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadWithOptions() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadV1ListenerPolicies(t *testing.T) {
	body := `version: 1
listeners:
  mqtt:
    - address: ":1883"
      transport: tcp
      versions: ["3.1.1"]
    - address: ":8083"
      transport: websocket
      versions: ["5.0"]
      path: /mqtt
  amqp091:
    - address: ":5682"
      auth: external
  amqp1:
    - address: ":5672"
storage:
  type: memory
`
	cfg := loadTestYAML(t, body, LoadOptions{})
	if len(cfg.Listeners.MQTT) != 2 || cfg.Listeners.MQTT[0].ProtocolMode() != ProtocolModeV3 || cfg.Listeners.MQTT[1].ProtocolMode() != ProtocolModeV5 {
		t.Fatalf("unexpected MQTT listeners: %+v", cfg.Listeners.MQTT)
	}
	if cfg.Listeners.AMQP091[0].Auth != AMQP091AuthExternal || cfg.Listeners.AMQP1[0].HandshakeTimeout != 10*time.Second {
		t.Fatalf("unexpected AMQP listeners: amqp091=%+v amqp1=%+v", cfg.Listeners.AMQP091, cfg.Listeners.AMQP1)
	}
}

// v1 listeners are a list, not a fixed plain/TLS/mTLS matrix, so several
// listeners of the same transport and protocol version must all survive
// normalization and all be validated. A matrix keyed on those attributes
// silently keeps only the last one.
func TestLoadV1KeepsEveryListenerOfTheSameClass(t *testing.T) {
	body := `version: 1
listeners:
  mqtt:
    - address: ":1883"
      transport: tcp
      versions: ["3.1.1"]
    - address: ":1993"
      transport: tcp
      versions: ["3.1.1"]
    - address: ":3883"
      transport: tcp
      versions: ["3.1.1"]
  amqp091:
    - address: ":5682"
      auth: external
    - address: ":5683"
      auth: external
  amqp1: []
storage:
  type: memory
`
	cfg := loadTestYAML(t, body, LoadOptions{})

	addresses := make([]string, 0, len(cfg.Listeners.MQTT))
	for _, listener := range cfg.Listeners.MQTT {
		addresses = append(addresses, listener.Address)
	}
	if !slices.Equal(addresses, []string{":1883", ":1993", ":3883"}) {
		t.Fatalf("MQTT listeners = %v, want all three preserved in order", addresses)
	}
	if len(cfg.Listeners.AMQP091) != 2 {
		t.Fatalf("AMQP 0.9.1 listeners = %d, want 2", len(cfg.Listeners.AMQP091))
	}

	// Every listener is validated, not just the last of its class.
	cfg.Listeners.MQTT[0].MaxConnections = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted a negative limit on the first listener of its class")
	}
}

// An empty admin address disables the API; a blank one is a typo that must not
// silently disable it.
func TestLoadV1RejectsBlankAdminAddress(t *testing.T) {
	body := `version: 1
listeners:
  mqtt:
    - address: ":1883"
      transport: tcp
      versions: ["3.1.1"]
  amqp091: []
  amqp1: []
storage:
  type: memory
admin:
  address: "   "
`
	filename := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filename, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(filename)
	if err == nil || !strings.Contains(err.Error(), "admin.address cannot be blank when set") {
		t.Fatalf("Load() error = %v, want a blank admin.address failure", err)
	}
}

func TestLoadV1RejectsInvalidListenerTLSAndExperimentalGates(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "TLS missing key",
			body: strings.Replace(minimalV1, "      versions: [\"3.1.1\", \"5.0\"]", "      versions: [\"3.1.1\", \"5.0\"]\n      tls:\n        cert_file: server.pem", 1),
			want: "listeners.mqtt[0].tls.key_file is required",
		},
		{
			name: "local AMQP requires mTLS",
			body: strings.Replace(minimalV1, "  amqp091: []", "  amqp091:\n    - address: \":5682\"\n      auth: local", 1),
			want: "auth local requires tls.client_ca_file",
		},
		{
			name: "HTTP listener requires gate",
			body: minimalV1 + "experimental:\n  http:\n    listeners:\n      - address: \":8080\"\n",
			want: "experimental.http.listeners requires experimental.http.enabled: true",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadTestYAMLError(t, test.body, LoadOptions{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadWithOptions() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadV1ClusterDerivationAndNodePrecedence(t *testing.T) {
	t.Setenv("FLUXMQ_NODE_ID", "node2")
	body := minimalV1 + `cluster:
  members:
    node1: node1.internal
    node2: node2.internal
    node3: node3.internal
  ports:
    etcd_peer: 2380
    transport: 7948
  tls:
    ca_file: /run/ca
    cert_file: /run/cert
    key_file: /run/key
`
	body = strings.Replace(body, "storage:\n  type: memory", "storage:\n  type: badger\n  data_dir: /var/lib/fluxmq", 1)
	cfg := loadTestYAML(t, body, LoadOptions{NodeID: testClusterNode1})
	if cfg.Cluster.NodeID != testClusterNode1 {
		t.Fatalf("CLI node ID did not take precedence: %q", cfg.Cluster.NodeID)
	}
	if cfg.Cluster.Etcd.AdvertiseAddr != "node1.internal:2380" || cfg.Cluster.Etcd.ClientAddr != "127.0.0.1:2379" {
		t.Fatalf("unexpected etcd derivation: %+v", cfg.Cluster.Etcd)
	}
	if cfg.Cluster.Transport.Peers["node3"] != "node3.internal:7948" || !strings.Contains(cfg.Cluster.Etcd.InitialCluster, "node2=https://node2.internal:2380") {
		t.Fatalf("unexpected peer derivation: etcd=%q transport=%v", cfg.Cluster.Etcd.InitialCluster, cfg.Cluster.Transport.Peers)
	}
}

func TestLoadV1ClusterSecurityAndIdentityErrors(t *testing.T) {
	base := strings.Replace(minimalV1, "storage:\n  type: memory", "storage:\n  type: badger\n  data_dir: /tmp/fluxmq", 1)
	cluster := `cluster:
  members:
    node1: node1.internal
`
	tests := []struct {
		name string
		body string
		opts LoadOptions
		want string
	}{
		{name: "node ID required", body: base + cluster, want: "cluster node ID is required"},
		{name: "node ID must be member", body: base + cluster + "  allow_insecure: true\n", opts: LoadOptions{NodeID: "node2"}, want: "is not present in cluster.members"},
		{name: "secure by default", body: base + cluster, opts: LoadOptions{NodeID: testClusterNode1}, want: "cluster.tls is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("FLUXMQ_NODE_ID", "")
			_, err := loadTestYAMLError(t, test.body, test.opts)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadWithOptions() error = %v, want %q", err, test.want)
			}
		})
	}

	insecure := loadTestYAML(t, base+cluster+"  allow_insecure: true\n", LoadOptions{NodeID: testClusterNode1})
	if insecure.Cluster.Transport.TLSEnabled || !strings.Contains(insecure.Cluster.Etcd.InitialCluster, "http://") {
		t.Fatalf("explicit insecure cluster was not derived as plaintext: %+v", insecure.Cluster)
	}
}

func FuzzV1ConfigurationDecoder(f *testing.F) {
	f.Add([]byte(minimalV1))
	f.Add([]byte("version: 1\nlisteners: {}\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = loadV1(data, LoadOptions{})
	})
}

func loadTestYAML(t *testing.T, body string, options LoadOptions) *Config {
	t.Helper()
	cfg, err := loadTestYAMLError(t, body, options)
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}
	return cfg
}

func loadTestYAMLError(t *testing.T, body string, options LoadOptions) (*Config, error) {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filename, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return LoadWithOptions(filename, options)
}
