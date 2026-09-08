/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

// Command genman writes the rfswift man pages that ship inside the Linux
// packages (see scripts/generate-packaging-assets.sh at the repo root).
//
// Usage: go run ./tools/genman <output-dir>
package main

import (
	"fmt"
	"os"

	cli "penthertz/rfswift/cli"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: genman <output-dir>")
		os.Exit(2)
	}
	if err := cli.GenManPages(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "genman:", err)
		os.Exit(1)
	}
}
