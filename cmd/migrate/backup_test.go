package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestUnrestoredBackupGuard(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte("CREATE TABLE example (id INT);"))
	w.Close()
	path := filepath.Join(t.TempDir(), "backup.sql.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateBackup("laundry-inputs", "test", "test", false, path); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"baseline", "mobile-releases", "status"} {
		if validateBackup(command, "test", "test", false, path) == nil {
			t.Fatalf("allowed override for %s", command)
		}
	}
	if validateBackup("laundry-inputs", "production", "test", false, path) == nil {
		t.Fatal("allowed wrong target")
	}
	if validateBackup("laundry-inputs", "test", "test", true, path) == nil {
		t.Fatal("allowed contradictory flags")
	}
	if validateBackup("laundry-inputs", "test", "test", false, "") == nil {
		t.Fatal("allowed absent backup")
	}
	broken := append([]byte(nil), buf.Bytes()...)
	broken[len(broken)-5] ^= 0xff
	os.WriteFile(path, broken, 0600)
	if validateBackup("laundry-inputs", "test", "test", false, path) == nil {
		t.Fatal("allowed corrupt backup")
	}
}
