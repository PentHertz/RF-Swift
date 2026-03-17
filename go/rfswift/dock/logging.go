/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */

package dock

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	common "penthertz/rfswift/common"
	"penthertz/rfswift/tui"
)

// detectLoggingTool detects which terminal recording tool is available on the system.
//
//	in(1): bool forceScript when true, bypass asciinema detection and require the script command
//	out: string name of the detected tool ("asciinema" or "script")
//	out: error non-nil if no suitable tool is found in PATH
func detectLoggingTool(forceScript bool) (string, error) {
	if forceScript {
		if _, err := exec.LookPath("script"); err != nil {
			return "", fmt.Errorf("script command not found")
		}
		return "script", nil
	}

	if _, err := exec.LookPath("asciinema"); err == nil {
		return "asciinema", nil
	}

	if _, err := exec.LookPath("script"); err == nil {
		return "script", nil
	}

	return "", fmt.Errorf("neither asciinema nor script command found. Install asciinema with: pip install asciinema")
}

// StartLogging starts a terminal recording session using asciinema or script.
//
//	in(1): string outputFile path for the recording output file; auto-generated with timestamp when empty
//	in(2): bool useScript when true, forces use of the script command instead of asciinema
//	out: error non-nil if a session is already active, no tool is found, or recording fails
func StartLogging(outputFile string, useScript bool) error {
	if loggingPID != 0 {
		return fmt.Errorf("a recording session is already active (PID: %d)", loggingPID)
	}

	tool, err := detectLoggingTool(useScript)
	if err != nil {
		return err
	}

	if outputFile == "" {
		timestamp := time.Now().Format("20060102-150405")
		if tool == "asciinema" {
			outputFile = fmt.Sprintf("rfswift-session-%s.cast", timestamp)
		} else {
			outputFile = fmt.Sprintf("rfswift-session-%s.log", timestamp)
		}
	}

	common.PrintInfoMessage(fmt.Sprintf("Starting recording with %s...", tool))
	common.PrintInfoMessage(fmt.Sprintf("Output file: %s", outputFile))

	var cmd *exec.Cmd

	switch tool {
	case "asciinema":
		cmd = exec.Command("asciinema", "rec", outputFile)
	case "script":
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("script", "-q", outputFile)
		} else {
			cmd = exec.Command("script", "-q", "-f", outputFile)
		}
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "RFSWIFT_RECORDING=1")

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start recording: %v", err)
	}

	loggingPID = cmd.Process.Pid
	loggingFile = outputFile
	loggingTool = tool

	stateFile := filepath.Join(os.TempDir(), "rfswift-logging.state")
	state := fmt.Sprintf("%d\n%s\n%s", loggingPID, loggingFile, loggingTool)
	if err := ioutil.WriteFile(stateFile, []byte(state), 0644); err != nil {
		common.PrintWarningMessage(fmt.Sprintf("Failed to save state: %v", err))
	}

	// Set terminal title to show recording indicator
	fmt.Printf("\033]0;⏺ REC | RF Swift\007")

	common.PrintSuccessMessage("Recording started!")
	common.PrintInfoMessage("To stop recording:")
	if tool == "asciinema" {
		common.PrintInfoMessage("  - Press Ctrl+D or type 'exit'")
		common.PrintInfoMessage("  - Or run: rfswift log stop")
	} else {
		common.PrintInfoMessage("  - Type 'exit' or press Ctrl+D")
		common.PrintInfoMessage("  - Or run: rfswift log stop")
	}

	if err := cmd.Wait(); err != nil {
		loggingPID = 0
		loggingFile = ""
		loggingTool = ""
		os.Remove(stateFile)
		return fmt.Errorf("recording ended: %v", err)
	}

	common.PrintSuccessMessage(fmt.Sprintf("Recording saved to: %s", outputFile))

	loggingPID = 0
	loggingFile = ""
	loggingTool = ""
	os.Remove(stateFile)

	return nil
}

// StopLogging stops the current recording session by sending an interrupt signal to the recorder process.
//
//	out: error non-nil if no active session is found or the process cannot be signalled
func StopLogging() error {
	stateFile := filepath.Join(os.TempDir(), "rfswift-logging.state")
	data, err := ioutil.ReadFile(stateFile)
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(parts) >= 3 {
			fmt.Sscanf(parts[0], "%d", &loggingPID)
			loggingFile = parts[1]
			loggingTool = parts[2]
		}
	}

	if loggingPID == 0 {
		return fmt.Errorf("no active recording session found")
	}

	common.PrintInfoMessage(fmt.Sprintf("Stopping recording (PID: %d)...", loggingPID))

	process, err := os.FindProcess(loggingPID)
	if err != nil {
		return fmt.Errorf("failed to find process: %v", err)
	}

	if err := process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("failed to stop recording: %v", err)
	}

	// Reset terminal title
	fmt.Printf("\033]0;RF Swift\007")

	common.PrintSuccessMessage(fmt.Sprintf("Recording stopped: %s", loggingFile))

	loggingPID = 0
	loggingFile = ""
	loggingTool = ""
	os.Remove(stateFile)

	return nil
}

