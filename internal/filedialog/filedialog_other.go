//go:build !windows

package filedialog

import "errors"

type Filter struct {
	Name    string
	Pattern string
}

func OpenFile(title string, filters []Filter) (string, error) {
	return "", errors.New("file dialog not supported on this platform")
}

