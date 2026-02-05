package domain

import "testing"

func TestParseDomains(t *testing.T) {
	in := `
# comment
Example.com
foo.example.com, bar.example.com
invalid_domain
ok.example.com # tail
`
	ds := ParseDomains(in)
	want := map[string]bool{
		"example.com":     true,
		"foo.example.com": true,
		"bar.example.com": true,
		"ok.example.com":  true,
	}
	if len(ds) != len(want) {
		t.Fatalf("got %d domains: %#v", len(ds), ds)
	}
	for _, d := range ds {
		if !want[d] {
			t.Fatalf("unexpected domain: %s", d)
		}
	}
}

