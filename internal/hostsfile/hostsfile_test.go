package hostsfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyManagedBlock(t *testing.T) {
	orig := "127.0.0.1 localhost\n" + beginMarker + "\n1.1.1.1 a.com\n" + endMarker + "\n"
	block := BuildManagedBlock([]Mapping{{IP: "2.2.2.2", Domain: "b.com"}})
	next := ApplyManagedBlock(orig, block)
	if strings.Count(next, beginMarker) != 1 || strings.Count(next, endMarker) != 1 {
		t.Fatalf("managed block marker count mismatch:\n%s", next)
	}
	if !strings.Contains(next, "2.2.2.2 b.com") {
		t.Fatalf("new mapping not found:\n%s", next)
	}
	if strings.Contains(next, "1.1.1.1 a.com") {
		t.Fatalf("old mapping still present:\n%s", next)
	}
}

func TestWriteWithBackupAndRestore(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0644); err != nil {
		t.Fatal(err)
	}

	backup, newContent, err := WriteWithBackup(hostsPath, []Mapping{{IP: "1.2.3.4", Domain: "example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(backup) == "" {
		t.Fatalf("empty backup path")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	b, _ := os.ReadFile(hostsPath)
	if string(b) != newContent {
		t.Fatalf("written content mismatch")
	}

	if err := RestoreBackup(backup, hostsPath); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(hostsPath)
	if string(restored) != "127.0.0.1 localhost\n" {
		t.Fatalf("restore mismatch: %q", string(restored))
	}
}

