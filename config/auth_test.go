// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mqtttls "github.com/absmach/fluxmq/pkg/tls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPrincipalName = "audit-publisher"
	testPrincipalSAN  = "spiffe://example.org/audit-publisher"
	testAuditQueue    = "audit.events"
	testInternalAddr  = ":5683"
	testServiceAddr   = ":5684"
	testServerCert    = "server.crt"
	testServerKey     = "server.key"
	testClientCA      = "clients.crt"
	testBadPattern    = "is not a valid queue pattern"
)

const testV1Prefix = `version: 1
listeners:
  mqtt:
    - address: "127.0.0.1:1883"
      transport: tcp
      versions: ["3.1.1", "5.0"]
  amqp091: []
  amqp1: []
`

func TestLoadNestedAuth(t *testing.T) {
	dir := t.TempDir()
	current := writeSecret(t, dir, "current", strings.Repeat("a", 32)+"\n")
	previous := writeSecret(t, dir, "previous", strings.Repeat("b", 32)+"\r\n")
	filename := filepath.Join(dir, "config.yaml")
	contents := testV1Prefix + fmt.Sprintf(`
auth:
  external:
    url: "http://auth.internal:8181"
    transport: "http"
    timeout: 2s
    protocols:
      amqp091: true
    identity_cache_size: 123
    identity_cache_ttl: 1h
  local_principals:
    - name: %q
      certificate_uri_san: %q
      current_secret_file: %q
      previous_secret_file: %q
      permissions:
        publish:
          - exchange: ""
            routing_key: audit.events
        subscribe: []
`, testPrincipalName, testPrincipalSAN, current, previous)
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(filename)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Auth.External.URL != "http://auth.internal:8181" {
		t.Fatalf("external URL = %q", cfg.Auth.External.URL)
	}
	if cfg.Auth.External.Timeout != 2*time.Second {
		t.Fatalf("external timeout = %v", cfg.Auth.External.Timeout)
	}
	if !cfg.Auth.External.EnabledFor(protocolAMQP091) {
		t.Fatal("expected AMQP 0.9.1 external auth to be enabled")
	}
	if len(cfg.Auth.LocalPrincipals) != 1 {
		t.Fatalf("local principal count = %d, want 1", len(cfg.Auth.LocalPrincipals))
	}
	principal := cfg.Auth.LocalPrincipals[0]
	if principal.Name != testPrincipalName || principal.CertificateURISAN != testPrincipalSAN {
		t.Fatalf("unexpected local principal: %+v", principal)
	}
	if len(principal.Permissions.Publish) != 1 || principal.Permissions.Publish[0].RoutingKey != testAuditQueue {
		t.Fatalf("unexpected publish permissions: %+v", principal.Permissions.Publish)
	}
}

func TestLoadRejectsLegacyAuthKeys(t *testing.T) {
	tests := []string{
		authURLField, authTransportField, authTimeoutField, authProtocolsField,
		authIdentityCacheSizeField, authIdentityCacheTTLField,
	}

	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "config.yaml")
			contents := testV1Prefix + fmt.Sprintf("auth:\n  %s: value\n", key)
			if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := Load(filename)
			if err == nil {
				t.Fatal("Load() succeeded with a legacy auth key")
			}
			want := fmt.Sprintf("auth.%s: unknown field", key)
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("Load() error = %q, want it to contain %q", err, want)
			}
		})
	}
}

