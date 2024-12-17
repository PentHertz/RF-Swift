package rfutils

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
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
	if runtime.GOOS == "windows" {
		// No action needed on Windows
		return nil
	}

	// Set executable bit on Unix-like systems
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

func ExtractTarGz(src, destDir string) error {
	file, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error opening tar.gz file: %v", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("error creating gzip reader: %v", err)
	}
	defer gzr.Close()

	tarReader := tar.NewReader(gzr)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar header: %v", err)
		}

		targetPath := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("error creating directory: %v", err)
			}
		case tar.TypeReg:
			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("error creating file: %v", err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return fmt.Errorf("error writing file: %v", err)
			}
			outFile.Close()
		}
	}
	return nil
}

func ExtractZip(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("error opening zip file: %v", err)
	}
	defer r.Close()

	for _, file := range r.File {
		targetPath := filepath.Join(destDir, file.Name)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, file.Mode()); err != nil {
				return fmt.Errorf("error creating directory: %v", err)
			}
			continue
		}

		fileReader, err := file.Open()
		if err != nil {
			return fmt.Errorf("error opening file in zip: %v", err)
		}
		defer fileReader.Close()

		outFile, err := os.Create(targetPath)
		if err != nil {
			return fmt.Errorf("error creating file: %v", err)
		}
		if _, err := io.Copy(outFile, fileReader); err != nil {
			return fmt.Errorf("error writing file: %v", err)
		}
		outFile.Close()
	}
	return nil
}

func GetLatestRFSwift() {
	owner := "PentHertz"
	repo := "RF-Swift"

	release, err := GetLatestRelease(owner, repo)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	compareResult := VersionCompare(common.Version, release.TagName)
	if compareResult >= 0 {
		common.PrintSuccessMessage(fmt.Sprintf("You already have the latest version: %s", common.Version))
		return
	} else {
		common.PrintWarningMessage(fmt.Sprintf("Your current version is obsolete. Please update to version: %s", release.TagName))
	}

	common.PrintInfoMessage("Do you want to update to the latest version? (yes/no): ")
	var updateResponse string
	fmt.Scanln(&updateResponse)

	if strings.ToLower(updateResponse) != "yes" {
		common.PrintInfoMessage("Update aborted.")
		return
	}

	arch := runtime.GOARCH
	goos := runtime.GOOS

	var fileName string

	switch goos {
	case "linux":
		switch arch {
		case "amd64":
			fileName = "rfswift_Linux_x86_64.tar.gz"
		case "arm64":
			fileName = "rfswift_Linux_arm64.tar.gz"
		default:
			common.PrintErrorMessage(fmt.Errorf("Unsupported architecture: %s", arch))
			return
		}
	case "darwin":
		switch arch {
		case "amd64":
			fileName = "rfswift_Darwin_x86_64.tar.gz"
		case "arm64":
			fileName = "rfswift_Darwin_arm64.tar.gz"
		default:
			common.PrintErrorMessage(fmt.Errorf("Unsupported architecture: %s", arch))
			return
		}
	case "windows":
		switch arch {
		case "amd64":
			fileName = "rfswift_Windows_x86_64.zip"
		case "arm64":
			fileName = "rfswift_Windows_arm64.zip"
		default:
			common.PrintErrorMessage(fmt.Errorf("Unsupported architecture: %s", arch))
			return
		}
	default:
		common.PrintErrorMessage(fmt.Errorf("Unsupported operating system: %s", goos))
		return
	}

	downloadURL := ConstructDownloadURL(owner, repo, release.TagName, fileName)
	common.PrintInfoMessage(fmt.Sprintf("Latest release download URL: %s", downloadURL))

	currentBinaryPath, err := os.Executable()
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error determining the current executable path: %v", err))
		return
	}

	tempDest := filepath.Join(os.TempDir(), fileName)
	err = DownloadFile(downloadURL, tempDest)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error downloading file: %v", err))
		return
	}

	extractDir := filepath.Join(os.TempDir(), "rfswift_extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error creating extraction directory: %v", err))
		return
	}

	// Extract the file based on extension
	if strings.HasSuffix(fileName, ".tar.gz") {
		err = ExtractTarGz(tempDest, extractDir)
	} else if strings.HasSuffix(fileName, ".zip") {
		err = ExtractZip(tempDest, extractDir)
	} else {
		common.PrintErrorMessage(fmt.Errorf("Unsupported file format: %s", fileName))
		return
	}
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error extracting file: %v", err))
		return
	}

	newBinaryPath := filepath.Join(extractDir, "rfswift") // Adjust if the binary name differs
	err = ReplaceBinary(newBinaryPath, currentBinaryPath)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error replacing binary: %v", err))
		return
	}

	common.PrintSuccessMessage("File downloaded, extracted, and replaced successfully.")
}
