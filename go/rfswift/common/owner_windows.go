//go:build windows

/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package common

import "io/fs"

// fileOwner reports no owner on Windows: the configuration lives under
// %APPDATA%, which a sudo run cannot take over.
func fileOwner(fs.FileInfo) (int, bool) { return 0, false }
