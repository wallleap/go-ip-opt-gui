//go:build windows

package filedialog

import (
	"errors"
	"syscall"
	"unsafe"
)

type Filter struct {
	Name    string
	Pattern string
}

func OpenFile(title string, filters []Filter) (string, error) {
	filterStr, err := buildFilter(filters)
	if err != nil {
		return "", err
	}

	buf := make([]uint16, 4096)

	var ofn openFileName
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	ofn.lpstrFile = &buf[0]
	ofn.nMaxFile = uint32(len(buf))
	if filterStr != nil {
		ofn.lpstrFilter = filterStr
	}
	ofn.Flags = ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir
	if title != "" {
		ofn.lpstrTitle = syscall.StringToUTF16Ptr(title)
	}

	ret, _, callErr := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		if callErr != syscall.Errno(0) {
			return "", callErr
		}
		return "", errors.New("canceled")
	}
	return syscall.UTF16ToString(buf), nil
}

func buildFilter(filters []Filter) (*uint16, error) {
	if len(filters) == 0 {
		return nil, nil
	}
	var u16 []uint16
	for _, f := range filters {
		if f.Name == "" || f.Pattern == "" {
			continue
		}
		u16 = append(u16, []uint16(syscall.StringToUTF16(f.Name))...)
		u16 = append(u16, 0)
		u16 = append(u16, []uint16(syscall.StringToUTF16(f.Pattern))...)
		u16 = append(u16, 0)
	}
	u16 = append(u16, 0)
	if len(u16) == 1 {
		return nil, nil
	}
	return &u16[0], nil
}

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	Flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        unsafe.Pointer
	dwReserved        uint32
	FlagsEx           uint32
}

const (
	ofnExplorer     = 0x00080000
	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnNoChangeDir   = 0x00000008
)

var (
	modComdlg32          = syscall.NewLazyDLL("comdlg32.dll")
	procGetOpenFileNameW = modComdlg32.NewProc("GetOpenFileNameW")
)

