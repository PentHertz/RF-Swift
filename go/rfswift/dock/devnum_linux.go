//go:build linux

package dock

import "golang.org/x/sys/unix"

// deviceNumbers returns the major and minor numbers of a device node.
func deviceNumbers(path string) (major, minor uint32, ok bool) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFCHR {
		return 0, 0, false
	}
	return unix.Major(uint64(st.Rdev)), unix.Minor(uint64(st.Rdev)), true
}

func deviceNumbersSupported() bool { return true }
