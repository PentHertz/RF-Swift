/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */

package dock

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/moby/moby/client"

	common "penthertz/rfswift/common"
)

// saveUpgradeArchive spools a container archive without interpreting its paths
// or losing tar metadata. A partial copy is never returned as a usable backup.
func saveUpgradeArchive(reader io.Reader, dir string) (string, error) {
	f, err := os.CreateTemp(dir, "preserved-*.tar")
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(f, reader)
	err = errors.Join(copyErr, f.Close())
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// ExportContainer exports a container's filesystem to a compressed tar.gz file.
//
//	in(1): string containerID ID or name of the container to export
//	in(2): string outputFile path to the output .tar.gz file to create
//	out: error non-nil if the export or compression fails
func ExportContainer(containerID string, outputFile string) error {
	return ExportContainerWithProgress(containerID, outputFile, nil)
}

type ContainerExportProgress func(percent int, stage string, bytes int64)

type exportProgressReader struct {
	r        io.Reader
	written  int64
	progress ContainerExportProgress
}

func (r *exportProgressReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.written += int64(n)
	if n > 0 && r.progress != nil {
		r.progress(55, "Streaming and compressing container filesystem", r.written)
	}
	return n, err
}

// ExportContainerWithProgress reports real bytes streamed. The daemon does not
// provide the final filesystem tar size in advance, so the byte-processing
// phase remains indeterminate until the stream closes successfully.
func ExportContainerWithProgress(containerID string, outputFile string, progress ContainerExportProgress) error {
	report := func(percent int, stage string, bytes int64) {
		if progress != nil {
			progress(percent, stage, bytes)
		}
	}
	report(5, "Connecting to container engine", 0)
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Get container info
	containerJSON, err := inspectContainer(ctx, cli, containerID)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %v", err)
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")
	report(15, "Container inspected", 0)

	common.PrintInfoMessage(fmt.Sprintf("Exporting container '%s' to %s", containerName, outputFile))

	// Export container
	reader, err := cli.ContainerExport(ctx, containerID, client.ContainerExportOptions{})
	if err != nil {
		return fmt.Errorf("failed to export container: %v", err)
	}
	defer reader.Close()
	report(25, "Container export stream opened", 0)

	// Create output file
	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	// Copy with progress
	common.PrintInfoMessage("Compressing container data...")
	tracked := &exportProgressReader{r: reader, progress: progress}
	written, err := io.Copy(gzipWriter, tracked)
	if err != nil {
		return fmt.Errorf("failed to write compressed data: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to finish compressed data: %v", err)
	}
	report(100, "Container export complete", tracked.written)

	common.PrintSuccessMessage(fmt.Sprintf("Container exported successfully: %s (%.2f MB)",
		outputFile, float64(written)/(1024*1024)))
	return nil
}

// ExportImage exports one or more images to a compressed tar.gz file.
//
//	in(1): []string images list of image names or IDs to export
//	in(2): string outputFile path to the output .tar.gz file to create
//	out: error non-nil if saving or compressing the images fails
func ExportImage(images []string, outputFile string) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Normalize all image names
	for i, img := range images {
		images[i] = normalizeImageName(img)
	}

	common.PrintInfoMessage(fmt.Sprintf("Exporting %d image(s) to %s", len(images), outputFile))
	for _, img := range images {
		common.PrintInfoMessage(fmt.Sprintf("  - %s", img))
	}

	// Save images
	reader, err := cli.ImageSave(ctx, images)
	if err != nil {
		return fmt.Errorf("failed to save images: %v", err)
	}
	defer reader.Close()

	// Create output file
	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	// Copy with progress
	common.PrintInfoMessage("Compressing image data...")
	written, err := io.Copy(gzipWriter, reader)
	if err != nil {
		return fmt.Errorf("failed to write compressed data: %v", err)
	}

	common.PrintSuccessMessage(fmt.Sprintf("Image(s) exported successfully: %s (%.2f MB)",
		outputFile, float64(written)/(1024*1024)))
	return nil
}

// ImportContainer imports a container from a tar or tar.gz file and creates an image.
//
//	in(1): string inputFile path to the .tar or .tar.gz file to import
//	in(2): string imageName tag to assign to the resulting image
//	out: error non-nil if opening, decompressing, or importing the file fails
func ImportContainer(inputFile string, imageName string) error {
	return ImportContainerWithProgress(inputFile, imageName, nil)
}

