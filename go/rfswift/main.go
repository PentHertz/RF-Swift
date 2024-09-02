/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package main

import (
	"fmt"
	"strings"

	cli "penthertz/rfswift/cli"
	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
)

func DisplayVersion() {
	owner := "PentHertz"
	repo := "RF-Swift"

	release, err := rfutils.GetLatestRelease(owner, repo)
	if err != nil {
		fmt.Printf("Error getting latest release: %v\n", err)
		return
	}

	currentVersion := common.Version
	latestVersion := release.TagName

	compareResult := rfutils.VersionCompare(currentVersion, latestVersion)
	if compareResult >= 0 {
		fmt.Printf("\033[35m[+]\033[0m \033[37mYou are running version: \033[33m%s\033[37m (Up to date)\033[0m\n", currentVersion)
	} else {
		fmt.Printf("\033[31m[!]\033[0m \033[37mYou are running version: \033[33m%s\033[37m (\033[31mObsolete\033[37m)\033[0m\n", currentVersion)
		fmt.Printf("\033[35m[+]\033[0m Do you want to update to the latest version? (yes/no): ")
		var updateResponse string
		fmt.Scanln(&updateResponse)

		if strings.ToLower(updateResponse) != "yes" {
			fmt.Println("Update aborted.")
			return
		}

		rfutils.GetLatestRFSwift()
	}
}

func main() {
	common.PrintASCII()
	DisplayVersion()
	cli.Execute()
}