func TestLoadRejectsUnknownAuthFields(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filename, []byte(testV1Prefix+"auth:\n  external:\n    unsupported: true\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(filename)
	if err == nil || !strings.Contains(err.Error(), "auth.external.unsupported: unknown field") {
		t.Fatalf("Load() error = %v, want strict unknown-field error", err)
	}
}

func TestValidateLocalPrincipals(t *testing.T) {
	dir := t.TempDir()
	current := writeSecret(t, dir, "current", strings.Repeat("a", 32)+"\n")
	previous := writeSecret(t, dir, "previous", strings.Repeat("b", 32)+"\r\n")

	valid := func() LocalPrincipalConfig {
		return LocalPrincipalConfig{
			Name:               testPrincipalName,
			CertificateURISAN:  testPrincipalSAN,
			CurrentSecretFile:  current,
			PreviousSecretFile: previous,
			Permissions: LocalPermissionsConfig{
				Publish: []LocalPublishPermission{{Exchange: "", RoutingKey: testAuditQueue}},
			},
		}
	}

	tests := []struct {
		name       string
		principals func() []LocalPrincipalConfig
		wantError  string
	}{
		{
			name:       "valid current and previous secret",
			principals: func() []LocalPrincipalConfig { return []LocalPrincipalConfig{valid()} },
		},
		{
			name: "duplicate name",
			principals: func() []LocalPrincipalConfig {
				first, second := valid(), valid()
				second.CertificateURISAN = "spiffe://example.org/other"
				return []LocalPrincipalConfig{first, second}
			},
			wantError: ".name \"audit-publisher\" is duplicated",
		},
		{
			name: "duplicate URI SAN",
			principals: func() []LocalPrincipalConfig {
				first, second := valid(), valid()
				second.Name = "other"
				return []LocalPrincipalConfig{first, second}
			},
			wantError: ".certificate_uri_san \"spiffe://example.org/audit-publisher\" is duplicated",
		},
		{
			name: "missing current secret path",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.CurrentSecretFile = ""
				return []LocalPrincipalConfig{principal}
			},
			wantError: ".current_secret_file cannot be empty",
		},
		{
			name: "invalid URI SAN",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.CertificateURISAN = "not-an-absolute-uri"
				return []LocalPrincipalConfig{principal}
			},
			wantError: "must be an absolute URI",
		},
		{
			name: "publish wildcard",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Permissions.Publish[0].RoutingKey = "audit-*"
				return []LocalPrincipalConfig{principal}
			},
			wantError: "routing_key must be an exact value without wildcards",
		},
		{
			name: "non-default publish exchange",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Permissions.Publish[0].Exchange = "events"
				return []LocalPrincipalConfig{principal}
			},
			wantError: "exchange must be empty; local principals may publish only through the AMQP default exchange",
		},
		{
			name: "publish permission cannot set both an exact key and a prefix",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Permissions.Publish = []LocalPublishPermission{
					{RoutingKey: testAuditQueue, RoutingKeyPrefix: "m."},
				}
				return []LocalPrincipalConfig{principal}
			},
			wantError: "cannot set both routing_key and routing_key_prefix",
		},
		{
			name: "publish permission must set one of them",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Permissions.Publish = []LocalPublishPermission{{}}
				return []LocalPrincipalConfig{principal}
			},
			wantError: "must set either routing_key or routing_key_prefix",
		},
		{
			name: "publish prefix cannot contain wildcards",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Permissions.Publish = []LocalPublishPermission{{RoutingKeyPrefix: "m.#"}}
				return []LocalPrincipalConfig{principal}
			},
			wantError: "must not contain wildcards",
		},
		{
			name: "publish prefix cannot have surrounding whitespace",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Permissions.Publish = []LocalPublishPermission{{RoutingKeyPrefix: " m."}}
				return []LocalPrincipalConfig{principal}
			},
			wantError: "cannot have leading or trailing whitespace",
		},
		{
			name: "subscribe permission cannot be empty",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{""}
				return []LocalPrincipalConfig{principal}
			},
			wantError: "permissions.subscribe[0] cannot be empty",
		},
		{
			name: "subscribe permission accepts an AMQP-style wildcard",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{testAuditQueue + ".*"}
				return []LocalPrincipalConfig{principal}
			},
		},
		{
			name: "subscribe permission accepts an MQTT-style wildcard",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{testAuditQueue + ".+"}
				return []LocalPrincipalConfig{principal}
			},
		},
		{
			name: "subscribe permission accepts a queue name with a leading dollar",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{"$internal"}
				return []LocalPrincipalConfig{principal}
			},
		},
		{
			name: "subscribe permission accepts a queue name containing a slash",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{"a/b"}
				return []LocalPrincipalConfig{principal}
			},
		},
		{
			name: "subscribe permission rejects an MQTT-shaped wildcard",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{"audit/+"}
				return []LocalPrincipalConfig{principal}
			},
			wantError: testBadPattern,
		},
		{
			name: "subscribe permission rejects a malformed wildcard",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{testAuditQueue + ".#.tail"}
				return []LocalPrincipalConfig{principal}
			},
			wantError: testBadPattern,
		},
		{
			name: "subscribe permission rejects a partial-level wildcard",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{testAuditQueue + ".ev*"}
				return []LocalPrincipalConfig{principal}
			},
			wantError: testBadPattern,
		},
		{
			name: "subscribe permission rejects a queue address",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{"$queue/" + testAuditQueue + "/#"}
				return []LocalPrincipalConfig{principal}
			},
			wantError: "must name a queue rather than a queue address",
		},
		{
			name: "subscribe permission rejects one spelling duplicating another",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{testAuditQueue + ".*", testAuditQueue + ".+"}
				return []LocalPrincipalConfig{principal}
			},
			wantError: "duplicates an earlier subscribe permission",
		},
		{
			name: "subscribe permission cannot have surrounding whitespace",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{" " + testAuditQueue}
				return []LocalPrincipalConfig{principal}
			},
			wantError: "cannot have leading or trailing whitespace",
		},
		{
			name: "subscribe permission requires the service role",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Permissions.Subscribe = []string{testAuditQueue}
				return []LocalPrincipalConfig{principal}
			},
			wantError: "permissions.subscribe requires role \"service\"",
		},
		{
			name: "unknown role is rejected",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = "administrator"
				return []LocalPrincipalConfig{principal}
			},
			wantError: "role \"administrator\" is unknown",
		},
		{
			name: "role defaults to the least privileged one",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = ""
				return []LocalPrincipalConfig{principal}
			},
		},
		{
			name: "subscribe permission cannot be duplicated",
			principals: func() []LocalPrincipalConfig {
				principal := valid()
				principal.Role = LocalRoleService
				principal.Permissions.Subscribe = []string{testAuditQueue, testAuditQueue}
				return []LocalPrincipalConfig{principal}
			},
			wantError: "duplicates an earlier subscribe permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Auth.LocalPrincipals = tt.principals()
			err := cfg.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Validate() error = %v, want it to contain %q", err, tt.wantError)
			}
		})
	}
}

