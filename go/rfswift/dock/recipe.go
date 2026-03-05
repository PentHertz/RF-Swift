/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * YAML recipe-based image building
 */

package dock

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/term"

	common "penthertz/rfswift/common"
)

// BuildFromRecipe builds a Docker image from a YAML recipe file, optionally
// overriding the image tag and disabling the layer cache during the build.
//
//	in(1): string recipeFile  path to the YAML recipe file
//	in(2): string tagOverride image tag to use instead of the one defined in the recipe (empty string keeps the recipe tag)
//	in(3): bool   noCache     when true, passes --no-cache to the Docker build
//	out:   error              non-nil if any step of the build process fails
func BuildFromRecipe(recipeFile string, tagOverride string, noCache bool) error {
	// Read recipe file
	common.PrintInfoMessage(fmt.Sprintf("Reading recipe from: %s", recipeFile))
	data, err := ioutil.ReadFile(recipeFile)
	if err != nil {
		return fmt.Errorf("failed to read recipe file: %v", err)
	}

	// Parse YAML
	var recipe BuildRecipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return fmt.Errorf("failed to parse recipe: %v", err)
	}

	// Override tag if provided
	if tagOverride != "" {
		recipe.Tag = tagOverride
	}

	// Determine context directory
	recipeDir := filepath.Dir(recipeFile)
	var contextDir string

	if recipe.Context == "" {
		// Default: use recipe directory
		contextDir = recipeDir
		common.PrintInfoMessage(fmt.Sprintf("Using recipe directory as context: %s", contextDir))
	} else {
		// Context specified in recipe
		if filepath.IsAbs(recipe.Context) {
			contextDir = recipe.Context
		} else {
			// Relative to recipe file location
			contextDir = filepath.Join(recipeDir, recipe.Context)
		}
		common.PrintInfoMessage(fmt.Sprintf("Using context directory: %s", contextDir))
	}

	// Verify context directory exists
	if _, err := os.Stat(contextDir); os.IsNotExist(err) {
		return fmt.Errorf("context directory does not exist: %s", contextDir)
	}

	// Generate final image name
	finalImage := fmt.Sprintf("%s:%s", recipe.Name, recipe.Tag)
	common.PrintSuccessMessage(fmt.Sprintf("Building image: %s", finalImage))

	// Generate Dockerfile
	dockerfile, err := generateDockerfile(recipe)
	if err != nil {
		return fmt.Errorf("failed to generate Dockerfile: %v", err)
	}

	// Create temporary directory for build context
	tempDir, err := os.MkdirTemp("", "rfswift-build-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Write Dockerfile to temp directory
	dockerfilePath := filepath.Join(tempDir, "Dockerfile")
	if err := ioutil.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %v", err)
	}

	common.PrintSuccessMessage("Generated Dockerfile:")
	fmt.Println(dockerfile)
	fmt.Println()

	// Copy build context files
	if err := copyBuildContext(recipe, contextDir, tempDir); err != nil {
		return fmt.Errorf("failed to copy build context: %v", err)
	}

	// Build the image
	common.PrintInfoMessage("Starting Docker build...")
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Create tar archive of build context
	buildContext, err := createBuildContextTar(tempDir)
	if err != nil {
		return fmt.Errorf("failed to create build context: %v", err)
	}
	defer buildContext.Close()

	// Build options
	buildOptions := types.ImageBuildOptions{
		Tags:       []string{finalImage},
		Dockerfile: "Dockerfile",
		Remove:     true,
		NoCache:    noCache,
		Labels: map[string]string{
			"org.container.project": "rfswift",
		},
	}

	// Start build
	buildResp, err := cli.ImageBuild(ctx, buildContext, buildOptions)
	if err != nil {
		return fmt.Errorf("failed to start build: %v", err)
	}
	defer buildResp.Body.Close()

	// Stream build output
	termFd, isTerm := term.GetFdInfo(os.Stdout)
	if err := jsonmessage.DisplayJSONMessagesStream(buildResp.Body, os.Stdout, termFd, isTerm, nil); err != nil {
		return fmt.Errorf("error during build: %v", err)
	}

	common.PrintSuccessMessage(fmt.Sprintf("Successfully built image: %s", finalImage))
	return nil
}

