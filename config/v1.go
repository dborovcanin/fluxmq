// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	mqtttls "github.com/absmach/fluxmq/pkg/tls"
	"gopkg.in/yaml.v3"
)

const (
	VersionV1 = 1

	MQTTTransportTCP       = "tcp"
	MQTTTransportWebSocket = "websocket"
	MQTTVersion311         = "3.1.1"
	MQTTVersion50          = "5.0"
	AMQP091AuthExternal    = "external"
	AMQP091AuthLocal       = "local"
)

// LoadOptions contains the only supported process-local configuration
// override. NodeID selects this process from a shared static cluster manifest.
type LoadOptions struct {
	NodeID string
}

// ListenersConfig is the normalized listener set consumed by startup.
type ListenersConfig struct {
	MQTT    []MQTTListenerConfig
	AMQP091 []AMQP091ListenerConfig
	AMQP1   []AMQP1ListenerConfig
}

// MQTTListenerConfig is a normalized MQTT TCP or WebSocket listener.
type MQTTListenerConfig struct {
	Address        string
	Transport      string
	Versions       []string
	Path           string
	AllowedOrigins []string
	MaxConnections int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	TLS            *mqtttls.Config
}

// ProtocolMode returns the runtime MQTT protocol-selection mode.
func (c MQTTListenerConfig) ProtocolMode() string {
	if len(c.Versions) == 1 {
		if c.Versions[0] == MQTTVersion311 {
			return ProtocolModeV3
		}
		return ProtocolModeV5
	}
	return ProtocolModeAuto
}

// AMQP091ListenerConfig is a normalized AMQP 0.9.1 listener.
type AMQP091ListenerConfig struct {
	Address          string
	Auth             string
	MaxConnections   int
	HandshakeTimeout time.Duration
	TLS              *mqtttls.Config
}

// AMQP1ListenerConfig is a normalized AMQP 1.0 listener.
type AMQP1ListenerConfig struct {
	Address          string
	MaxConnections   int
	HandshakeTimeout time.Duration
	TLS              *mqtttls.Config
}

// AdminConfig configures the administrative API independently of listeners.
type AdminConfig struct {
	Address string
}

// HealthConfig configures the liveness/readiness endpoint.
type HealthConfig struct {
	Enabled bool
	Address string
}

// TelemetryConfig configures OpenTelemetry export.
type TelemetryConfig struct {
	Enabled         bool
	Endpoint        string
	ServiceName     string
	ServiceVersion  string
	TracesEnabled   bool
	MetricsEnabled  bool
	TraceSampleRate float64
	Insecure        bool
	CAFile          string
	CertFile        string
	KeyFile         string
}

// ClusterPortsConfig contains the ports shared by every cluster member.
type ClusterPortsConfig struct {
	EtcdPeer  int
	Transport int
}

// ClusterTLSConfig is the single mTLS identity used by the embedded-etcd peer
// and broker transport planes.
type ClusterTLSConfig struct {
	CAFile   string
	CertFile string
	KeyFile  string
}

// ExperimentalConfig contains features outside the v1 compatibility contract.
type ExperimentalConfig struct {
	HTTP      ExperimentalHTTPConfig
	CoAP      ExperimentalCoAPConfig
	QueueRaft ExperimentalQueueRaftConfig
}

type ExperimentalHTTPConfig struct {
	Enabled   bool
	Listeners []ExperimentalHTTPListenerConfig
}

type ExperimentalHTTPListenerConfig struct {
	Address string
	TLS     *mqtttls.Config
}

type ExperimentalCoAPConfig struct {
	Enabled   bool
	Listeners []ExperimentalCoAPListenerConfig
}

type ExperimentalCoAPListenerConfig struct {
	Address string
	TLS     *mqtttls.Config
}

type ExperimentalQueueRaftConfig struct {
	Enabled             bool
	Port                int
	AutoProvisionGroups bool
	ReplicationFactor   int
	SyncMode            bool
	MinInSyncReplicas   int
	AckTimeout          time.Duration
	WritePolicy         string
	DistributionMode    string
	HeartbeatTimeout    time.Duration
	ElectionTimeout     time.Duration
	SnapshotInterval    time.Duration
	SnapshotThreshold   uint64
	Groups              map[string]RaftGroupConfig
}

type document struct {
	Version         int                   `yaml:"version"`
	Listeners       *listenersDocument    `yaml:"listeners"`
	Admin           *adminDocument        `yaml:"admin"`
	Health          *healthDocument       `yaml:"health"`
	Telemetry       *telemetryDocument    `yaml:"telemetry"`
	ShutdownTimeout time.Duration         `yaml:"shutdown_timeout"`
	Broker          BrokerConfig          `yaml:"broker"`
	Session         SessionConfig         `yaml:"session"`
	Log             LogConfig             `yaml:"log"`
	Storage         storageDocument       `yaml:"storage"`
	Cluster         *clusterDocument      `yaml:"cluster,omitempty"`
	Experimental    *experimentalDocument `yaml:"experimental"`
	Webhook         WebhookConfig         `yaml:"webhook"`
	RateLimit       RateLimitConfig       `yaml:"ratelimit"`
	QueueManager    QueueManagerConfig    `yaml:"queue_manager"`
	Queues          []QueueConfig         `yaml:"queues"`
	Auth            AuthConfig            `yaml:"auth"`
	Hooks           HooksConfig           `yaml:"hooks"`
}

type listenersDocument struct {
	MQTT    []mqttListenerDocument    `yaml:"mqtt"`
	AMQP091 []amqp091ListenerDocument `yaml:"amqp091"`
	AMQP1   []amqp1ListenerDocument   `yaml:"amqp1"`
}

// Limits and timeouts are pointers so that an omitted key takes the default
// while an explicit 0 means unlimited. Collapsing the two would make "no cap"
// unexpressible and hide which of the two an operator asked for.
type mqttListenerDocument struct {
	Address        string               `yaml:"address"`
	Transport      string               `yaml:"transport"`
	Versions       []string             `yaml:"versions"`
	Path           string               `yaml:"path"`
	AllowedOrigins []string             `yaml:"allowed_origins"`
	MaxConnections *int                 `yaml:"max_connections"`
	ReadTimeout    *time.Duration       `yaml:"read_timeout"`
	WriteTimeout   *time.Duration       `yaml:"write_timeout"`
	TLS            *listenerTLSDocument `yaml:"tls,omitempty"`
}

