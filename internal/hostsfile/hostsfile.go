package hostsfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	beginMarker = "# ip-opt-gui begin"
	endMarker   = "# ip-opt-gui end"
)

type Mapping struct {
	IP     string
	Domain string
}

func DefaultHostsPath() string {
	switch runtime.GOOS {
	case "windows":
		winDir := os.Getenv("WINDIR")
		if winDir == "" {
			winDir = `C:\Windows`
		}
		return filepath.Join(winDir, "System32", "drivers", "etc", "hosts")
	default:
		return "/etc/hosts"
	}
}

func Read(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func BuildManagedBlock(mappings []Mapping) string {
	var b strings.Builder
	b.WriteString(beginMarker)
	b.WriteString("\n")
	for _, m := range mappings {
		ip := strings.TrimSpace(m.IP)
		d := strings.TrimSpace(m.Domain)
		if ip == "" || d == "" {
			continue
		}
		b.WriteString(ip)
		b.WriteString(" ")
		b.WriteString(d)
		b.WriteString("\n")
	}
	b.WriteString(endMarker)
	b.WriteString("\n")
	return b.String()
}

func ApplyManagedBlock(existing string, block string) string {
	existing = normalizeNewlines(existing)
	lines := strings.Split(existing, "\n")

	var out []string
	inManaged := false
	for _, line := range lines {
		lineTrim := strings.TrimSpace(line)
		if !inManaged && lineTrim == beginMarker {
			inManaged = true
			continue
		}
		if inManaged {
			if lineTrim == endMarker {
				inManaged = false
			}
			continue
		}
		out = append(out, line)
	}

	next := strings.TrimRight(strings.Join(out, "\n"), "\n")
	if next != "" {
		next += "\n"
	}
	next += normalizeNewlines(block)
	return next
}

func WriteWithBackup(path string, mappings []Mapping) (backupPath string, newContent string, err error) {
	orig, err := Read(path)
	if err != nil {
		return "", "", err
	}
	block := BuildManagedBlock(mappings)
	newContent = ApplyManagedBlock(orig, block)

	backupPath, err = backupFile(path, orig)
	if err != nil {
		return "", "", err
	}

	mode := os.FileMode(0644)
	if st, statErr := os.Stat(path); statErr == nil {
		mode = st.Mode()
	}
	if err := os.WriteFile(path, []byte(newContent), mode); err != nil {
		return "", "", err
	}
	return backupPath, newContent, nil
}

func RestoreBackup(backupPath, hostsPath string) error {
	if strings.TrimSpace(backupPath) == "" {
		return errors.New("empty backup path")
	}
	b, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	mode := os.FileMode(0644)
	if st, statErr := os.Stat(hostsPath); statErr == nil {
		mode = st.Mode()
	}
	return os.WriteFile(hostsPath, b, mode)
}

func backupFile(path string, content string) (string, error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ts := time.Now().Format("20060102_150405")
	backup := filepath.Join(dir, fmt.Sprintf("%s.bak.%s", base, ts))
	if err := os.WriteFile(backup, []byte(content), 0644); err != nil {
		return "", err
	}
	return backup, nil
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

