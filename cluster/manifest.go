// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const staticManifestFile = "static-members.sha256"

// VerifyStaticManifest checks a v1 cluster data directory against the member
// map that formed it. Static membership changes require a fresh data
// directory. It only reads: the fingerprint is recorded by RecordStaticManifest
// once the node has actually formed, so a failed bootstrap does not pin an
// empty directory to a membership that never existed.
func VerifyStaticManifest(dataDir, fingerprint string) error {
	if strings.TrimSpace(dataDir) == "" || strings.TrimSpace(fingerprint) == "" {
		return errors.New("cluster data directory and manifest fingerprint are required")
	}
	stored, err := os.ReadFile(manifestPath(dataDir))
	if err == nil {
		if strings.TrimSpace(string(stored)) != fingerprint {
			return fmt.Errorf("static cluster membership differs from existing data in %s; restore the original member map or use a fresh storage directory", filepath.Dir(dataDir))
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("read static cluster manifest: %w", err)
	}

	entries, readErr := os.ReadDir(dataDir)
	if readErr == nil && len(entries) > 0 {
		return fmt.Errorf("existing cluster data in %s has no v1 static-membership manifest; use a fresh storage directory", dataDir)
	}
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("inspect cluster data directory: %w", readErr)
	}
	return nil
}

// RecordStaticManifest pins a data directory to its member map. Call it only
// after the node has joined its cluster, so that the pin describes a membership
// the node really formed.
func RecordStaticManifest(dataDir, fingerprint string) error {
	if strings.TrimSpace(dataDir) == "" || strings.TrimSpace(fingerprint) == "" {
		return errors.New("cluster data directory and manifest fingerprint are required")
	}
	marker := manifestPath(dataDir)
	if stored, err := os.ReadFile(marker); err == nil {
		if strings.TrimSpace(string(stored)) != fingerprint {
			return fmt.Errorf("static cluster membership differs from existing data in %s", filepath.Dir(dataDir))
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read static cluster manifest: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dataDir), 0o750); err != nil {
		return fmt.Errorf("create cluster data directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(dataDir), ".static-members-*")
	if err != nil {
		return fmt.Errorf("create static cluster manifest: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName) //nolint:errcheck // best-effort cleanup after atomic rename
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure static cluster manifest: %w", err)
	}
	if _, err := temporary.WriteString(fingerprint + "\n"); err != nil {
		temporary.Close()
		return fmt.Errorf("write static cluster manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync static cluster manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close static cluster manifest: %w", err)
	}
	if err := os.Rename(temporaryName, marker); err != nil {
		return fmt.Errorf("install static cluster manifest: %w", err)
	}
	return nil
}

func manifestPath(dataDir string) string {
	return filepath.Join(filepath.Dir(dataDir), staticManifestFile)
}
