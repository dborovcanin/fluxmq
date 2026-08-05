// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyStaticManifest(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "node1", "etcd")
	if err := VerifyStaticManifest(dataDir, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("first VerifyStaticManifest() error = %v", err)
	}
	if err := VerifyStaticManifest(dataDir, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("same manifest was rejected: %v", err)
	}
	if err := VerifyStaticManifest(dataDir, strings.Repeat("b", 64)); err == nil || !strings.Contains(err.Error(), "membership differs") {
		t.Fatalf("changed membership error = %v", err)
	}
}

func TestVerifyStaticManifestRejectsUnmarkedData(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "node1", "etcd")
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "member"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyStaticManifest(dataDir, strings.Repeat("a", 64)); err == nil || !strings.Contains(err.Error(), "no v1 static-membership manifest") {
		t.Fatalf("unmarked existing data error = %v", err)
	}
}