// The local listener authenticates against the local principal store and uses
// the single-node durable path for exact publish targets.
func TestValidateLocalAMQP091Listener(t *testing.T) {
	tests := []struct {
		name           string
		listener       *AMQP091ListenerConfig
		clusterEnabled bool
		withPrincipal  bool
		publish        []LocalPublishPermission
		wantError      string
	}{
		{
			name:     "no local listener",
			listener: nil,
		},
		{
			name:      "requires client CA",
			listener:  localListener(&mqtttls.Config{CertFile: testServerCert, KeyFile: testServerKey}),
			wantError: errLocalRequiresClientCA,
		},
		{
			name:      "requires TLS",
			listener:  localListener(nil),
			wantError: errLocalRequiresClientCA,
		},
		{
			name:      "requires a local principal",
			listener:  validLocalListener(),
			wantError: "listeners.amqp091[0].auth local requires auth.local_principals",
		},
		{
			name:          "rejects a negative connection limit",
			listener:      func() *AMQP091ListenerConfig { l := validLocalListener(); l.MaxConnections = -1; return l }(),
			withPrincipal: true,
			wantError:     "max_connections cannot be negative",
		},
		{
			// An exact target is appended on the receiving node only, so a
			// cluster would hide those records from consumers elsewhere.
			name:           "rejects clustering with an exact publish target",
			listener:       validLocalListener(),
			clusterEnabled: true,
			withPrincipal:  true,
			publish:        []LocalPublishPermission{{RoutingKey: testAuditQueue}},
			wantError:      "cannot be combined with cluster.members",
		},
		{
			// A prefix names no queue and is an ordinary topic publish, which
			// the cluster forwards like any other, so it may run clustered.
			name:           "allows clustering with prefix permissions only",
			listener:       validLocalListener(),
			clusterEnabled: true,
			withPrincipal:  true,
			publish:        []LocalPublishPermission{{RoutingKeyPrefix: "m."}},
		},
		{
			name:          "valid mandatory mTLS",
			listener:      validLocalListener(),
			withPrincipal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			// Local principals are a single-node feature, so these cases
			// configure clustering explicitly.
			cfg.Cluster.Enabled = tt.clusterEnabled
			if tt.listener != nil {
				cfg.Listeners.AMQP091 = []AMQP091ListenerConfig{*tt.listener}
			}
			if tt.withPrincipal {
				cfg.Auth.LocalPrincipals = []LocalPrincipalConfig{{
					Name:              testPrincipalName,
					CertificateURISAN: testPrincipalSAN,
					CurrentSecretFile: writeSecret(t, t.TempDir(), "current", strings.Repeat("a", 32)),
					Permissions:       LocalPermissionsConfig{Publish: tt.publish},
				}}
			}

			err := cfg.Validate()
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func localListener(tlsConfig *mqtttls.Config) *AMQP091ListenerConfig {
	return &AMQP091ListenerConfig{
		Address: testInternalAddr, Auth: AMQP091AuthLocal, MaxConnections: 32,
		HandshakeTimeout: 10 * time.Second, TLS: tlsConfig,
	}
}

func validLocalListener() *AMQP091ListenerConfig {
	return localListener(&mqtttls.Config{
		CertFile: testServerCert, KeyFile: testServerKey,
		ClientCAFile: testClientCA, ClientAuth: clientAuthRequire,
	})
}

// A local listener admits principals by client certificate, so the loader must
// derive mandatory client-certificate verification from client_ca_file alone:
// v1 has no client_auth key for an operator to weaken.
func TestLoadLocalAMQP091ListenerRequiresClientCertificates(t *testing.T) {
	dir := t.TempDir()
	secret := writeSecret(t, dir, "current", strings.Repeat("a", 32))
	filename := filepath.Join(dir, "config.yaml")
	body := `version: 1

listeners:
  mqtt: []
  amqp1: []
  amqp091:
    - address: "` + testInternalAddr + `"
      auth: local
      tls:
        cert_file: "` + testServerCert + `"
        key_file: "` + testServerKey + `"
        client_ca_file: "` + testClientCA + `"

storage:
  type: memory

auth:
  local_principals:
    - name: "` + testPrincipalName + `"
      certificate_uri_san: "` + testPrincipalSAN + `"
      current_secret_file: "` + secret + `"
      permissions:
        publish:
          - routing_key: "` + testAuditQueue + `"
`
	require.NoError(t, os.WriteFile(filename, []byte(body), 0o600))

	cfg, err := Load(filename)
	require.NoError(t, err)
	require.Len(t, cfg.Listeners.AMQP091, 1)
	require.NotNil(t, cfg.Listeners.AMQP091[0].TLS)
	assert.Equal(t, clientAuthRequire, cfg.Listeners.AMQP091[0].TLS.ClientAuth)
	assert.True(t, hasLocalAMQP091Listener(cfg))
}

func TestHasLocalAMQP091ListenerIgnoresExternalListeners(t *testing.T) {
	cfg := Default()
	cfg.Listeners.AMQP091 = []AMQP091ListenerConfig{{
		Address: testServiceAddr, Auth: AMQP091AuthExternal, MaxConnections: 32,
	}}
	assert.False(t, hasLocalAMQP091Listener(cfg))
}

// Configuration validation names the secret files but never opens them, so an
// operator or CI job can check a production configuration on a machine that has
// no /run/secrets. The contents are enforced in broker/localauth, which runs at
// startup and on every reload.
func TestValidateDoesNotReadSecretFiles(t *testing.T) {
	cfg := Default()
	cfg.Listeners.AMQP091 = []AMQP091ListenerConfig{*validLocalListener()}
	cfg.Auth.LocalPrincipals = []LocalPrincipalConfig{{
		Name:               testPrincipalName,
		CertificateURISAN:  testPrincipalSAN,
		CurrentSecretFile:  "/run/secrets/absent-current",
		PreviousSecretFile: "/run/secrets/absent-previous",
		Permissions: LocalPermissionsConfig{
			Publish: []LocalPublishPermission{{RoutingKeyPrefix: "m."}},
		},
	}}

	require.NoError(t, cfg.Validate(), "validation must not depend on secret files being present")
}

// The paths themselves are configuration, so a missing or blank one is still a
// configuration error.
func TestValidateRejectsUnnamedSecretFiles(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*LocalPrincipalConfig)
		wantError string
	}{
		{
			name:      "current omitted",
			mutate:    func(p *LocalPrincipalConfig) { p.CurrentSecretFile = "" },
			wantError: "current_secret_file cannot be empty",
		},
		{
			name:      "current blank",
			mutate:    func(p *LocalPrincipalConfig) { p.CurrentSecretFile = "   " },
			wantError: "current_secret_file cannot be empty",
		},
		{
			name:      "previous blank",
			mutate:    func(p *LocalPrincipalConfig) { p.PreviousSecretFile = "   " },
			wantError: "previous_secret_file cannot be empty",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			principal := LocalPrincipalConfig{
				Name:              testPrincipalName,
				CertificateURISAN: testPrincipalSAN,
				CurrentSecretFile: "/run/secrets/current",
				Permissions: LocalPermissionsConfig{
					Publish: []LocalPublishPermission{{RoutingKeyPrefix: "m."}},
				},
			}
			test.mutate(&principal)

			err := ValidateLocalPrincipals([]LocalPrincipalConfig{principal})
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantError)
		})
	}
}

