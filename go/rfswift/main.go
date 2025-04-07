/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package main

import (
	cli "penthertz/rfswift/cli"
	common "penthertz/rfswift/common"
)

func main() {
	common.PrintASCII()
	cli.Execute()
}