type ContainerImportProgress func(percent int, stage string, bytes int64, total int64)

func ImportContainerWithProgress(inputFile string, imageName string, progress ContainerImportProgress) error {
	report := func(percent int, stage string, bytes, total int64) {
		if progress != nil {
			progress(percent, stage, bytes, total)
		}
	}
	report(3, "Connecting to container engine", 0, 0)
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	common.PrintInfoMessage(fmt.Sprintf("Importing container from %s as image '%s'", inputFile, imageName))

	// Open input file
	inFile, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("failed to open input file: %v", err)
	}
	defer inFile.Close()
	var total int64
	if info, statErr := inFile.Stat(); statErr == nil {
		total = info.Size()
	}
	tracked := &exportProgressReader{r: inFile}
	tracked.progress = func(_ int, _ string, bytes int64) {
		percent := 25
		if total > 0 {
			percent = 15 + int(float64(bytes)/float64(total)*65)
			if percent > 80 {
				percent = 80
			}
		}
		report(percent, "Reading container archive", bytes, total)
	}
	report(10, "Opening container archive", 0, total)

	// Check if file is gzipped
	var reader io.Reader
	gzipReader, err := gzip.NewReader(tracked)
	if err == nil {
		// File is gzipped
		common.PrintInfoMessage("Decompressing tar.gz file...")
		reader = gzipReader
		defer gzipReader.Close()
	} else {
		// File is plain tar
		common.PrintInfoMessage("Reading tar file...")
		tracked.written = 0
		inFile.Seek(0, 0) // Reset file pointer
		reader = tracked
	}

	// Import container with label
	importResponse, err := cli.ImageImport(ctx, client.ImageImportSource{
		Source:     reader,
		SourceName: "-",
	}, imageName, client.ImageImportOptions{
		// Add RF Swift label
		Changes: []string{
			`LABEL "org.container.project"="rfswift"`,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to import container: %v", err)
	}
	defer importResponse.Close()
	report(85, "Registering imported container image", tracked.written, total)

	// Docker and compatible daemons stream errors as JSON in an otherwise
	// successful HTTP response. Do not claim success unless every status record
	// is clean and the requested local tag can be inspected afterwards.
	scanner := bufio.NewScanner(importResponse)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var status struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &status); err == nil {
			message := strings.TrimSpace(status.ErrorDetail.Message)
			if message == "" {
				message = strings.TrimSpace(status.Error)
			}
			if message != "" {
				return fmt.Errorf("container image import failed: %s", message)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed reading container image import response: %w", err)
	}
	if _, err := ImageInspectCompat(ctx, cli, imageName); err != nil {
		return fmt.Errorf("container archive was read but local image %q was not created: %w", imageName, err)
	}
	report(100, "Container archive imported", tracked.written, total)

	common.PrintSuccessMessage(fmt.Sprintf("Container imported successfully as image: %s", imageName))
	return nil
}

// ImportImage imports one or more images from a tar or tar.gz file.
//
//	in(1): string inputFile path to the .tar or .tar.gz file to load
//	out: error non-nil if opening, decompressing, or loading the file fails
func ImportImage(inputFile string) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	common.PrintInfoMessage(fmt.Sprintf("Importing image(s) from %s", inputFile))

	// Open input file
	inFile, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("failed to open input file: %v", err)
	}
	defer inFile.Close()

	// Check if file is gzipped
	var reader io.Reader
	gzipReader, err := gzip.NewReader(inFile)
	if err == nil {
		// File is gzipped
		common.PrintInfoMessage("Decompressing tar.gz file...")
		reader = gzipReader
		defer gzipReader.Close()
	} else {
		// File is plain tar
		common.PrintInfoMessage("Reading tar file...")
		inFile.Seek(0, 0) // Reset file pointer
		reader = inFile
	}

	// Load images - no third parameter needed
	loadResponse, err := cli.ImageLoad(ctx, reader)
	if err != nil {
		return fmt.Errorf("failed to load images: %v", err)
	}
	defer loadResponse.Close()

	// Parse response to show loaded images
	scanner := bufio.NewScanner(loadResponse)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Loaded image") || strings.Contains(line, "sha256") {
			common.PrintInfoMessage(line)
		}
	}

	common.PrintSuccessMessage("Image(s) imported successfully")
	return nil
}
