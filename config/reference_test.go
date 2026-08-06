// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

var updateReference = flag.Bool("update-reference", false, "rewrite the generated key table in the configuration reference")

const (
	referenceRequired = "required"

	referencePath = "../docs/content/docs/reference/configuration-reference.md"
	referenceHead = "<!-- BEGIN GENERATED KEYS -->"
	referenceTail = "<!-- END GENERATED KEYS -->"
)

// The configuration reference is the contract for everyone outside this
// repository, so it is generated from the same document types the loader walks
// rather than maintained by hand. Run
//
//	go test ./config -run TestConfigurationReferenceIsCurrent -update-reference
//
// to rewrite it after changing a key.
func TestConfigurationReferenceIsCurrent(t *testing.T) {
	generated := renderReferenceTable()

	current, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatalf("read configuration reference: %v", err)
	}
	updated, err := replaceReferenceBlock(string(current), generated)
	if err != nil {
		t.Fatal(err)
	}

	if *updateReference {
		if err := os.WriteFile(referencePath, []byte(updated), 0o644); err != nil { //nolint:gosec // documentation file
			t.Fatalf("write configuration reference: %v", err)
		}
		return
	}

	if updated != string(current) {
		t.Error("the configuration reference no longer matches the schema; " +
			"rerun with -update-reference to regenerate it")
	}
}

// Every key the loader accepts must reach the reference. A key that exists but
// is undocumented is a contract nobody outside this repository can see.
func TestConfigurationReferenceCoversEveryKey(t *testing.T) {
	current, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatalf("read configuration reference: %v", err)
	}
	text := string(current)

	for _, path := range collectSchemaPaths(reflect.TypeFor[document](), "") {
		if !strings.Contains(text, "`"+path+"`") {
			t.Errorf("configuration key %q is absent from the reference", path)
		}
	}
}

func replaceReferenceBlock(document, block string) (string, error) {
	start := strings.Index(document, referenceHead)
	end := strings.Index(document, referenceTail)
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("configuration reference is missing the %s / %s markers", referenceHead, referenceTail)
	}
	return document[:start+len(referenceHead)] + "\n\n" + block + "\n" + document[end:], nil
}

func renderReferenceTable() string {
	defaults := referenceDefaults()

	var rows []string
	for _, path := range collectSchemaPaths(reflect.TypeFor[document](), "") {
		rows = append(rows, fmt.Sprintf("| `%s` | %s | %s |", path, referenceType(path), defaults[path]))
	}
	sort.Strings(rows)

	var out strings.Builder
	out.WriteString("| Key | Type | Default |\n| --- | --- | --- |\n")
	out.WriteString(strings.Join(rows, "\n"))
	out.WriteString("\n")
	return out.String()
}

// referenceType reports the YAML shape of a key, derived from the document
// types so it cannot disagree with what the decoder accepts.
func referenceType(path string) string {
	typ, ok := lookupSchemaType(reflect.TypeFor[document](), path)
	if !ok {
		return "—"
	}
	switch typ.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.String:
		return "string"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Int64:
		if typ == reflect.TypeOf(time.Duration(0)) {
			return "duration"
		}
		return "integer"
	default:
		return "integer"
	}
}

func lookupSchemaType(typ reflect.Type, path string) (reflect.Type, bool) {
	for _, segment := range strings.Split(path, ".") {
		name := strings.TrimSuffix(strings.TrimSuffix(segment, "[]"), "{}")
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct {
			return nil, false
		}
		field, ok := fieldByYAMLName(typ, name)
		if !ok {
			return nil, false
		}
		typ = field
	}
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	return typ, true
}

func fieldByYAMLName(typ reflect.Type, name string) (reflect.Type, bool) {
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
		if tag == name {
			return field.Type, true
		}
	}
	return nil, false
}

// referenceDefaults reports the value each key takes when it is omitted, read
// from the same defaults the loader applies. Listener entries are defaulted
// per element rather than in the document, so they are named explicitly.
func referenceDefaults() map[string]string {
	values := map[string]string{
		"listeners.mqtt[].max_connections":            fmt.Sprintf("`%d`", defaultMaxConnections),
		"listeners.mqtt[].read_timeout":               fmt.Sprintf("`%s`", defaultListenerTimeout),
		"listeners.mqtt[].write_timeout":              fmt.Sprintf("`%s`", defaultListenerTimeout),
		"listeners.mqtt[].path":                       fmt.Sprintf("`%s` for websocket", defaultWSPath),
		"listeners.amqp091[].max_connections":         fmt.Sprintf("`%d`", defaultMaxConnections),
		"listeners.amqp091[].handshake_timeout":       fmt.Sprintf("`%s`", defaultHandshakeTimeout),
		"listeners.amqp1[].max_connections":           fmt.Sprintf("`%d`", defaultMaxConnections),
		"listeners.amqp1[].handshake_timeout":         fmt.Sprintf("`%s`", defaultHandshakeTimeout),
		"cluster.ports.etcd_peer":                     "`2380`",
		"cluster.ports.transport":                     "`7948`",
		"version":                                     referenceRequired,
		"listeners.mqtt[].address":                    referenceRequired,
		"listeners.amqp091[].address":                 referenceRequired,
		"listeners.amqp091[].auth":                    referenceRequired,
		"listeners.amqp1[].address":                   referenceRequired,
		"storage.type":                                referenceRequired,
		"queues[].name":                               referenceRequired,
		"experimental.http.listeners[].address":       referenceRequired,
		"experimental.coap.listeners[].address":       referenceRequired,
		"auth.local_principals[].name":                referenceRequired,
		"auth.local_principals[].current_secret_file": referenceRequired,
	}

	encoded, err := yaml.Marshal(defaultDocument())
	if err != nil {
		return values
	}
	var decoded map[string]any
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		return values
	}
	flattenDefaults("", decoded, values)

	for _, path := range collectSchemaPaths(reflect.TypeFor[document](), "") {
		if _, ok := values[path]; !ok {
			values[path] = "—"
		}
	}
	return values
}

func flattenDefaults(prefix string, node map[string]any, out map[string]string) {
	for key, value := range node {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		switch typed := value.(type) {
		case map[string]any:
			flattenDefaults(path, typed, out)
		case nil:
			continue
		default:
			if _, taken := out[path]; taken {
				continue
			}
			rendered := fmt.Sprintf("%v", typed)
			if rendered == "" {
				continue
			}
			out[path] = "`" + rendered + "`"
		}
	}
}
