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
	// Skip printing ASCII art during autocompletion
	isCompletion := false
	
	// Check if being used for completion
	if len(os.Args) > 1 && os.Args[1] == "__complete" {
		isCompletion = true
	}
	
	// Skip for the completion command itself
	if os.Args[1] == "completion" {
		isCompletion = true
	}
	
	// Only print ASCII art when not in completion mode
	if !isCompletion {
		common.PrintASCII()
	}
	
	cli.Execute()
}