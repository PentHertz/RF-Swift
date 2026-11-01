//go:build linux

package hostsetup

import "syscall"

// diskFreeBytes returns the space available to unprivileged users on the
// filesystem holding path.
func diskFreeBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
