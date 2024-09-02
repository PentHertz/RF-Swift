package rfutils

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/go-resty/resty/v2"

	common "penthertz/rfswift/common"
)

type Release struct {
	TagName string `json:"tag_name"`
}

func GetLatestRelease(owner string, repo string) (Release, error) {
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

func ConstructDownloadURL(owner, repo, tag, fileName string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, tag, fileName)
}

func DownloadFile(url, dest string) error {
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
	bar.Set(pb.Bytes, true)
	go func() {
		colors := []string{"\033[31m", "\033[32m", "\033[33m", "\033[34m", "\033[35m", "\033[36m"}
		for i := 0; bar.IsStarted(); i++ {
			bar.SetTemplateString(fmt.Sprintf("%s{{counters . }} {{bar . }} {{percent . }}%%", colors[i%len(colors)]))
			time.Sleep(100 * time.Millisecond)
		}
	}()

	barReader := bar.NewProxyReader(resp.Body)
	_, err = io.Copy(out, barReader)
	bar.Finish()

	return err
}

func MakeExecutable(path string) error {
	err := os.Chmod(path, 0755)
	if err != nil {
		return err
	}
	return nil
}

func ReplaceBinary(newBinaryPath, currentBinaryPath string) error {
	err := MakeExecutable(newBinaryPath)
	if err != nil {
		return err
	}

	err = os.Rename(newBinaryPath, currentBinaryPath)
	if err != nil {
		return err
	}

	return nil
}

func VersionCompare(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		p1, _ := strconv.Atoi(parts1[i])
		p2, _ := strconv.Atoi(parts2[i])

		if p1 > p2 {
			return 1
		}
		if p1 < p2 {
			return -1
		}
	}

	if len(parts1) > len(parts2) {
		return 1
	}
	if len(parts1) < len(parts2) {
		return -1
	}

	return 0
}

func GetLatestRFSwift() {
	owner := "PentHertz"
	repo := "RF-Swift"

	release, err := GetLatestRelease(owner, repo)
	if err != nil {
		log.Fatalf("Error getting latest release: %v", err)
	}

	compareResult := VersionCompare(common.Version, release.TagName)
	if compareResult >= 0 {
		fmt.Printf("\033[35m[+]\033[0m \033[37mYou already have the latest version: \033[33m%s\033[0m\n", common.Version)
		return
	} else if compareResult < 0 {
		fmt.Printf("\033[31m[!]\033[0m \033[37mYour current version (\033[33m%s\033[37m) is obsolete. Please update to version (\033[33m%s\033[37m).\n", common.Version, release.TagName)
	}

	fmt.Printf("\033[35m[+]\033[0m Do you want to update to the latest version? (yes/no): ")
	var updateResponse string
	fmt.Scanln(&updateResponse)

	if strings.ToLower(updateResponse) != "yes" {
		fmt.Println("Update aborted.")
		return
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

	downloadURL := ConstructDownloadURL(owner, repo, release.TagName, fileName)
	fmt.Printf("Latest release download URL: %s\n", downloadURL)

	fmt.Printf("\033[35m[+]\033[0m Do you want to replace the existing binary with this new release? (yes/no): ")
	var response string
	fmt.Scanln(&response)

	currentBinaryPath, err := os.Executable()
	if err != nil {
		log.Fatalf("Error determining the current executable path: %v", err)
	}

	if response == "yes" {
		tempDest := filepath.Join(os.TempDir(), fileName)
		err = DownloadFile(downloadURL, tempDest)
		if err != nil {
			log.Fatalf("Error downloading file: %v", err)
		}

		err = ReplaceBinary(tempDest, currentBinaryPath)
		if err != nil {
			log.Fatalf("Error replacing binary: %v", err)
		}

		fmt.Println("File downloaded and replaced successfully.")
	} else {
		var dest string
		ext := filepath.Ext(fileName)
		dest = filepath.Join(filepath.Dir(currentBinaryPath), fmt.Sprintf("%s_%s%s", strings.TrimSuffix(fileName, ext), release.TagName, ext))

		err = DownloadFile(downloadURL, dest)
		if err != nil {
			log.Fatalf("Error downloading file: %v", err)
		}

		err = MakeExecutable(dest)
		if err != nil {
			log.Fatalf("Error making binary executable: %v", err)
		}

		fmt.Printf("File downloaded and saved at %s\n", dest)
	}
}