func writeSecret(t *testing.T, dir, name, contents string) string {
	t.Helper()
	filename := filepath.Join(dir, name)
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	return filename
}

// The auth subtree is decoded strictly, so a permission field the allowlist
// omits is rejected at parse time however valid the struct would be. Parsing
// real YAML is the only thing that catches that; validating a struct built in
// Go never reaches the decoder.
func TestLoadAcceptsPublishPermissionFields(t *testing.T) {
	tests := []struct {
		name       string
		permission string
	}{
		{name: "exact routing key", permission: "routing_key: \"audit.events\""},
		{name: "routing key prefix", permission: "routing_key_prefix: \"m.\""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			secret := filepath.Join(dir, "secret")
			require.NoError(t, os.WriteFile(secret, []byte(strings.Repeat("a", 32)), 0o600))

			filename := filepath.Join(dir, "config.yaml")
			body := testV1Prefix + "auth:\n" +
				"  local_principals:\n" +
				"    - name: \"svc\"\n" +
				"      certificate_uri_san: \"spiffe://absmach/svc\"\n" +
				"      role: \"service\"\n" +
				"      current_secret_file: \"" + secret + "\"\n" +
				"      permissions:\n" +
				"        publish:\n" +
				"          - " + tc.permission + "\n" +
				"        subscribe:\n" +
				"          - \"m\"\n"
			require.NoError(t, os.WriteFile(filename, []byte(body), 0o600))

			cfg, err := Load(filename)
			require.NoError(t, err)
			require.Len(t, cfg.Auth.LocalPrincipals, 1)
			require.Len(t, cfg.Auth.LocalPrincipals[0].Permissions.Publish, 1)
			assert.Equal(t, []string{"m"}, cfg.Auth.LocalPrincipals[0].Permissions.Subscribe)
		})
	}
}

