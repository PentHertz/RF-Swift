/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package main

import (
	"os"

	"golang.org/x/term"
	cli "penthertz/rfswift/cli"
	common "penthertz/rfswift/common"
)

// main is the program entry point. It prints the ASCII banner on interactive
// terminals only - not for shell-completion generation, not when stdout is a
// pipe (scripts, --json consumers), and not when RFSWIFT_NO_BANNER is set (the
// Windows rfswift driving this Linux one inside WSL already showed its own) -
// then delegates all command handling to the CLI layer via cli.Execute.
func main() {
	isCompletion := false

	if len(os.Args) > 1 {
		if (os.Args[1] == "completion") || (os.Args[1] == "__complete") {
			isCompletion = true

		}
	}

	if !isCompletion && os.Getenv("RFSWIFT_NO_BANNER") == "" && term.IsTerminal(int(os.Stdout.Fd())) {
		common.PrintASCII()
	}

	cli.Execute()
}
