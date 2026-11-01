//go:build windows

package hostsetup

import "io/fs"

// fileUID: Windows files have no uid.
func fileUID(fs.FileInfo) (uint32, bool) { return 0, false }
