//go:build !windows

package hostsetup

import (
	"io/fs"
	"syscall"
)

// fileUID returns the uid owning a file, when the platform reports one.
func fileUID(info fs.FileInfo) (uint32, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Uid, true
}
