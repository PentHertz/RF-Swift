//go:build !linux

package hostsetup

import "errors"

// diskFreeBytes: the Qubes OS checks only run on Linux.
func diskFreeBytes(string) (uint64, error) { return 0, errors.New("not supported on this platform") }
