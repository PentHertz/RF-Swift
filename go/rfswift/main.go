/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package main

import (
	"os"
	cli "penthertz/rfswift/cli"
	common "penthertz/rfswift/common"
)

func main() {
	isCompletion := false

	if len(os.Args) > 1 {
		if (os.Args[1] == "completion") || (os.Args[1] == "__complete") {
			isCompletion = true

		}
	}

	if isCompletion == false {
		common.PrintASCII()
	}

	cli.Execute()
}