// Clustering is restart-required while local principals are runtime-safe, so a
// reload that disables clustering and adds an exact publish target in one edit
// would otherwise apply the target inside a still-clustered runtime.
func TestValidateAgainstRuntimeRefusesExactTargetWhileClustered(t *testing.T) {
	dir := t.TempDir()
	secret := writeSecret(t, dir, "current", strings.Repeat("a", 32))

	withPermission := func(clusterEnabled bool, permission LocalPublishPermission) *Config {
		cfg := Default()
		cfg.Cluster.Enabled = clusterEnabled
		cfg.Listeners.AMQP091 = []AMQP091ListenerConfig{*validLocalListener()}
		cfg.Auth.LocalPrincipals = []LocalPrincipalConfig{{
			Name:              testPrincipalName,
			CertificateURISAN: testPrincipalSAN,
			CurrentSecretFile: secret,
			Permissions:       LocalPermissionsConfig{Publish: []LocalPublishPermission{permission}},
		}}
		return cfg
	}

	prefixOnly := LocalPublishPermission{RoutingKeyPrefix: "m."}
	exact := LocalPublishPermission{RoutingKey: testAuditQueue}

	t.Run("adding an exact target under a clustered runtime is refused", func(t *testing.T) {
		err := ValidateAgainstRuntime(withPermission(true, prefixOnly), withPermission(false, exact))
		require.Error(t, err, "the desired config disables clustering, but the running node has not restarted")
		assert.Contains(t, err.Error(), "while the running node is clustered")
	})

	t.Run("keeping only prefixes under a clustered runtime is allowed", func(t *testing.T) {
		assert.NoError(t, ValidateAgainstRuntime(withPermission(true, prefixOnly), withPermission(true, prefixOnly)))
	})

	t.Run("an exact target on an unclustered runtime is allowed", func(t *testing.T) {
		assert.NoError(t, ValidateAgainstRuntime(withPermission(false, prefixOnly), withPermission(false, exact)))
	})

	t.Run("removing the running listener does not make the reload safe", func(t *testing.T) {
		next := withPermission(false, exact)
		next.Listeners.AMQP091 = nil
		err := ValidateAgainstRuntime(withPermission(true, prefixOnly), next)
		require.Error(t, err, "the running listener remains active until restart")
		assert.Contains(t, err.Error(), "while the running node is clustered")
	})

	t.Run("no running local listener means no local publication to strand", func(t *testing.T) {
		running := withPermission(true, prefixOnly)
		running.Listeners.AMQP091 = nil
		assert.NoError(t, ValidateAgainstRuntime(running, withPermission(false, exact)))
	})
}
