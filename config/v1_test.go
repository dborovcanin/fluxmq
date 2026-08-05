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
		{name: "version exact", body: strings.Replace(minimalV1, "version: 1", "version: 2", 1), want: "unsupported configuration version 2"},
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

// A persistent broker resolves every durable path under storage.data_dir, so
// an unset or relative root would scatter data across the process working
// directory rather than the operator's chosen volume.
func TestLoadV1BadgerRequiresAbsoluteDataDir(t *testing.T) {
	withStorage := func(storage string) string {
		return strings.Replace(minimalV1, "storage:\n  type: memory", storage, 1)
	}
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing",
			body: withStorage("storage:\n  type: badger"),
			want: "storage.data_dir is required when storage.type is badger",
		},
		{
			name: "relative",
			body: withStorage("storage:\n  type: badger\n  data_dir: ./data"),
			want: `storage.data_dir must be an absolute path, got "./data"`,
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

	cfg := loadTestYAML(t, withStorage("storage:\n  type: badger\n  data_dir: /var/lib/fluxmq"), LoadOptions{})
	if cfg.Storage.BadgerDir != "/var/lib/fluxmq/broker" {
		t.Fatalf("BadgerDir = %q, want it derived from data_dir", cfg.Storage.BadgerDir)
	}
}

// The version selects the schema, so an unsupported one must be reported as
// such rather than as whatever unknown keys that version happens to add.
func TestLoadV1ReportsUnsupportedVersionBeforeUnknownFields(t *testing.T) {
	body := "version: 2\nlisteners:\n  mqtt: []\n  quic: []\nstorage:\n  type: memory\n"
	_, err := loadTestYAMLError(t, body, LoadOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsupported configuration version 2") {
		t.Fatalf("LoadWithOptions() error = %v, want an unsupported-version failure", err)
	}
	if strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("a future version's keys were judged against the v1 schema: %v", err)
	}
}

// Reserved queues back protocol-level addressing rather than an operator's own
// workload, so omitting them — or writing an empty list — must not delete them.
func TestLoadV1KeepsReservedQueues(t *testing.T) {
	reserved := ReservedQueues()[0].Name

	tests := []struct {
		name string
		body string
	}{
		{name: "omitted", body: minimalV1},
		{name: "empty list", body: minimalV1 + "queues: []\n"},
		{name: "other queues only", body: minimalV1 + "queues:\n  - name: telemetry\n    topics: [\"m/#\"]\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := loadTestYAML(t, test.body, LoadOptions{})
			index := slices.IndexFunc(cfg.Queues, func(q QueueConfig) bool { return q.Name == reserved })
			if index < 0 {
				t.Fatalf("reserved queue %q was dropped: %+v", reserved, cfg.Queues)
			}
			if !cfg.Queues[index].Reserved {
				t.Fatalf("queue %q lost its reserved flag", reserved)
			}
		})
	}

	t.Run("retuned by the operator", func(t *testing.T) {
		body := minimalV1 + "queues:\n  - name: " + reserved + "\n    topics: [\"$queue/#\"]\n    limits:\n      max_depth: 5\n"
		cfg := loadTestYAML(t, body, LoadOptions{})
		index := slices.IndexFunc(cfg.Queues, func(q QueueConfig) bool { return q.Name == reserved })
		if index < 0 {
			t.Fatalf("reserved queue %q was dropped", reserved)
		}
		if !cfg.Queues[index].Reserved {
			t.Fatal("an operator-declared reserved queue must stay reserved")
		}
		if cfg.Queues[index].Limits.MaxDepth != 5 {
			t.Fatalf("operator tuning was ignored: %+v", cfg.Queues[index].Limits)
		}
	})
}

// etcd persists the peer URLs it formed with, so the manifest fingerprint has
// to cover everything those URLs are built from. A fingerprint over the member
// map alone would let a changed port or a flip to plaintext pass the check and
// then disagree with etcd's own recorded membership.
func TestLoadV1ClusterFingerprintCoversDerivedPeerURLs(t *testing.T) {
	fingerprint := func(t *testing.T, ports, extra string) string {
		t.Helper()
		body := strings.Replace(minimalV1, "storage:\n  type: memory", "storage:\n  type: badger\n  data_dir: /var/lib/fluxmq", 1)
		body += "cluster:\n  members:\n    node1: node1.internal\n    node2: node2.internal\n" + ports + extra
		return loadTestYAML(t, body, LoadOptions{NodeID: testClusterNode1}).Cluster.ManifestFingerprint
	}

	tls := "  tls:\n    ca_file: /run/ca\n    cert_file: /run/cert\n    key_file: /run/key\n"
	base := fingerprint(t, "  ports:\n    etcd_peer: 2380\n", tls)

	if same := fingerprint(t, "  ports:\n    etcd_peer: 2380\n", tls); same != base {
		t.Fatal("fingerprint is not stable for an unchanged manifest")
	}
	if changed := fingerprint(t, "  ports:\n    etcd_peer: 12380\n", tls); changed == base {
		t.Fatal("changing cluster.ports.etcd_peer left the fingerprint unchanged")
	}
	if insecure := fingerprint(t, "  ports:\n    etcd_peer: 2380\n", "  allow_insecure: true\n"); insecure == base {
		t.Fatal("dropping cluster TLS left the fingerprint unchanged, but the peer URL scheme changed")
	}
}