type amqp091ListenerDocument struct {
	Address          string               `yaml:"address"`
	Auth             string               `yaml:"auth"`
	MaxConnections   *int                 `yaml:"max_connections"`
	HandshakeTimeout *time.Duration       `yaml:"handshake_timeout"`
	TLS              *listenerTLSDocument `yaml:"tls,omitempty"`
}

type amqp1ListenerDocument struct {
	Address          string               `yaml:"address"`
	MaxConnections   *int                 `yaml:"max_connections"`
	HandshakeTimeout *time.Duration       `yaml:"handshake_timeout"`
	TLS              *listenerTLSDocument `yaml:"tls,omitempty"`
}

type listenerTLSDocument struct {
	CertFile     string   `yaml:"cert_file"`
	KeyFile      string   `yaml:"key_file"`
	ClientCAFile string   `yaml:"client_ca_file"`
	MinVersion   string   `yaml:"min_version"`
	CipherSuites []string `yaml:"cipher_suites"`
}

type adminDocument struct {
	Address string `yaml:"address"`
}

type healthDocument struct {
	Enabled bool   `yaml:"enabled"`
	Address string `yaml:"address"`
}

type telemetryDocument struct {
	Enabled         bool    `yaml:"enabled"`
	Endpoint        string  `yaml:"endpoint"`
	ServiceName     string  `yaml:"service_name"`
	ServiceVersion  string  `yaml:"service_version"`
	TracesEnabled   bool    `yaml:"traces_enabled"`
	MetricsEnabled  bool    `yaml:"metrics_enabled"`
	TraceSampleRate float64 `yaml:"trace_sample_rate"`
	Insecure        bool    `yaml:"insecure"`
	CAFile          string  `yaml:"ca_file"`
	CertFile        string  `yaml:"cert_file"`
	KeyFile         string  `yaml:"key_file"`
}

type storageDocument struct {
	Type             string `yaml:"type"`
	DataDir          string `yaml:"data_dir"`
	SyncWrites       bool   `yaml:"sync_writes"`
	RecoverOnStartup bool   `yaml:"recover_on_startup"`
}

type clusterDocument struct {
	Members       map[string]string    `yaml:"members"`
	Ports         clusterPortsDocument `yaml:"ports"`
	TLS           *clusterTLSDocument  `yaml:"tls,omitempty"`
	AllowInsecure bool                 `yaml:"allow_insecure"`
}

type clusterPortsDocument struct {
	EtcdPeer  int `yaml:"etcd_peer"`
	Transport int `yaml:"transport"`
}

