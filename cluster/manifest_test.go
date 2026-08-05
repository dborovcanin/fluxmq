// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyAndRecordStaticManifest(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "node1", "etcd")
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)

	if err := VerifyStaticManifest(dataDir, first); err != nil {
		t.Fatalf("first VerifyStaticManifest() error = %v", err)
	}
	if err := RecordStaticManifest(dataDir, first); err != nil {
		t.Fatalf("RecordStaticManifest() error = %v", err)
	}
	if err := VerifyStaticManifest(dataDir, first); err != nil {
		t.Fatalf("same manifest was rejected: %v", err)
	}
	if err := RecordStaticManifest(dataDir, first); err != nil {
		t.Fatalf("re-recording the same manifest failed: %v", err)
	}
	if err := VerifyStaticManifest(dataDir, second); err == nil || !strings.Contains(err.Error(), "membership differs") {
		t.Fatalf("changed membership error = %v", err)
	}
	if err := RecordStaticManifest(dataDir, second); err == nil || !strings.Contains(err.Error(), "membership differs") {
		t.Fatalf("recording a changed membership error = %v", err)
	}
}

// Verification must not write. A node whose etcd fails to start leaves no pin
// behind, so a corrected member map still starts on the same directory.
func TestVerifyStaticManifestDoesNotRecord(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "node1", "etcd")

	if err := VerifyStaticManifest(dataDir, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("VerifyStaticManifest() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dataDir), staticManifestFile)); !os.IsNotExist(err) {
		t.Fatalf("verification wrote a manifest: %v", err)
	}
	if err := VerifyStaticManifest(dataDir, strings.Repeat("b", 64)); err != nil {
		t.Fatalf("a corrected member map was refused after a failed start: %v", err)
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
