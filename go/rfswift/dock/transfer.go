/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */

package dock

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/image"

	common "penthertz/rfswift/common"
)

// extractTarArchive extracts a tar archive from a reader into the destination directory.
//
//	in(1): io.Reader reader source tar stream to extract from
//	in(2): string destDir filesystem path where archive contents are written
//	out: error non-nil if extraction fails at any step
func extractTarArchive(reader io.Reader, destDir string) error {
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// createTarArchive creates a tar archive from a local source directory, preserving the container path structure.
//
//	in(1): string srcDir local directory whose contents are packed into the archive
//	in(2): string containerPath destination path inside the container, used as the archive root name
//	out: io.ReadCloser pipe reader that streams the tar data (caller must close)
//	out: error non-nil if the archive cannot be started
func createTarArchive(srcDir string, containerPath string) (io.ReadCloser, error) {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		tarWriter := tar.NewWriter(pw)
		defer tarWriter.Close()

		// Get the base name of the container path
		baseName := filepath.Base(containerPath)

		// First, check what's actually in srcDir
		// Docker cp creates: srcDir/baseName/contents
		actualSrcDir := filepath.Join(srcDir, baseName)

		// If the expected structure exists, use it
		if _, err := os.Stat(actualSrcDir); err == nil {
			srcDir = actualSrcDir
		}

		filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Create tar header
			header, err := tar.FileInfoHeader(fi, fi.Name())
			if err != nil {
				return err
			}

			// Get relative path from srcDir
			relPath, err := filepath.Rel(srcDir, file)
			if err != nil {
				return err
			}

			// Skip the root directory itself
			if relPath == "." {
				// Use baseName for the directory itself
				header.Name = baseName
			} else {
				// Build path: baseName/relPath
				header.Name = filepath.Join(baseName, relPath)
			}

			if err := tarWriter.WriteHeader(header); err != nil {
				return err
			}

			// Write file content if it's a regular file
			if !fi.IsDir() {
				data, err := os.Open(file)
				if err != nil {
					return err
				}
				defer data.Close()
				if _, err := io.Copy(tarWriter, data); err != nil {
					return err
				}
			}

			return nil
		})
	}()

	return pr, nil
}

// ExportContainer exports a container's filesystem to a compressed tar.gz file.
//
//	in(1): string containerID ID or name of the container to export
//	in(2): string outputFile path to the output .tar.gz file to create
//	out: error non-nil if the export or compression fails
func ExportContainer(containerID string, outputFile string) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Get container info
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %v", err)
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	common.PrintInfoMessage(fmt.Sprintf("Exporting container '%s' to %s", containerName, outputFile))

	// Export container
	reader, err := cli.ContainerExport(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to export container: %v", err)
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
	common.PrintInfoMessage("Compressing container data...")
	written, err := io.Copy(gzipWriter, reader)
	if err != nil {
		return fmt.Errorf("failed to write compressed data: %v", err)
	}

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

	// Import container with label
	importResponse, err := cli.ImageImport(ctx, image.ImportSource{
		Source:     reader,
		SourceName: "-",
	}, imageName, image.ImportOptions{
		// Add RF Swift label
		Changes: []string{
			`LABEL "org.container.project"="rfswift"`,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to import container: %v", err)
	}
	defer importResponse.Close()

	// Read response
	buf := new(strings.Builder)
	io.Copy(buf, importResponse)

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
	defer loadResponse.Body.Close()

	// Parse response to show loaded images
	scanner := bufio.NewScanner(loadResponse.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Loaded image") || strings.Contains(line, "sha256") {
			common.PrintInfoMessage(line)
		}
	}

	common.PrintSuccessMessage("Image(s) imported successfully")
	return nil
}