// The admin API is unauthenticated, so binding it beyond loopback is stated
// plainly rather than left for an operator to notice.
func TestSecurityWarningsCoverAdminExposure(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		{name: "all interfaces", address: ":8082", want: true},
		{name: "explicit external", address: "10.0.0.4:8082", want: true},
		{name: "loopback IPv4", address: defaultAdminAddress},
		{name: "loopback IPv6", address: "[::1]:8082"},
		{name: "localhost", address: "localhost:8082"},
		{name: "disabled", address: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			cfg.Admin.Address = test.address
			warnings := cfg.SecurityWarnings()
			if got := len(warnings) > 0; got != test.want {
				t.Fatalf("SecurityWarnings() = %v, want warning=%v", warnings, test.want)
			}
		})
	}
}

// An omitted limit or timeout takes the default; an explicit 0 means unlimited
// connections or no deadline. Collapsing the two would make "no cap"
// unexpressible and would silently change meaning if the default ever moved.
func TestLoadV1ZeroMeansUnlimitedAndOmittedMeansDefault(t *testing.T) {
	listener := func(fields string) string {
		return strings.Replace(minimalV1, "      versions: [\"3.1.1\", \"5.0\"]", "      versions: [\"3.1.1\", \"5.0\"]\n"+fields, 1)
	}

	t.Run("omitted takes the default", func(t *testing.T) {
		got := loadTestYAML(t, minimalV1, LoadOptions{}).Listeners.MQTT[0]
		if got.MaxConnections != defaultMaxConnections {
			t.Fatalf("max_connections = %d, want the default %d", got.MaxConnections, defaultMaxConnections)
		}
		if got.ReadTimeout != defaultListenerTimeout || got.WriteTimeout != defaultListenerTimeout {
			t.Fatalf("timeouts = %v/%v, want the default %v", got.ReadTimeout, got.WriteTimeout, defaultListenerTimeout)
		}
	})

	t.Run("explicit zero means unlimited", func(t *testing.T) {
		body := listener("      max_connections: 0\n      read_timeout: 0s\n      write_timeout: 0s")
		got := loadTestYAML(t, body, LoadOptions{}).Listeners.MQTT[0]
		if got.MaxConnections != 0 || got.ReadTimeout != 0 || got.WriteTimeout != 0 {
			t.Fatalf("an explicit 0 was replaced by a default: %+v", got)
		}
	})

	t.Run("explicit value is kept", func(t *testing.T) {
		got := loadTestYAML(t, listener("      max_connections: 42\n      read_timeout: 5s"), LoadOptions{}).Listeners.MQTT[0]
		if got.MaxConnections != 42 || got.ReadTimeout != 5*time.Second {
			t.Fatalf("explicit values were not preserved: %+v", got)
		}
		if got.WriteTimeout != defaultListenerTimeout {
			t.Fatalf("an omitted sibling lost its default: %v", got.WriteTimeout)
		}
	})

	t.Run("negative is still rejected", func(t *testing.T) {
		_, err := loadTestYAMLError(t, listener("      max_connections: -1"), LoadOptions{})
		if err == nil || !strings.Contains(err.Error(), "max_connections cannot be negative") {
			t.Fatalf("LoadWithOptions() error = %v, want a negative-limit failure", err)
		}
	})

	t.Run("AMQP handshake timeout", func(t *testing.T) {
		withAMQP := func(fields string) string {
			return strings.Replace(minimalV1, "  amqp1: []", "  amqp1:\n    - address: \":5672\"\n"+fields, 1)
		}
		if got := loadTestYAML(t, withAMQP(""), LoadOptions{}).Listeners.AMQP1[0]; got.HandshakeTimeout != defaultHandshakeTimeout {
			t.Fatalf("handshake_timeout = %v, want the default %v", got.HandshakeTimeout, defaultHandshakeTimeout)
		}
		cfg := loadTestYAML(t, withAMQP("      handshake_timeout: 0s"), LoadOptions{})
		if cfg.Listeners.AMQP1[0].HandshakeTimeout != 0 {
			t.Fatal("an explicit 0 handshake_timeout was replaced by a default")
		}
		// Disabling the only bound on a pre-auth connection is legal but loud.
		warnings := cfg.SecurityWarnings()
		if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, " "), "handshake_timeout is 0") {
			t.Fatalf("SecurityWarnings() = %v, want a disabled-handshake warning", warnings)
		}
	})
}

// Save writes resolved values, so a saved file reloads to exactly the same
// configuration rather than re-defaulting anything that was explicitly zero.
func TestSaveRoundTripsExplicitZero(t *testing.T) {
	body := strings.Replace(minimalV1, "      versions: [\"3.1.1\", \"5.0\"]",
		"      versions: [\"3.1.1\", \"5.0\"]\n      max_connections: 0\n      read_timeout: 0s", 1)
	cfg := loadTestYAML(t, body, LoadOptions{})

	saved := filepath.Join(t.TempDir(), "saved.yaml")
	if err := cfg.Save(saved); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	reloaded, err := Load(saved)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := reloaded.Listeners.MQTT[0]
	if got.MaxConnections != 0 || got.ReadTimeout != 0 {
		t.Fatalf("an explicit 0 did not survive a save/load round trip: %+v", got)
	}
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