// ReplayLog replays a recorded terminal session using asciinema play or cat depending on file type.
//
//	in(1): string inputFile path to the .cast or .log recording file to replay
//	in(2): float64 speed playback speed multiplier passed to asciinema (e.g. 2.0 for double speed)
//	out: error non-nil if the file does not exist, the required tool is missing, or playback fails
func ReplayLog(inputFile string, speed float64) error {
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", inputFile)
	}

	var tool string
	if strings.HasSuffix(inputFile, ".cast") {
		tool = "asciinema"
	} else {
		tool = "script"
	}

	common.PrintInfoMessage(fmt.Sprintf("Replaying session from: %s", inputFile))

	var cmd *exec.Cmd

	switch tool {
	case "asciinema":
		if _, err := exec.LookPath("asciinema"); err != nil {
			return fmt.Errorf("asciinema not found. Install with: pip install asciinema")
		}
		speedStr := fmt.Sprintf("%.1f", speed)
		cmd = exec.Command("asciinema", "play", "-s", speedStr, inputFile)
	case "script":
		common.PrintWarningMessage("Script logs don't support playback. Displaying content:")
		cmd = exec.Command("cat", inputFile)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to replay session: %v", err)
	}

	return nil
}

// LogEntry holds metadata about a recorded session file.
type LogEntry struct {
	Path    string
	Tool    string
	Size    float64 // KB
	ModTime string
}

// FindLogs searches a directory for rfswift session recordings and returns metadata.
//
//	in(1): string logDir directory to search; defaults to "." when empty
//	out: ([]LogEntry, error)
func FindLogs(logDir string) ([]LogEntry, error) {
	if logDir == "" {
		logDir = "."
	}

	var entries []LogEntry

	err := filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		isRfswiftLog := strings.HasSuffix(path, ".cast") ||
			(strings.HasSuffix(path, ".log") && strings.Contains(path, "rfswift"))
		if !isRfswiftLog {
			return nil
		}

		tool := "script"
		if strings.HasSuffix(path, ".cast") {
			tool = "asciinema"
		}
		entries = append(entries, LogEntry{
			Path:    path,
			Tool:    tool,
			Size:    float64(info.Size()) / 1024,
			ModTime: info.ModTime().Format("2006-01-02 15:04"),
		})
		return nil
	})

	return entries, err
}

// ListLogs lists all recorded session files found under a directory in a styled table.
//
//	in(1): string logDir directory to search for .cast and rfswift-session .log files; defaults to "." when empty
//	out: error non-nil if the directory walk fails
func ListLogs(logDir string) error {
	entries, err := FindLogs(logDir)
	if err != nil {
		return fmt.Errorf("failed to search directory: %v", err)
	}

	if len(entries) == 0 {
		common.PrintInfoMessage("No session recordings found")
		return nil
	}

	rows := make([][]string, len(entries))
	for i, e := range entries {
		rows[i] = []string{fmt.Sprintf("%d", i+1), e.Path, e.Tool, fmt.Sprintf("%.1f KB", e.Size), e.ModTime}
	}

	tui.RenderTable(tui.TableConfig{
		Title:   "Session Recordings",
		Headers: []string{"#", "File", "Tool", "Size", "Date"},
		Rows:    rows,
	})

	return nil
}

