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

func promptUserForUpdate() bool {
	fmt.Printf("\033[35m[+]\033[0m Do you want to update to the latest version? (yes/no): ")
	var response string
	fmt.Scanln(&response)
	return strings.ToLower(response) == "yes"
}

func DisplayVersion() {
	owner := "PentHertz"
	repo := "RF-Swift"

	release, err := rfutils.GetLatestRelease(owner, repo)
	if err != nil {
		rfutils.DisplayNotification(
			"Error",
			fmt.Sprintf("Unable to fetch the latest release.\nDetails: %v", err),
			"error",
		)
		return
	}

	currentVersion := common.Version
	latestVersion := release.TagName

	compareResult := rfutils.VersionCompare(currentVersion, latestVersion)
	if compareResult >= 0 {
		rfutils.DisplayNotification(
			" Up-to-date",
			fmt.Sprintf("You are running the latest version: %s", currentVersion),
			"info",
		)
		return
	}

	common.PrintWarningMessage(fmt.Sprintf("Current version: %s\nLatest version: %s", currentVersion, latestVersion))

	rfutils.GetLatestRFSwift()
}

func main() {
	common.PrintASCII()
	DisplayVersion()
	cli.Execute()
}