// generateDockerfile produces a Dockerfile source string by iterating over the
// steps defined in the provided BuildRecipe and rendering each step type
// (run, copy, workdir, script, cleanup) into the appropriate Dockerfile
// instruction.
//
//	in(1): BuildRecipe recipe  the parsed recipe describing the image to build
//	out:   string              the generated Dockerfile content
//	out:   error               non-nil if the recipe cannot be rendered
func generateDockerfile(recipe BuildRecipe) (string, error) {
	var dockerfile strings.Builder

	// Header
	dockerfile.WriteString("# Generated by RF Swift Build System\n")
	dockerfile.WriteString(fmt.Sprintf("# Recipe: %s\n\n", recipe.Name))

	// Base image
	dockerfile.WriteString(fmt.Sprintf("FROM %s\n\n", recipe.BaseImage))

	// Labels
	if len(recipe.Labels) > 0 {
		for key, value := range recipe.Labels {
			dockerfile.WriteString(fmt.Sprintf("LABEL \"%s\"=\"%s\"\n", key, value))
		}
		dockerfile.WriteString("\n")
	}

	// Process steps
	for _, step := range recipe.Steps {
		switch step.Type {
		case "run":
			for _, cmd := range step.Commands {
				dockerfile.WriteString(fmt.Sprintf("RUN %s\n", cmd))
			}
			dockerfile.WriteString("\n")

		case "copy":
			for _, item := range step.Items {
				dockerfile.WriteString(fmt.Sprintf("COPY %s %s\n", item.Source, item.Destination))
			}
			dockerfile.WriteString("\n")

		case "workdir":
			dockerfile.WriteString(fmt.Sprintf("WORKDIR %s\n\n", step.Path))

		case "script":
			if step.Name != "" {
				dockerfile.WriteString(fmt.Sprintf("# %s\n", step.Name))
			}
			if len(step.Functions) > 0 {
				cmds := make([]string, len(step.Functions))
				for i, fn := range step.Functions {
					cmds[i] = fmt.Sprintf("%s %s", step.Script, fn)
				}
				dockerfile.WriteString(fmt.Sprintf("RUN %s\n\n", strings.Join(cmds, " && \\\n\t")))
			}

		case "cleanup":
			cmds := []string{}
			for _, path := range step.Paths {
				cmds = append(cmds, fmt.Sprintf("rm -rf %s", path))
			}
			if step.AptClean {
				cmds = append(cmds, "apt-fast clean", "rm -rf /var/lib/apt/lists/*")
			}
			if len(cmds) > 0 {
				dockerfile.WriteString(fmt.Sprintf("RUN %s\n\n", strings.Join(cmds, " && \\\n\t")))
			}
		}
	}

	return dockerfile.String(), nil
}

// copyBuildContext copies every file or directory referenced by "copy" steps in
// the recipe from sourceDir into destDir, preserving the relative source paths
// so the generated Dockerfile COPY instructions resolve correctly.
//
//	in(1): BuildRecipe recipe  the parsed recipe whose copy steps are inspected
//	in(2): string      sourceDir  root directory that contains the source files
//	in(3): string      destDir    destination directory (typically the temp build context)
//	out:   error                  non-nil if any source path is missing or a copy operation fails
func copyBuildContext(recipe BuildRecipe, sourceDir, destDir string) error {
	// Find all files that need to be copied
	filesToCopy := make(map[string]bool)

	for _, step := range recipe.Steps {
		if step.Type == "copy" {
			for _, item := range step.Items {
				filesToCopy[item.Source] = true
			}
		}
	}

	// Copy each file/directory
	for source := range filesToCopy {
		srcPath := filepath.Join(sourceDir, source)
		dstPath := filepath.Join(destDir, source)

		srcInfo, err := os.Stat(srcPath)
		if err != nil {
			return fmt.Errorf("source not found: %s", srcPath)
		}

		if srcInfo.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyDir recursively copies the directory tree rooted at src into dst,
// recreating subdirectory structure and delegating individual file copies to
// copyFile.
//
//	in(1): string src  source directory path
//	in(2): string dst  destination directory path
//	out:   error       non-nil if the walk or any copy/mkdir operation fails
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

// copyFile copies a single file from src to dst, creating any missing parent
// directories in the destination path with permission 0755.
//
//	in(1): string src  path to the source file
//	in(2): string dst  path to the destination file
//	out:   error       non-nil if the source cannot be opened, the destination
//	                   cannot be created, or the data copy fails
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	os.MkdirAll(filepath.Dir(dst), 0755)

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// createBuildContextTar produces a streaming tar archive of all files under
// sourceDir. The archive is written in a background goroutine through a pipe so
// the caller can begin reading immediately without waiting for the entire
// directory to be archived.
//
//	in(1): string sourceDir  root directory to archive
//	out:   io.ReadCloser     reader from which the tar stream can be consumed
//	out:   error             non-nil if the pipe cannot be set up (walk errors are propagated through the pipe)
func createBuildContextTar(sourceDir string) (io.ReadCloser, error) {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		tw := tar.NewWriter(pw)
		defer tw.Close()

		filepath.Walk(sourceDir, func(file string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(sourceDir, file)
			if err != nil {
				return err
			}

			if relPath == "." {
				return nil
			}

			header, err := tar.FileInfoHeader(fi, fi.Name())
			if err != nil {
				return err
			}

			header.Name = relPath

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if !fi.IsDir() {
				data, err := os.Open(file)
				if err != nil {
					return err
				}
				defer data.Close()

				if _, err := io.Copy(tw, data); err != nil {
					return err
				}
			}

			return nil
		})
	}()

	return pr, nil
}