type clusterTLSDocument struct {
	CAFile   string `yaml:"ca_file"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type experimentalDocument struct {
	HTTP      experimentalHTTPDocument      `yaml:"http"`
	CoAP      experimentalCoAPDocument      `yaml:"coap"`
	QueueRaft experimentalQueueRaftDocument `yaml:"queue_raft"`
}

type experimentalHTTPDocument struct {
	Enabled   bool                               `yaml:"enabled"`
	Listeners []experimentalHTTPListenerDocument `yaml:"listeners"`
}

type experimentalHTTPListenerDocument struct {
	Address string               `yaml:"address"`
	TLS     *listenerTLSDocument `yaml:"tls,omitempty"`
}

type experimentalCoAPDocument struct {
	Enabled   bool                               `yaml:"enabled"`
	Listeners []experimentalCoAPListenerDocument `yaml:"listeners"`
}

type experimentalCoAPListenerDocument struct {
	Address string               `yaml:"address"`
	TLS     *listenerTLSDocument `yaml:"tls,omitempty"`
}

type experimentalQueueRaftDocument struct {
	Enabled             bool                       `yaml:"enabled"`
	Port                int                        `yaml:"port"`
	AutoProvisionGroups bool                       `yaml:"auto_provision_groups"`
	ReplicationFactor   int                        `yaml:"replication_factor"`
	SyncMode            bool                       `yaml:"sync_mode"`
	MinInSyncReplicas   int                        `yaml:"min_in_sync_replicas"`
	AckTimeout          time.Duration              `yaml:"ack_timeout"`
	WritePolicy         string                     `yaml:"write_policy"`
	DistributionMode    string                     `yaml:"distribution_mode"`
	HeartbeatTimeout    time.Duration              `yaml:"heartbeat_timeout"`
	ElectionTimeout     time.Duration              `yaml:"election_timeout"`
	SnapshotInterval    time.Duration              `yaml:"snapshot_interval"`
	SnapshotThreshold   uint64                     `yaml:"snapshot_threshold"`
	Groups              map[string]RaftGroupConfig `yaml:"groups"`
}

func defaultDocument() document {
	runtime := Default()
	return document{
		Version:         VersionV1,
		Admin:           &adminDocument{Address: runtime.Admin.Address},
		Health:          &healthDocument{Enabled: runtime.Health.Enabled, Address: runtime.Health.Address},
		Telemetry:       &telemetryDocument{Enabled: runtime.Telemetry.Enabled, Endpoint: runtime.Telemetry.Endpoint, ServiceName: runtime.Telemetry.ServiceName, ServiceVersion: runtime.Telemetry.ServiceVersion, TracesEnabled: runtime.Telemetry.TracesEnabled, MetricsEnabled: runtime.Telemetry.MetricsEnabled, TraceSampleRate: runtime.Telemetry.TraceSampleRate},
		ShutdownTimeout: runtime.ShutdownTimeout,
		Broker:          runtime.Broker,
		Session:         runtime.Session,
		Log:             runtime.Log,
		Storage:         storageDocument{Type: runtime.Storage.Type, DataDir: runtime.Storage.DataDir, SyncWrites: runtime.Storage.SyncWrites, RecoverOnStartup: runtime.Storage.RecoverOnStartup},
		Webhook:         runtime.Webhook,
		RateLimit:       runtime.RateLimit,
		QueueManager:    runtime.QueueManager,
		Queues:          runtime.Queues,
		Auth:            runtime.Auth,
		Hooks:           runtime.Hooks,
		Experimental: &experimentalDocument{QueueRaft: experimentalQueueRaftDocument{
			Port: 7100, AutoProvisionGroups: true, ReplicationFactor: 3, SyncMode: true,
			MinInSyncReplicas: 2, AckTimeout: 5 * time.Second, WritePolicy: writePolicyForward,
			DistributionMode: distributionModeReplicate, HeartbeatTimeout: time.Second, ElectionTimeout: 3 * time.Second,
			SnapshotInterval: 5 * time.Minute, SnapshotThreshold: 8192,
		}},
	}
}

// LoadWithOptions loads, strictly validates, and normalizes a v1 YAML file.
func LoadWithOptions(filename string, options LoadOptions) (*Config, error) {
	if filename == "" {
		cfg := Default()
		cfg.Development = true
		return cfg, nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", filename, err)
	}
	return loadV1(data, options)
}

// Default returns the safe no-file development configuration: one loopback
// MQTT listener supporting both stable protocol versions and memory storage.
func Default() *Config {
	cfg := defaultRuntime()
	cfg.Version = VersionV1
	cfg.Development = true
	cfg.Listeners = ListenersConfig{
		MQTT: []MQTTListenerConfig{{
			Address: "127.0.0.1:1883", Transport: MQTTTransportTCP,
			Versions: []string{MQTTVersion311, MQTTVersion50}, MaxConnections: 10000,
			ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second,
		}},
	}
	cfg.Admin = AdminConfig{Address: defaultAdminAddress}
	cfg.Health = HealthConfig{Enabled: true, Address: defaultHealthAddress}
	cfg.Telemetry = TelemetryConfig{
		Endpoint: "127.0.0.1:4317", ServiceName: "fluxmq", ServiceVersion: "1.0.0",
		MetricsEnabled: true, TraceSampleRate: 0.1,
	}
	cfg.Experimental.QueueRaft = ExperimentalQueueRaftConfig{
		Port: 7100, AutoProvisionGroups: true, ReplicationFactor: 3, SyncMode: true,
		MinInSyncReplicas: 2, AckTimeout: 5 * time.Second, WritePolicy: writePolicyForward,
		DistributionMode: distributionModeReplicate, HeartbeatTimeout: time.Second, ElectionTimeout: 3 * time.Second,
		SnapshotInterval: 5 * time.Minute, SnapshotThreshold: 8192,
	}
	cfg.Storage = StorageConfig{Type: storageTypeMemory}
	cfg.Cluster.Enabled = false
	cfg.Cluster.NodeID = "single-node"
	cfg.Cluster.Raft.Enabled = false
	cfg.Cluster.Raft.WritePolicy = writePolicyForward
	cfg.Cluster.Raft.DistributionMode = "forward"
	return cfg
}

func loadV1(data []byte, options LoadOptions) (*Config, error) {
	node, err := decodeYAMLDocument(data)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// The version decides which schema the rest of the file is read against, so
	// it is settled before any field is judged. Checking it later would report a
	// future version's new keys as unknown fields and never mention the version.
	version, err := documentVersion(node)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	if version != VersionV1 {
		return nil, fmt.Errorf("invalid configuration: unsupported configuration version %d; this build supports version %d", version, VersionV1)
	}
	if err := validateStrictNode(node, reflect.TypeFor[document](), ""); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	doc := defaultDocument()
	doc.Listeners = nil
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if doc.Listeners == nil {
		return nil, fmt.Errorf("invalid configuration: listeners is required")
	}

	cfg, err := normalizeDocument(doc, options)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

func marshalV1(cfg *Config) ([]byte, error) {
	if cfg == nil {
		return nil, errors.New("configuration is nil")
	}
	doc := document{
		Version:   VersionV1,
		Listeners: &listenersDocument{},
		Admin:     &adminDocument{Address: cfg.Admin.Address},
		Health:    &healthDocument{Enabled: cfg.Health.Enabled, Address: cfg.Health.Address},
		Telemetry: &telemetryDocument{
			Enabled: cfg.Telemetry.Enabled, Endpoint: cfg.Telemetry.Endpoint,
			ServiceName: cfg.Telemetry.ServiceName, ServiceVersion: cfg.Telemetry.ServiceVersion,
			TracesEnabled: cfg.Telemetry.TracesEnabled, MetricsEnabled: cfg.Telemetry.MetricsEnabled,
			TraceSampleRate: cfg.Telemetry.TraceSampleRate, Insecure: cfg.Telemetry.Insecure,
			CAFile: cfg.Telemetry.CAFile, CertFile: cfg.Telemetry.CertFile, KeyFile: cfg.Telemetry.KeyFile,
		},
		ShutdownTimeout: cfg.ShutdownTimeout,
		Broker:          cfg.Broker, Session: cfg.Session, Log: cfg.Log,
		Storage: storageDocument{Type: cfg.Storage.Type, DataDir: cfg.Storage.DataDir, SyncWrites: cfg.Storage.SyncWrites, RecoverOnStartup: cfg.Storage.RecoverOnStartup},
		Webhook: cfg.Webhook, RateLimit: cfg.RateLimit, QueueManager: cfg.QueueManager,
		Queues: cfg.Queues, Auth: cfg.Auth, Hooks: cfg.Hooks,
		Experimental: &experimentalDocument{
			HTTP: experimentalHTTPDocument{Enabled: cfg.Experimental.HTTP.Enabled},
			CoAP: experimentalCoAPDocument{Enabled: cfg.Experimental.CoAP.Enabled},
			QueueRaft: experimentalQueueRaftDocument{
				Enabled: cfg.Experimental.QueueRaft.Enabled, Port: cfg.Experimental.QueueRaft.Port,
				AutoProvisionGroups: cfg.Experimental.QueueRaft.AutoProvisionGroups,
				ReplicationFactor:   cfg.Experimental.QueueRaft.ReplicationFactor,
				SyncMode:            cfg.Experimental.QueueRaft.SyncMode, MinInSyncReplicas: cfg.Experimental.QueueRaft.MinInSyncReplicas,
				AckTimeout: cfg.Experimental.QueueRaft.AckTimeout, WritePolicy: cfg.Experimental.QueueRaft.WritePolicy,
				DistributionMode: cfg.Experimental.QueueRaft.DistributionMode,
				HeartbeatTimeout: cfg.Experimental.QueueRaft.HeartbeatTimeout, ElectionTimeout: cfg.Experimental.QueueRaft.ElectionTimeout,
				SnapshotInterval: cfg.Experimental.QueueRaft.SnapshotInterval, SnapshotThreshold: cfg.Experimental.QueueRaft.SnapshotThreshold,
				Groups: cfg.Experimental.QueueRaft.Groups,
			},
		},
	}
	for _, listener := range cfg.Listeners.MQTT {
		doc.Listeners.MQTT = append(doc.Listeners.MQTT, mqttListenerDocument{
			Address: listener.Address, Transport: listener.Transport, Versions: listener.Versions,
			Path: listener.Path, AllowedOrigins: listener.AllowedOrigins,
			MaxConnections: &listener.MaxConnections, ReadTimeout: &listener.ReadTimeout,
			WriteTimeout: &listener.WriteTimeout, TLS: listenerTLSDocumentFromRuntime(listener.TLS),
		})
	}
	for _, listener := range cfg.Listeners.AMQP091 {
		doc.Listeners.AMQP091 = append(doc.Listeners.AMQP091, amqp091ListenerDocument{
			Address: listener.Address, Auth: listener.Auth, MaxConnections: &listener.MaxConnections,
			HandshakeTimeout: &listener.HandshakeTimeout, TLS: listenerTLSDocumentFromRuntime(listener.TLS),
		})
	}
	for _, listener := range cfg.Listeners.AMQP1 {
		doc.Listeners.AMQP1 = append(doc.Listeners.AMQP1, amqp1ListenerDocument{
			Address: listener.Address, MaxConnections: &listener.MaxConnections,
			HandshakeTimeout: &listener.HandshakeTimeout, TLS: listenerTLSDocumentFromRuntime(listener.TLS),
		})
	}
	for _, listener := range cfg.Experimental.HTTP.Listeners {
		doc.Experimental.HTTP.Listeners = append(doc.Experimental.HTTP.Listeners, experimentalHTTPListenerDocument{Address: listener.Address, TLS: listenerTLSDocumentFromRuntime(listener.TLS)})
	}
	for _, listener := range cfg.Experimental.CoAP.Listeners {
		doc.Experimental.CoAP.Listeners = append(doc.Experimental.CoAP.Listeners, experimentalCoAPListenerDocument{Address: listener.Address, TLS: listenerTLSDocumentFromRuntime(listener.TLS)})
	}
	if cfg.Cluster.Enabled {
		doc.Cluster = &clusterDocument{
			Members:       cloneStringMap(cfg.Cluster.Members),
			Ports:         clusterPortsDocument{EtcdPeer: cfg.Cluster.Ports.EtcdPeer, Transport: cfg.Cluster.Ports.Transport},
			AllowInsecure: cfg.Cluster.AllowInsecure,
		}
		if cfg.Cluster.TLS != (ClusterTLSConfig{}) {
			doc.Cluster.TLS = &clusterTLSDocument{CAFile: cfg.Cluster.TLS.CAFile, CertFile: cfg.Cluster.TLS.CertFile, KeyFile: cfg.Cluster.TLS.KeyFile}
		}
	}
	return yaml.Marshal(doc)
}

func listenerTLSDocumentFromRuntime(cfg *mqtttls.Config) *listenerTLSDocument {
	if cfg == nil {
		return nil
	}
	return &listenerTLSDocument{
		CertFile: cfg.CertFile, KeyFile: cfg.KeyFile, ClientCAFile: cfg.ClientCAFile,
		MinVersion: cfg.MinVersion, CipherSuites: cfg.CipherSuites,
	}
}

// SecurityWarnings reports configured exposures that are legal but that an
// operator should see stated plainly at startup and from `config validate`.
// They are warnings rather than errors because refusing them outright would
// break deployments that reach the API over a trusted network.
func (c *Config) SecurityWarnings() []string {
	var warnings []string
	// The admin API can reload configuration, delete queues, purge and truncate
	// them, and disconnect sessions, and it has no authentication yet. Binding
	// it beyond loopback publishes all of that to whoever can route to the host.
	if c.Admin.Address != "" && !isLoopbackAddress(c.Admin.Address) {
		warnings = append(warnings, fmt.Sprintf(
			"admin.address %q is not loopback and the admin API is unauthenticated: any client that can reach it may reload configuration, delete or purge queues, and disconnect sessions; bind it to 127.0.0.1 and reach it through a trusted proxy",
			c.Admin.Address))
	}
	// A handshake deadline is the only bound on an unauthenticated connection
	// before it identifies itself, so disabling it lets idle peers hold listener
	// slots indefinitely. It is the operator's call, but not a quiet one.
	for i, listener := range c.Listeners.AMQP091 {
		if listener.HandshakeTimeout == 0 {
			warnings = append(warnings, fmt.Sprintf(
				"listeners.amqp091[%d].handshake_timeout is 0, so an unauthenticated peer may hold a connection slot indefinitely", i))
		}
	}
	for i, listener := range c.Listeners.AMQP1 {
		if listener.HandshakeTimeout == 0 {
			warnings = append(warnings, fmt.Sprintf(
				"listeners.amqp1[%d].handshake_timeout is 0, so an unauthenticated peer may hold a connection slot indefinitely", i))
		}
	}
	return warnings
}

// isLoopbackAddress reports whether a host:port listen address accepts
// connections only from the local host. A missing host means every interface.
func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// documentVersion reads the schema version out of the raw document, ahead of
// any schema-dependent decoding.
func documentVersion(node *yaml.Node) (int, error) {
	if node.Kind != yaml.MappingNode {
		return 0, errors.New("configuration must be a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != "version" {
			continue
		}
		value := node.Content[i+1]
		if value.Kind != yaml.ScalarNode {
			return 0, errors.New("version must be an integer")
		}
		version, err := strconv.Atoi(value.Value)
		if err != nil {
			return 0, fmt.Errorf("version must be an integer, got %q", value.Value)
		}
		return version, nil
	}
	return 0, fmt.Errorf("version is required and must be %d", VersionV1)
}

func decodeYAMLDocument(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		return nil, err
	}
	if len(node.Content) == 0 {
		return nil, errors.New("configuration is empty")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple YAML documents are not supported")
		}
		return nil, err
	}
	return node.Content[0], nil
}

func validateStrictNode(node *yaml.Node, typ reflect.Type, path string) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if node.Kind == yaml.AliasNode {
		return pathError(path, "YAML aliases are not supported")
	}

	switch typ.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return pathError(path, "must be a mapping")
		}
		fields := yamlFields(typ)
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			fieldPath := joinYAMLPath(path, key)
			if _, exists := seen[key]; exists {
				return pathError(fieldPath, "duplicate field")
			}
			seen[key] = struct{}{}
			fieldType, exists := fields[key]
			if !exists {
				return pathError(fieldPath, "unknown field")
			}
			if err := validateStrictNode(node.Content[i+1], fieldType, fieldPath); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if node.Kind != yaml.SequenceNode {
			return pathError(path, "must be a sequence")
		}
		for i, child := range node.Content {
			if err := validateStrictNode(child, typ.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		if node.Kind != yaml.MappingNode {
			return pathError(path, "must be a mapping")
		}
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			fieldPath := joinYAMLPath(path, key)
			if _, exists := seen[key]; exists {
				return pathError(fieldPath, "duplicate field")
			}
			seen[key] = struct{}{}
			if err := validateStrictNode(node.Content[i+1], typ.Elem(), fieldPath); err != nil {
				return err
			}
		}
	default:
		if node.Kind != yaml.ScalarNode || node.Tag == yamlNullTag {
			return pathError(path, "must be a scalar")
		}
	}
	return nil
}

func yamlFields(typ reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
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
		fields[tag] = field.Type
	}
	return fields
}

func joinYAMLPath(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}

func pathError(path, message string) error {
	if path == "" {
		return errors.New(message)
	}
	return fmt.Errorf("%s: %s", path, message)
}

func normalizeDocument(doc document, options LoadOptions) (*Config, error) {
	cfg := Default()
	cfg.Version = VersionV1
	cfg.Development = false
	cfg.Listeners = ListenersConfig{}
	cfg.Broker = doc.Broker
	cfg.Session = doc.Session
	cfg.Log = doc.Log
	cfg.Webhook = doc.Webhook
	cfg.RateLimit = doc.RateLimit
	cfg.QueueManager = doc.QueueManager
	cfg.Queues = applyReservedQueues(doc.Queues)
	cfg.Auth = doc.Auth
	cfg.Hooks = doc.Hooks
	cfg.ShutdownTimeout = doc.ShutdownTimeout
	cfg.Storage = StorageConfig{
		Type: doc.Storage.Type, DataDir: filepath.Clean(doc.Storage.DataDir),
		SyncWrites: doc.Storage.SyncWrites, RecoverOnStartup: doc.Storage.RecoverOnStartup,
	}
	if cfg.Storage.DataDir == "." && strings.TrimSpace(doc.Storage.DataDir) == "" {
		cfg.Storage.DataDir = ""
	}
	if cfg.Storage.Type == storageTypeBadger {
		// A persistent broker resolves every durable path under this root, so
		// leaving it unset or relative would scatter data across whatever
		// directory the process happened to start in.
		if cfg.Storage.DataDir == "" {
			return nil, errors.New("storage.data_dir is required when storage.type is badger")
		}
		if !filepath.IsAbs(cfg.Storage.DataDir) {
			return nil, fmt.Errorf("storage.data_dir must be an absolute path, got %q", doc.Storage.DataDir)
		}
		cfg.Storage.BadgerDir = filepath.Join(cfg.Storage.DataDir, "broker")
	}

	if doc.Admin != nil {
		// An empty address disables the admin API, but a blank one is a typo:
		// trimming it silently would disable the API the operator asked for.
		if doc.Admin.Address != "" && strings.TrimSpace(doc.Admin.Address) == "" {
			return nil, errors.New("admin.address cannot be blank when set")
		}
		cfg.Admin.Address = strings.TrimSpace(doc.Admin.Address)
	}
	if doc.Health != nil {
		cfg.Health = HealthConfig{Enabled: doc.Health.Enabled, Address: strings.TrimSpace(doc.Health.Address)}
	}
	if doc.Telemetry != nil {
		cfg.Telemetry = TelemetryConfig{
			Enabled: doc.Telemetry.Enabled, Endpoint: strings.TrimSpace(doc.Telemetry.Endpoint),
			ServiceName: doc.Telemetry.ServiceName, ServiceVersion: doc.Telemetry.ServiceVersion,
			TracesEnabled: doc.Telemetry.TracesEnabled, MetricsEnabled: doc.Telemetry.MetricsEnabled,
			TraceSampleRate: doc.Telemetry.TraceSampleRate, Insecure: doc.Telemetry.Insecure,
			CAFile: doc.Telemetry.CAFile, CertFile: doc.Telemetry.CertFile, KeyFile: doc.Telemetry.KeyFile,
		}
	}

	if err := normalizeListeners(cfg, *doc.Listeners); err != nil {
		return nil, err
	}
	if doc.Experimental != nil {
		if err := normalizeExperimental(cfg, *doc.Experimental); err != nil {
			return nil, err
		}
	}
	if doc.Cluster != nil && len(doc.Cluster.Members) > 0 {
		if err := normalizeCluster(cfg, *doc.Cluster, options.NodeID); err != nil {
			return nil, err
		}
	} else {
		cfg.Cluster = Default().Cluster
		cfg.Cluster.Enabled = false
		cfg.Cluster.NodeID = "single-node"
	}
	return cfg, nil
}

func normalizeListeners(cfg *Config, listeners listenersDocument) error {
	seenAddresses := make(map[string]string)
	for i, listener := range listeners.MQTT {
		path := fmt.Sprintf("listeners.mqtt[%d]", i)
		if err := validateAddress(path+".address", listener.Address, seenAddresses); err != nil {
			return err
		}
		if listener.Transport != MQTTTransportTCP && listener.Transport != MQTTTransportWebSocket {
			return fmt.Errorf("%s.transport must be %q or %q", path, MQTTTransportTCP, MQTTTransportWebSocket)
		}
		versions, err := normalizeMQTTVersions(path+".versions", listener.Versions)
		if err != nil {
			return err
		}
		if negative(listener.MaxConnections) {
			return fmt.Errorf("%s.max_connections cannot be negative", path)
		}
		if negative(listener.ReadTimeout) || negative(listener.WriteTimeout) {
			return fmt.Errorf("%s timeouts cannot be negative", path)
		}
		tlsConfig, err := normalizeListenerTLS(path+".tls", listener.TLS)
		if err != nil {
			return err
		}
		pathValue := listener.Path
		if pathValue == "" && listener.Transport == MQTTTransportWebSocket {
			pathValue = defaultWSPath
		}
		if listener.Transport == MQTTTransportTCP && (listener.Path != "" || len(listener.AllowedOrigins) > 0) {
			return fmt.Errorf("%s path and allowed_origins require transport: websocket", path)
		}
		cfg.Listeners.MQTT = append(cfg.Listeners.MQTT, MQTTListenerConfig{
			Address: listener.Address, Transport: listener.Transport, Versions: versions,
			Path: pathValue, AllowedOrigins: listener.AllowedOrigins,
			MaxConnections: orDefault(listener.MaxConnections, defaultMaxConnections),
			ReadTimeout:    orDefault(listener.ReadTimeout, defaultListenerTimeout),
			WriteTimeout:   orDefault(listener.WriteTimeout, defaultListenerTimeout), TLS: tlsConfig,
		})
	}
	for i, listener := range listeners.AMQP091 {
		path := fmt.Sprintf("listeners.amqp091[%d]", i)
		if err := validateAddress(path+".address", listener.Address, seenAddresses); err != nil {
			return err
		}
		if listener.Auth != AMQP091AuthExternal && listener.Auth != AMQP091AuthLocal {
			return fmt.Errorf("%s.auth must be %q or %q", path, AMQP091AuthExternal, AMQP091AuthLocal)
		}
		if negative(listener.MaxConnections) || negative(listener.HandshakeTimeout) {
			return fmt.Errorf("%s limits and timeouts cannot be negative", path)
		}
		tlsConfig, err := normalizeListenerTLS(path+".tls", listener.TLS)
		if err != nil {
			return err
		}
		if listener.Auth == AMQP091AuthLocal && (tlsConfig == nil || tlsConfig.ClientCAFile == "") {
			return fmt.Errorf("%s.auth local requires tls.client_ca_file", path)
		}
		cfg.Listeners.AMQP091 = append(cfg.Listeners.AMQP091, AMQP091ListenerConfig{
			Address: listener.Address, Auth: listener.Auth,
			MaxConnections:   orDefault(listener.MaxConnections, defaultMaxConnections),
			HandshakeTimeout: orDefault(listener.HandshakeTimeout, defaultHandshakeTimeout), TLS: tlsConfig,
		})
	}
	for i, listener := range listeners.AMQP1 {
		path := fmt.Sprintf("listeners.amqp1[%d]", i)
		if err := validateAddress(path+".address", listener.Address, seenAddresses); err != nil {
			return err
		}
		if negative(listener.MaxConnections) || negative(listener.HandshakeTimeout) {
			return fmt.Errorf("%s limits and timeouts cannot be negative", path)
		}
		tlsConfig, err := normalizeListenerTLS(path+".tls", listener.TLS)
		if err != nil {
			return err
		}
		cfg.Listeners.AMQP1 = append(cfg.Listeners.AMQP1, AMQP1ListenerConfig{
			Address: listener.Address, MaxConnections: orDefault(listener.MaxConnections, defaultMaxConnections),
			HandshakeTimeout: orDefault(listener.HandshakeTimeout, defaultHandshakeTimeout), TLS: tlsConfig,
		})
	}
	if len(cfg.Listeners.MQTT)+len(cfg.Listeners.AMQP091)+len(cfg.Listeners.AMQP1) == 0 {
		return errors.New("at least one stable messaging listener must be configured")
	}
	return nil
}

func validateNormalizedRuntime(cfg *Config) error {
	if len(cfg.Listeners.MQTT)+len(cfg.Listeners.AMQP091)+len(cfg.Listeners.AMQP1) == 0 {
		return errors.New("at least one stable messaging listener must be configured")
	}
	for i, listener := range cfg.Listeners.MQTT {
		path := fmt.Sprintf("listeners.mqtt[%d]", i)
		if strings.TrimSpace(listener.Address) == "" {
			return fmt.Errorf("%s.address cannot be empty", path)
		}
		if listener.Transport != MQTTTransportTCP && listener.Transport != MQTTTransportWebSocket {
			return fmt.Errorf("%s.transport is invalid", path)
		}
		if _, err := normalizeMQTTVersions(path+".versions", listener.Versions); err != nil {
			return err
		}
		if listener.MaxConnections < 0 || listener.ReadTimeout < 0 || listener.WriteTimeout < 0 {
			return fmt.Errorf("%s limits and timeouts cannot be negative", path)
		}
		if listener.TLS != nil && (listener.TLS.CertFile == "" || listener.TLS.KeyFile == "") {
			return fmt.Errorf("%s.tls requires cert_file and key_file", path)
		}
	}
	for i, listener := range cfg.Listeners.AMQP091 {
		path := fmt.Sprintf("listeners.amqp091[%d]", i)
		if strings.TrimSpace(listener.Address) == "" {
			return fmt.Errorf("%s.address cannot be empty", path)
		}
		if listener.Auth != AMQP091AuthExternal && listener.Auth != AMQP091AuthLocal {
			return fmt.Errorf("%s.auth is invalid", path)
		}
		if listener.Auth == AMQP091AuthLocal && (listener.TLS == nil || listener.TLS.ClientCAFile == "") {
			return fmt.Errorf("%s.auth local requires tls.client_ca_file", path)
		}
		if listener.Auth == AMQP091AuthLocal && len(cfg.Auth.LocalPrincipals) == 0 {
			return fmt.Errorf("%s.auth local requires auth.local_principals", path)
		}
		// An exact publish target is appended and synced on the receiving node
		// only, and is deliberately never forwarded to other nodes: forwarding
		// would acknowledge a publisher on a barrier no single node established.
		// In a cluster that makes those records unreachable from consumers
		// attached elsewhere, with nothing to signal it, so refuse the
		// combination rather than serve a principal whose records only some
		// readers can see.
		//
		// The permission decides this, not the listener, exactly as it decides
		// how a publication is routed. A prefix permission cannot name a queue,
		// so it never takes that single-node durable path and a principal
		// holding only prefix permissions may run clustered.
		//
		// A prefix publication may still be captured by a queue whose own topics
		// pattern matches it, and that append is likewise not forwarded to nodes
		// that already know the queue — remote consumers are served by the
		// delivery engine instead. That is not what this rule gates: capture
		// applies to every publisher on every protocol, so refusing a local
		// principal for it would single out the one publisher whose behavior is
		// declared in configuration. What is gated here is the durable-stream
		// path, which bypasses cluster distribution entirely by design.
		if listener.Auth == AMQP091AuthLocal && cfg.Cluster.Enabled {
			if name, target, found := firstExactPublishTarget(cfg.Auth.LocalPrincipals); found {
				return fmt.Errorf("auth.local_principals %q grants exact publish target %q, which cannot be combined with cluster.members because exact-target records are durable only on the receiving node; use a routing_key_prefix permission or a single-node deployment for %s", name, target, path)
			}
		}
		if listener.MaxConnections < 0 || listener.HandshakeTimeout < 0 {
			return fmt.Errorf("%s limits and timeouts cannot be negative", path)
		}
		if listener.TLS != nil && (listener.TLS.CertFile == "" || listener.TLS.KeyFile == "") {
			return fmt.Errorf("%s.tls requires cert_file and key_file", path)
		}
	}
	for i, listener := range cfg.Listeners.AMQP1 {
		path := fmt.Sprintf("listeners.amqp1[%d]", i)
		if strings.TrimSpace(listener.Address) == "" {
			return fmt.Errorf("%s.address cannot be empty", path)
		}
		if listener.MaxConnections < 0 || listener.HandshakeTimeout < 0 {
			return fmt.Errorf("%s limits and timeouts cannot be negative", path)
		}
		if listener.TLS != nil && (listener.TLS.CertFile == "" || listener.TLS.KeyFile == "") {
			return fmt.Errorf("%s.tls requires cert_file and key_file", path)
		}
	}
	return nil
}

func normalizeExperimental(cfg *Config, experimental experimentalDocument) error {
	cfg.Experimental.HTTP.Enabled = experimental.HTTP.Enabled
	if !experimental.HTTP.Enabled && len(experimental.HTTP.Listeners) > 0 {
		return errors.New("experimental.http.listeners requires experimental.http.enabled: true")
	}
	for i, listener := range experimental.HTTP.Listeners {
		path := fmt.Sprintf("experimental.http.listeners[%d]", i)
		if strings.TrimSpace(listener.Address) == "" {
			return fmt.Errorf("%s.address cannot be empty", path)
		}
		tlsConfig, err := normalizeListenerTLS(path+".tls", listener.TLS)
		if err != nil {
			return err
		}
		cfg.Experimental.HTTP.Listeners = append(cfg.Experimental.HTTP.Listeners, ExperimentalHTTPListenerConfig{Address: listener.Address, TLS: tlsConfig})
	}
	cfg.Experimental.CoAP.Enabled = experimental.CoAP.Enabled
	if !experimental.CoAP.Enabled && len(experimental.CoAP.Listeners) > 0 {
		return errors.New("experimental.coap.listeners requires experimental.coap.enabled: true")
	}
	for i, listener := range experimental.CoAP.Listeners {
		path := fmt.Sprintf("experimental.coap.listeners[%d]", i)
		if strings.TrimSpace(listener.Address) == "" {
			return fmt.Errorf("%s.address cannot be empty", path)
		}
		tlsConfig, err := normalizeListenerTLS(path+".tls", listener.TLS)
		if err != nil {
			return err
		}
		cfg.Experimental.CoAP.Listeners = append(cfg.Experimental.CoAP.Listeners, ExperimentalCoAPListenerConfig{Address: listener.Address, TLS: tlsConfig})
	}

	raft := experimental.QueueRaft
	cfg.Experimental.QueueRaft = ExperimentalQueueRaftConfig{
		Enabled: raft.Enabled, Port: raft.Port, AutoProvisionGroups: raft.AutoProvisionGroups,
		ReplicationFactor: raft.ReplicationFactor, SyncMode: raft.SyncMode,
		MinInSyncReplicas: raft.MinInSyncReplicas, AckTimeout: raft.AckTimeout,
		WritePolicy: raft.WritePolicy, DistributionMode: raft.DistributionMode,
		HeartbeatTimeout: raft.HeartbeatTimeout, ElectionTimeout: raft.ElectionTimeout,
		SnapshotInterval: raft.SnapshotInterval, SnapshotThreshold: raft.SnapshotThreshold, Groups: raft.Groups,
	}
	if raft.Enabled && raft.Port <= 0 {
		return errors.New("experimental.queue_raft.port must be between 1 and 65535")
	}
	return nil
}

func normalizeCluster(cfg *Config, cluster clusterDocument, nodeID string) error {
	if len(cluster.Members) == 0 {
		return errors.New("cluster.members must not be empty")
	}
	selectedNodeID := strings.TrimSpace(nodeID)
	if selectedNodeID == "" {
		selectedNodeID = strings.TrimSpace(os.Getenv("FLUXMQ_NODE_ID"))
	}
	if selectedNodeID == "" {
		return errors.New("cluster node ID is required; use --node-id or FLUXMQ_NODE_ID")
	}
	localHost, exists := cluster.Members[selectedNodeID]
	if !exists {
		return fmt.Errorf("cluster node ID %q is not present in cluster.members", selectedNodeID)
	}
	for id, host := range cluster.Members {
		if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
			return errors.New("cluster.members IDs must be non-empty and have no surrounding whitespace")
		}
		if strings.TrimSpace(host) == "" || strings.Contains(host, "://") {
			return fmt.Errorf("cluster.members.%s must be a host name or IP address without a scheme", id)
		}
	}
	ports := ClusterPortsConfig{EtcdPeer: cluster.Ports.EtcdPeer, Transport: cluster.Ports.Transport}
	if ports.EtcdPeer == 0 {
		ports.EtcdPeer = 2380
	}
	if ports.Transport == 0 {
		ports.Transport = 7948
	}
	if !validPort(ports.EtcdPeer) || !validPort(ports.Transport) || ports.EtcdPeer == ports.Transport {
		return errors.New("cluster.ports must contain distinct ports between 1 and 65535")
	}

	tlsConfig := ClusterTLSConfig{}
	if cluster.TLS != nil {
		tlsConfig = ClusterTLSConfig{CAFile: cluster.TLS.CAFile, CertFile: cluster.TLS.CertFile, KeyFile: cluster.TLS.KeyFile}
	}
	if tlsConfig == (ClusterTLSConfig{}) {
		if !cluster.AllowInsecure {
			return errors.New("cluster.tls is required unless cluster.allow_insecure is true")
		}
	} else if tlsConfig.CAFile == "" || tlsConfig.CertFile == "" || tlsConfig.KeyFile == "" {
		return errors.New("cluster.tls.ca_file, cert_file, and key_file are all required")
	}

	scheme := "https"
	if cluster.AllowInsecure && tlsConfig == (ClusterTLSConfig{}) {
		scheme = protocolHTTP
	}
	ids := make([]string, 0, len(cluster.Members))
	for id := range cluster.Members {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	initialMembers := make([]string, 0, len(ids))
	transportPeers := make(map[string]string, len(ids))
	for _, id := range ids {
		host := cluster.Members[id]
		initialMembers = append(initialMembers, id+"="+scheme+"://"+net.JoinHostPort(host, strconv.Itoa(ports.EtcdPeer)))
		transportPeers[id] = net.JoinHostPort(host, strconv.Itoa(ports.Transport))
	}
	storageRoot := cfg.Storage.DataDir
	if storageRoot == "" {
		return errors.New("storage.data_dir is required for cluster mode")
	}
	nodeDataDir := filepath.Join(storageRoot, "cluster", selectedNodeID)

	cfg.Cluster.Enabled = true
	cfg.Cluster.NodeID = selectedNodeID
	cfg.Cluster.Members = cloneStringMap(cluster.Members)
	cfg.Cluster.Ports = ports
	cfg.Cluster.TLS = tlsConfig
	cfg.Cluster.AllowInsecure = cluster.AllowInsecure
	cfg.Cluster.ManifestFingerprint = clusterFingerprint(initialMembers)
	cfg.Cluster.Etcd = EtcdConfig{
		DataDir: filepath.Join(nodeDataDir, "etcd"), BindAddr: net.JoinHostPort("0.0.0.0", strconv.Itoa(ports.EtcdPeer)),
		AdvertiseAddr: net.JoinHostPort(localHost, strconv.Itoa(ports.EtcdPeer)),
		ClientAddr:    "127.0.0.1:2379", InitialCluster: strings.Join(initialMembers, ","), Bootstrap: true,
		HybridRetainedSizeThreshold: 1024,
	}
	cfg.Cluster.Transport = TransportConfig{
		BindAddr: net.JoinHostPort("0.0.0.0", strconv.Itoa(ports.Transport)), Peers: transportPeers,
		RouteBatchMaxSize: 256, RouteBatchMaxDelay: 5 * time.Millisecond,
		RouteBatchFlushWorkers: 4, RoutePublishTimeout: 15 * time.Second,
		TLSEnabled: tlsConfig != (ClusterTLSConfig{}), TLSCAFile: tlsConfig.CAFile,
		TLSCertFile: tlsConfig.CertFile, TLSKeyFile: tlsConfig.KeyFile,
	}
	deriveQueueRaft(cfg, nodeDataDir)
	return nil
}

func deriveQueueRaft(cfg *Config, nodeDataDir string) {
	raft := cfg.Experimental.QueueRaft
	cfg.Cluster.Raft.Enabled = raft.Enabled
	if !raft.Enabled {
		return
	}
	peers := make(map[string]string, len(cfg.Cluster.Members))
	for id, host := range cfg.Cluster.Members {
		peers[id] = net.JoinHostPort(host, strconv.Itoa(raft.Port))
	}
	cfg.Cluster.Raft = RaftConfig{
		Enabled: true, AutoProvisionGroups: raft.AutoProvisionGroups,
		ReplicationFactor: raft.ReplicationFactor, SyncMode: raft.SyncMode,
		MinInSyncReplicas: raft.MinInSyncReplicas, AckTimeout: raft.AckTimeout,
		WritePolicy: raft.WritePolicy, DistributionMode: raft.DistributionMode,
		BindAddr: net.JoinHostPort("0.0.0.0", strconv.Itoa(raft.Port)), DataDir: filepath.Join(nodeDataDir, "queue-raft"),
		Peers: peers, HeartbeatTimeout: raft.HeartbeatTimeout, ElectionTimeout: raft.ElectionTimeout,
		SnapshotInterval: raft.SnapshotInterval, SnapshotThreshold: raft.SnapshotThreshold, Groups: raft.Groups,
	}
}

func normalizeListenerTLS(path string, tlsDoc *listenerTLSDocument) (*mqtttls.Config, error) {
	if tlsDoc == nil {
		return nil, nil
	}
	if tlsDoc.CertFile == "" {
		return nil, fmt.Errorf("%s.cert_file is required", path)
	}
	if tlsDoc.KeyFile == "" {
		return nil, fmt.Errorf("%s.key_file is required", path)
	}
	clientAuth := ""
	if tlsDoc.ClientCAFile != "" {
		clientAuth = clientAuthRequire
	}
	return &mqtttls.Config{
		CertFile: tlsDoc.CertFile, KeyFile: tlsDoc.KeyFile, ClientCAFile: tlsDoc.ClientCAFile,
		ClientAuth: clientAuth, MinVersion: tlsDoc.MinVersion, CipherSuites: tlsDoc.CipherSuites,
	}, nil
}

func normalizeMQTTVersions(path string, versions []string) ([]string, error) {
	if len(versions) == 0 {
		return nil, fmt.Errorf("%s must contain at least one version", path)
	}
	seen := make(map[string]struct{}, len(versions))
	result := make([]string, 0, len(versions))
	for _, version := range versions {
		if version != MQTTVersion311 && version != MQTTVersion50 {
			return nil, fmt.Errorf("%s contains unsupported MQTT version %q", path, version)
		}
		if _, exists := seen[version]; exists {
			return nil, fmt.Errorf("%s contains duplicate MQTT version %q", path, version)
		}
		seen[version] = struct{}{}
		result = append(result, version)
	}
	sort.Strings(result)
	return result, nil
}

func validateAddress(path, address string, seen map[string]string) error {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return fmt.Errorf("%s cannot be empty", path)
	}
	if previous, exists := seen[trimmed]; exists {
		return fmt.Errorf("%s duplicates %s", path, previous)
	}
	seen[trimmed] = path
	return nil
}

// negative reports whether a configured value is below zero. Unset and zero are
// both fine: zero is the explicit "unlimited" or "no deadline" setting.
func negative[T int | time.Duration](value *T) bool {
	return value != nil && *value < 0
}

// orDefault returns the configured value, or the fallback when the key was
// omitted. An explicit zero is preserved: it means unlimited for a limit and no
// deadline for a timeout.
func orDefault[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// clusterFingerprint pins a data directory to the etcd peer URLs that formed
// it. etcd persists those URLs, so the fingerprint must cover everything they
// are built from — member IDs, hosts, the peer port, and the scheme that
// cluster.tls or cluster.allow_insecure selects. Hashing only the member map
// would let a changed port or a flip to plaintext pass the check and then
// disagree with etcd's own recorded membership.
func clusterFingerprint(initialMembers []string) string {
	hash := sha256.New()
	for _, member := range initialMembers {
		_, _ = io.WriteString(hash, member)
		_, _ = io.WriteString(hash, "\n")
	}
	return hex.EncodeToString(hash.Sum(nil))
}
