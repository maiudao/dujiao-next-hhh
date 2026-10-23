package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIdentityMigrationRequiresFreshVerifiedDistinctBackups(t *testing.T) {
	now := time.Now().UTC()
	root := t.TempDir()
	artifact := func(name string) identityBackupArtifact {
		t.Helper()
		path := filepath.Join(root, name)
		data := []byte("isolated backup fixture " + name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		return identityBackupArtifact{Path: path, SHA256: hex.EncodeToString(hash[:]), RestoreVerified: true}
	}
	valid := identityBackupManifest{Batch: "fixture", CreatedAt: now.Add(-time.Minute), IdentityLinksEmpty: true, Postgres: artifact("main.dump"), Shop: artifact("shop.db")}
	for _, test := range []struct {
		name   string
		mutate func(*identityBackupManifest)
		valid  bool
	}{
		{"verified", func(*identityBackupManifest) {}, true},
		{"stale", func(m *identityBackupManifest) { m.CreatedAt = now.Add(-2 * time.Hour) }, false},
		{"future", func(m *identityBackupManifest) { m.CreatedAt = now.Add(time.Minute) }, false},
		{"no-main-restore", func(m *identityBackupManifest) { m.Postgres.RestoreVerified = false }, false},
		{"no-shop-restore", func(m *identityBackupManifest) { m.Shop.RestoreVerified = false }, false},
		{"changed-backup", func(m *identityBackupManifest) { m.Shop.SHA256 = m.Postgres.SHA256 }, false},
		{"same-backup", func(m *identityBackupManifest) { m.Shop = m.Postgres }, false},
		{"existing-links", func(m *identityBackupManifest) { m.IdentityLinksEmpty = false }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := valid
			test.mutate(&m)
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "manifest.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := verifyIdentityBackup(path, now); (err == nil) != test.valid {
				t.Fatalf("unexpected verification: %v", err)
			}
		})
	}
	if runIdentityCommand([]string{"migrate"}) != 2 {
		t.Fatal("migration allowed without explicit gate")
	}
}
