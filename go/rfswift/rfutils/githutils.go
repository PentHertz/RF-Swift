package rfutils

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/cheggaaa/pb/v3"
	"github.com/go-resty/resty/v2"
)

type Release struct {
	TagName string `json:"tag_name"`
}

func getLatestRelease(owner string, repo string) (Release, error) {
	/*
	*	Get Latest Release information
	*	in(1): owner string
	*	in(2): repository string
	*	out: status
	*/ 
	client := resty.New()

	resp, err := client.R().
		SetHeader("Accept", "application/vnd.github.v3+json").
		Get(fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo))

	if err != nil {
		return Release{}, err
	}

	if resp.StatusCode() != http.StatusOK {
		return Release{}, fmt.Errorf("failed to get latest release: %s", resp.Status())
	}

	var release Release
	if err := json.Unmarshal(resp.Body(), &release); err != nil {
		return Release{}, err
	}

	return release, nil
}

func constructDownloadURL(owner, repo, tag, fileName string) string {
	/*
	*	Construct download URL link for RF Swift release
	*	in(1): owner string
	*	in(2): repository string
	*	in(3): tag string
	*	in(4): filename string
	*	out: status
	*/ 
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, tag, fileName)
}

func downloadFile(url, dest string) error {
	/*
	*	Download RF swift binary realse
	*	in(1): string url
	*	in(2): destinarion string
	*	out: status
	*/ 
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: %s", resp.Status)
	}

	size, err := strconv.Atoi(resp.Header.Get("Content-Length"))
	if err != nil {
		return err
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	bar := pb.Full.Start64(int64(size))
	barReader := bar.NewProxyReader(resp.Body)
	_, err = io.Copy(out, barReader)
	bar.Finish()

	return err
}

func makeExecutable(path string) error {
	/*
	*	Making downloaded RF Swift binary executable
	*	in(1): string path
	*	out: status
	*/ 
	err := os.Chmod(path, 0755)
	if err != nil {
		return err
	}
	return nil
}

func replaceBinary(newBinaryPath, binaryName string) error {
	/*
	*	Replace original RF Swift binary by the latest release
	*	in(1): latest binary string
	*	in(2): original binary string
	*	out: status
	*/ 
	// Ensure the new binary is executable
	err := makeExecutable(newBinaryPath)
	if err != nil {
		return err
	}

	// Determine the current binary path
	currentBinaryPath, err := exec.LookPath(binaryName)
	if err != nil {
		return err
	}

	// Replace the current binary with the new one
	err = os.Rename(newBinaryPath, currentBinaryPath)
	if err != nil {
		return err
	}

	return nil
}

func GetLatestRFSwift() {
	/*
	*	Print latest RF Swift binary from official Penthertz' repository
	*/
	owner := "PentHertz"
	repo := "RF-Swift"

	release, err := getLatestRelease(owner, repo)
	if err != nil {
		log.Fatalf("Error getting latest release: %v", err)
	}

	arch := runtime.GOARCH
	goos := runtime.GOOS

	var fileName string

	switch goos {
	case "linux":
		switch arch {
		case "amd64":
			fileName = "rfswift_linux_amd64"
		case "arm64":
			fileName = "rfswift_linux_arm64"
		default:
			log.Fatalf("Unsupported architecture: %s", arch)
		}
	case "windows":
		switch arch {
		case "amd64":
			fileName = "rfswift_windows_amd64.exe"
		case "arm64":
			fileName = "rfswift_windows_arm64.exe"
		default:
			log.Fatalf("Unsupported architecture: %s", arch)
		}
	default:
		log.Fatalf("Unsupported operating system: %s", goos)
	}

	downloadURL := constructDownloadURL(owner, repo, release.TagName, fileName)
	fmt.Printf("Latest release download URL: %s\n", downloadURL)

	fmt.Printf("Do you want to replace the existing 'rfswift' binary with this new release? (yes/no): ")
	var response string
	fmt.Scanln(&response)

	if response == "yes" {
		tempDest := filepath.Join(os.TempDir(), fileName)
		err = downloadFile(downloadURL, tempDest)
		if err != nil {
			log.Fatalf("Error downloading file: %v", err)
		}

		err = replaceBinary(tempDest, "./rfswift")
		if err != nil {
			log.Fatalf("Error replacing binary: %v", err)
		}

		fmt.Println("File downloaded and replaced successfully.")
	} else {
		// Get the current binary directory
		currentBinaryPath, err := exec.LookPath("./rfswift")
		if err != nil {
			log.Fatalf("Error locating current binary: %v", err)
		}

		var dest string
		ext := filepath.Ext(fileName)
		name := strings.TrimSuffix(fileName, ext)
		dest = filepath.Join(filepath.Dir(currentBinaryPath), fmt.Sprintf("%s_%s%s", name, release.TagName, ext))

		err = downloadFile(downloadURL, dest)
		if err != nil {
			log.Fatalf("Error downloading file: %v", err)
		}

		// Make the new binary executable
		err = makeExecutable(dest)
		if err != nil {
			log.Fatalf("Error making binary executable: %v", err)
		}

		fmt.Printf("File downloaded and saved at %s\n", dest)
	}
}