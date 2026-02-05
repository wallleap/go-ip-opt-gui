package domain

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func NormalizeDomain(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	s = strings.TrimSuffix(s, ".")
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", false
	}
	if !isDomainName(s) {
		return "", false
	}
	return s, true
}

func isDomainName(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	if strings.Contains(s, "..") {
		return false
	}
	labels := strings.Split(s, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			ch := label[i]
			switch {
			case ch >= 'a' && ch <= 'z':
			case ch >= '0' && ch <= '9':
			case ch == '-':
			default:
				return false
			}
		}
	}
	return true
}

func ParseDomains(text string) []string {
	var out []string
	seen := map[string]bool{}

	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		line = strings.ReplaceAll(line, ",", " ")
		line = strings.ReplaceAll(line, ";", " ")
		for _, token := range strings.Fields(line) {
			if d, ok := NormalizeDomain(token); ok && !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

func ReadDomainsFromFile(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseDomains(string(b)), nil
}

func ReadDomainsFromHosts(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := map[string]bool{}
	var out []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		for _, token := range fields[1:] {
			if d, ok := NormalizeDomain(token); ok && !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func EnsureReadableFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return "", errors.New("path is a directory")
	}
	return abs, nil
}