// ContainerRunWithRecording runs a container with session recording via asciinema or script.
//
//	in(1): string containerName name to assign to the new container
//	in(2): string recordOutput path for the recording output file; auto-generated with timestamp when empty
//	in(3): string image image name to run; passed as -i flag when non-empty
//	in(4): map[string]string extraArgs additional CLI flag/value pairs to append to the run command
//	out: error non-nil if the recording tool is unavailable or the recorded session fails
func ContainerRunWithRecording(containerName string, recordOutput string, image string, extraArgs map[string]string) error {
	tool, err := detectLoggingTool(false)
	if err != nil {
		return err
	}

	if recordOutput == "" {
		timestamp := time.Now().Format("20060102-150405")
		if tool == "asciinema" {
			recordOutput = fmt.Sprintf("rfswift-run-%s-%s.cast", containerName, timestamp)
		} else {
			recordOutput = fmt.Sprintf("rfswift-run-%s-%s.log", containerName, timestamp)
		}
	}

	common.PrintInfoMessage(fmt.Sprintf("🔴 Recording session with %s to: %s", tool, recordOutput))

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	runCmdStr := fmt.Sprintf("%s run -n %s", executable, containerName)

	if image != "" {
		runCmdStr += fmt.Sprintf(" -i %s", image)
	}

	for flag, value := range extraArgs {
		if value != "" {
			runCmdStr += fmt.Sprintf(" %s %s", flag, value)
		}
	}

	var recordCmd *exec.Cmd

	switch tool {
	case "asciinema":
		recordCmd = exec.Command("asciinema", "rec", "-c", runCmdStr, recordOutput)
	case "script":
		if runtime.GOOS == "darwin" {
			recordCmd = exec.Command("script", "-q", "-c", runCmdStr, recordOutput)
		} else {
			recordCmd = exec.Command("script", "-q", "-f", "-c", runCmdStr, recordOutput)
		}
	}

	recordCmd.Stdin = os.Stdin
	recordCmd.Stdout = os.Stdout
	recordCmd.Stderr = os.Stderr
	recordCmd.Env = append(os.Environ(), "RFSWIFT_RECORDING=1")

	if err := recordCmd.Run(); err != nil {
		return fmt.Errorf("recording session failed: %v", err)
	}

	fmt.Printf("\033]0;Terminal\007")
	common.PrintSuccessMessage(fmt.Sprintf("🔴 Session recorded to: %s", recordOutput))

	return nil
}

// ContainerExecWithRecording executes into a running container with session recording via asciinema or script.
//
//	in(1): string containerIdentifier ID or name of the target container; falls back to the latest rfswift container when empty
//	in(2): string workingDir working directory inside the container; omitted from the exec command when set to "/root" or empty
//	in(3): string recordOutput path for the recording output file; auto-generated with timestamp when empty
//	in(4): string execCommand shell command to run inside the container; omitted when set to "/bin/bash" or empty
//	out: error non-nil if no container is found, the engine client fails, or the recorded session fails
func ContainerExecWithRecording(containerIdentifier string, workingDir string, recordOutput string, execCommand string) error {
	tool, err := detectLoggingTool(false)
	if err != nil {
		return err
	}

	if containerIdentifier == "" {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		containerIdentifier = latestDockerID(labelKey, labelValue)
		if containerIdentifier == "" {
			return fmt.Errorf("no container specified and no recent rfswift container found")
		}
		common.PrintInfoMessage(fmt.Sprintf("Using latest container: %s", containerIdentifier))
	}

	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return err
	}
	defer cli.Close()

	containerName := containerIdentifier
	containerJSON, err := cli.ContainerInspect(ctx, containerIdentifier)
	if err == nil {
		containerName = strings.TrimPrefix(containerJSON.Name, "/")
	}

	if recordOutput == "" {
		timestamp := time.Now().Format("20060102-150405")
		if tool == "asciinema" {
			recordOutput = fmt.Sprintf("rfswift-exec-%s-%s.cast", containerName, timestamp)
		} else {
			recordOutput = fmt.Sprintf("rfswift-exec-%s-%s.log", containerName, timestamp)
		}
	}

	common.PrintInfoMessage(fmt.Sprintf("🔴 Recording session with %s to: %s", tool, recordOutput))

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	execCmdStr := fmt.Sprintf("%s exec -c %s", executable, containerIdentifier)

	if workingDir != "" && workingDir != "/root" {
		execCmdStr += fmt.Sprintf(" -w %s", workingDir)
	}

	if execCommand != "" && execCommand != "/bin/bash" {
		execCmdStr += fmt.Sprintf(" -e %s", execCommand)
	}

	// Pass through VPN config to the recording subprocess
	if containerCfg.vpn != "" {
		execCmdStr += fmt.Sprintf(" --vpn %s", containerCfg.vpn)
	}

	var recordCmd *exec.Cmd

	switch tool {
	case "asciinema":
		recordCmd = exec.Command("asciinema", "rec", "-c", execCmdStr, recordOutput)
	case "script":
		if runtime.GOOS == "darwin" {
			recordCmd = exec.Command("script", "-q", "-c", execCmdStr, recordOutput)
		} else {
			recordCmd = exec.Command("script", "-q", "-f", "-c", execCmdStr, recordOutput)
		}
	}

	recordCmd.Stdin = os.Stdin
	recordCmd.Stdout = os.Stdout
	recordCmd.Stderr = os.Stderr
	recordCmd.Env = append(os.Environ(), "RFSWIFT_RECORDING=1")

	if err := recordCmd.Run(); err != nil {
		return fmt.Errorf("recording session failed: %v", err)
	}

	fmt.Printf("\033]0;Terminal\007")
	common.PrintSuccessMessage(fmt.Sprintf("📹 Session recorded to: %s", recordOutput))

	return nil
}
