package dock

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
	"os/signal"
    "runtime"

	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/term"
	"golang.org/x/crypto/ssh/terminal"

	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
)

var inout chan []byte

type DockerInst struct {
	net          string
	privileged   bool
	xdisplay     string
	x11forward   string
	usbforward   string
	usbdevice    string
	shell        string
	imagename    string
	extrabinding string
	entrypoint   string
	extrahosts   string
	extraenv     string
	pulse_server string
	network_mode string
}

var dockerObj = DockerInst{net: "host",
	privileged:   true,
	xdisplay:     "DISPLAY=:0",
	entrypoint:   "/bin/bash",
	x11forward:   "/tmp/.X11-unix:/tmp/.X11-unix",
	usbforward:   "/dev/bus/usb:/dev/bus/usb",
	extrabinding: "/dev/ttyACM0:/dev/ttyACM0,/run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/dev/snd:/dev/snd,/dev/dri:/dev/dri,/dev/input:/dev/input", // Some more if needed /run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/dev/snd:/dev/snd,/dev/dri:/dev/dri
	imagename:    "myrfswift:latest",
	extrahosts:   "",
	extraenv:     "",
	network_mode: "host",
	pulse_server: "tcp:localhost:34567",
	shell:        "/bin/bash"} // Instance with default values

func init() {
	updateDockerObjFromConfig()
}

func updateDockerObjFromConfig() {
	config, err := rfutils.ReadOrCreateConfig(common.ConfigFileByPlatform())
	if err != nil {
		log.Printf("Error reading config: %v. Using default values.", err)
		return
	}

	// Update dockerObj with values from config
	dockerObj.imagename = config.General.ImageName
	dockerObj.shell = config.Container.Shell
	dockerObj.network_mode = config.Container.Network
	dockerObj.x11forward = config.Container.X11Forward
	dockerObj.xdisplay = config.Container.XDisplay
	dockerObj.extrahosts = config.Container.ExtraHost
	dockerObj.extraenv = config.Container.ExtraEnv
	dockerObj.pulse_server = config.Audio.PulseServer

	// Handle bindings
	var bindings []string
	for _, binding := range config.Container.Bindings {
		if strings.Contains(binding, "/dev/bus/usb") {
			dockerObj.usbforward = binding
		} else if strings.Contains(binding, ".X11-unix") {
			dockerObj.x11forward = binding
		} else {
			bindings = append(bindings, binding)
		}
	}
	dockerObj.extrabinding = strings.Join(bindings, ",")
}

func resizeTty(ctx context.Context, cli *client.Client, contid string, fd int) {
	for {
		width, height, err := getTerminalSize(fd)
		if err != nil {
			log.Printf("Error getting terminal size: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		err = cli.ContainerResize(ctx, contid, container.ResizeOptions{
			Height: uint(height),
			Width:  uint(width),
		})
		if err != nil {
			log.Printf("Error resizing container TTY: %v", err)
		}

		time.Sleep(1 * time.Second)
	}
}

func checkIfImageIsUpToDate(repo, tag string) (bool, error) {
	architecture := getArchitecture()
	tags, err := getLatestDockerHubTags(repo, architecture)
	if err != nil {
		return false, err
	}

	for _, latestTag := range tags {
		if latestTag.Name == tag {
			return true, nil
		}
	}

	return false, nil
}

func parseImageName(imageName string) (string, string) {
	parts := strings.Split(imageName, ":")
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 {
		tag = parts[1]
	}
	return repo, tag
}

func getLocalImageCreationDate(ctx context.Context, cli *client.Client, imageName string) (time.Time, error) {
	localImage, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return time.Time{}, err
	}
	localImageTime, err := time.Parse(time.RFC3339, localImage.Created)
	if err != nil {
		return time.Time{}, err
	}
	return localImageTime, nil
}

func checkImageStatus(ctx context.Context, cli *client.Client, repo, tag string) (bool, bool, error) {
	architecture := getArchitecture()

	// Get the local image creation date
	localImageTime, err := getLocalImageCreationDate(ctx, cli, fmt.Sprintf("%s:%s", repo, tag))
	if err != nil {
		return false, true, err
	}

	// Get the remote image creation date
	remoteImageTime, err := getRemoteImageCreationDate(repo, tag, architecture)
	if err != nil {
		if err.Error() == "tag not found" {
			return false, true, nil // Custom image if tag not found
		}
		return false, true, err
	}

	// Adjust the remote image creation time by an offset of 2 hours
	remoteImageTimeAdjusted := remoteImageTime.Add(-2 * time.Hour)

	// Compare local and adjusted remote image times
	if localImageTime.Before(remoteImageTimeAdjusted) {
		return false, false, nil // Obsolete
	}
	return true, false, nil // Up-to-date
}

func printContainerProperties(ctx context.Context, cli *client.Client, containerName string, props map[string]string, size string) {
	white := "\033[37m"
	blue := "\033[34m"
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	reset := "\033[0m"

	// Determine if the image is up-to-date, obsolete, or custom
	repo, tag := parseImageName(props["ImageName"])
	isUpToDate, isCustom, err := checkImageStatus(ctx, cli, repo, tag)
	if err != nil {
		if err.Error() != "tag not found" {
			log.Printf("Error checking image status: %v", err)
		}
	}

	imageStatus := fmt.Sprintf("%s (Custom)", props["ImageName"])
	imageStatusColor := yellow
	if !isCustom {
		if isUpToDate {
			imageStatus = fmt.Sprintf("%s (Up to date)", props["ImageName"])
			imageStatusColor = green
		} else {
			imageStatus = fmt.Sprintf("%s (Obsolete)", props["ImageName"])
			imageStatusColor = red
		}
	}

	properties := [][]string{
		{"Container Name", containerName},
		{"X Display", props["XDisplay"]},
		{"Shell", props["Shell"]},
		{"Privileged Mode", props["Privileged"]},
		{"Network Mode", props["NetworkMode"]},
		{"Image Name", imageStatus},
		{"Size on Disk", size},
		{"Bindings", props["Bindings"]},
		{"Extra Hosts", props["ExtraHosts"]},
	}

	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80 // default width if terminal size cannot be determined
	}

	// Adjust width for table borders and padding
	maxContentWidth := width - 4
	if maxContentWidth < 20 {
		maxContentWidth = 20 // Minimum content width
	}

	maxKeyLen := 0
	for _, property := range properties {
		if len(property[0]) > maxKeyLen {
			maxKeyLen = len(property[0])
		}
	}

	maxValueLen := maxContentWidth - maxKeyLen - 7 // 7 for borders and spaces
	if maxValueLen < 10 {
		maxValueLen = 10 // Minimum value length
	}

	totalWidth := maxKeyLen + maxValueLen + 7

	// Print the title in blue, aligned to the left with some padding
	title := "🧊 Container Summary"
	leftPadding := 2 // You can adjust this value for more or less left padding
	fmt.Printf("%s%s%s%s%s\n", blue, strings.Repeat(" ", leftPadding), title, strings.Repeat(" ", totalWidth-leftPadding-len(title)), reset)

	fmt.Printf("%s", white) // Switch to white color for the box
	fmt.Printf("╭%s╮\n", strings.Repeat("─", totalWidth-2))

	for i, property := range properties {
		key := property[0]
		value := property[1]
		valueColor := white

		if key == "Image Name" {
			valueColor = imageStatusColor
		}

		// Wrap long values
		wrappedValue := wrapText(value, maxValueLen)
		valueLines := strings.Split(wrappedValue, "\n")

		for j, line := range valueLines {
			if j == 0 {
				fmt.Printf("│ %-*s │ %s%-*s%s │\n", maxKeyLen, key, valueColor, maxValueLen, line, reset)
			} else {
				fmt.Printf("│ %-*s │ %s%-*s%s │\n", maxKeyLen, "", valueColor, maxValueLen, line, reset)
			}

			if j < len(valueLines)-1 {
				fmt.Printf("│%s│%s│\n", strings.Repeat(" ", maxKeyLen+2), strings.Repeat(" ", maxValueLen+2))
			}
		}

		if i < len(properties)-1 {
			fmt.Printf("├%s┼%s┤\n", strings.Repeat("─", maxKeyLen+2), strings.Repeat("─", maxValueLen+2))
		}
	}

	fmt.Printf("╰%s╯\n", strings.Repeat("─", totalWidth-2))
	fmt.Printf("%s", reset)
	fmt.Println() // Ensure we end with a newline for clarity
}

func wrapText(text string, maxWidth int) string {
	var result strings.Builder
	currentLineWidth := 0

	words := strings.Fields(text)
	for i, word := range words {
		if currentLineWidth+len(word) > maxWidth {
			if currentLineWidth > 0 {
				result.WriteString("\n")
				currentLineWidth = 0
			}
			if len(word) > maxWidth {
				for len(word) > maxWidth {
					result.WriteString(word[:maxWidth] + "\n")
					word = word[maxWidth:]
				}
			}
		}
		result.WriteString(word)
		currentLineWidth += len(word)
		if i < len(words)-1 && currentLineWidth+1+len(words[i+1]) <= maxWidth {
			result.WriteString(" ")
			currentLineWidth++
		}
	}

	return result.String()
}

func DockerLast(ifilter string, labelKey string, labelValue string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	containerFilters := filters.NewArgs()
	if ifilter != "" {
		containerFilters.Add("ancestor", ifilter)
	}
	if labelKey != "" && labelValue != "" {
		containerFilters.Add("label", fmt.Sprintf("%s=%s", labelKey, labelValue))
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Limit:   10,
		Filters: containerFilters,
	})
	if err != nil {
		panic(err)
	}

	//rfutils.ClearScreen()

	tableData := [][]string{}
	for _, container := range containers {
		created := time.Unix(container.Created, 0).Format(time.RFC3339)
		imageTag := container.Image
		containerName := container.Names[0]
		if containerName[0] == '/' {
			containerName = containerName[1:]
		}
		containerID := container.ID[:12]
		command := container.Command
		tableData = append(tableData, []string{
			created,
			imageTag,
			containerName,
			containerID,
			command,
		})
	}

	headers := []string{"Created", "Image Tag", "Container Name", "Container ID", "Command"}
	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80
	}

	columnWidths := make([]int, len(headers))
	for i, header := range headers {
		columnWidths[i] = len(header)
	}
	for _, row := range tableData {
		for i, col := range row {
			if len(col) > columnWidths[i] {
				columnWidths[i] = len(col)
			}
		}
	}

	totalWidth := len(headers) + 1
	for _, w := range columnWidths {
		totalWidth += w + 2
	}

	if totalWidth > width {
		excess := totalWidth - width
		for i := range columnWidths {
			reduction := excess / len(columnWidths)
			if columnWidths[i] > reduction {
				columnWidths[i] -= reduction
				excess -= reduction
			}
		}
		totalWidth = width
	}

	pink := "\033[35m"
	white := "\033[37m"
	reset := "\033[0m"
	title := "🤖 Last Run Containers"

	fmt.Printf("%s%s%s%s%s\n", pink, strings.Repeat(" ", 2), title, strings.Repeat(" ", totalWidth-2-len(title)), reset)
	fmt.Print(white)

	printHorizontalBorder(columnWidths, "┌", "┬", "┐")
	printRow(headers, columnWidths, "│")
	printHorizontalBorder(columnWidths, "├", "┼", "┤")

	for i, row := range tableData {
		printRow(row, columnWidths, "│")
		if i < len(tableData)-1 {
			printHorizontalBorder(columnWidths, "├", "┼", "┤")
		}
	}

	printHorizontalBorder(columnWidths, "└", "┴", "┘")

	fmt.Print(reset)
	fmt.Println()
}

func printHorizontalBorder(columnWidths []int, left, middle, right string) {
	fmt.Print(left)
	for i, width := range columnWidths {
		fmt.Print(strings.Repeat("─", width+2))
		if i < len(columnWidths)-1 {
			fmt.Print(middle)
		}
	}
	fmt.Println(right)
}

func printRow(row []string, columnWidths []int, separator string) {
	fmt.Print(separator)
	for i, col := range row {
		fmt.Printf(" %-*s ", columnWidths[i], col)
		fmt.Print(separator)
	}
	fmt.Println()
}

func distributeColumnWidths(availableWidth int, columnWidths []int) []int {
	totalCurrentWidth := 0
	for _, width := range columnWidths {
		totalCurrentWidth += width
	}
	for i := range columnWidths {
		columnWidths[i] = int(float64(columnWidths[i]) / float64(totalCurrentWidth) * float64(availableWidth))
		if columnWidths[i] < 1 {
			columnWidths[i] = 1
		}
	}
	return columnWidths
}

func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength-3] + "..."
}

func latestDockerID(labelKey string, labelValue string) string {
	/* Get latest Docker container ID by image label
	   in(1): string label key
	   in(2): string label value
	   out: string container ID
	*/
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	// Filter containers by the specified image label
	containerFilters := filters.NewArgs()
	containerFilters.Add("label", fmt.Sprintf("%s=%s", labelKey, labelValue))

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: containerFilters,
	})
	if err != nil {
		panic(err)
	}

	var latestContainer types.Container
	for _, container := range containers {
		if latestContainer.ID == "" || container.Created > latestContainer.Created {
			latestContainer = container
		}
	}

	if latestContainer.ID == "" {
		fmt.Println("No container found with the specified image label.")
		return ""
	}

	return latestContainer.ID
}

func getContainerProperties(ctx context.Context, cli *client.Client, containerID string) (map[string]string, error) {
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	// Extract the DISPLAY environment variable value
	var xdisplay string
	for _, env := range containerJSON.Config.Env {
		if strings.HasPrefix(env, "DISPLAY=") {
			xdisplay = strings.TrimPrefix(env, "DISPLAY=")
			break
		}
	}

	// Get the image details to find the size
	imageInfo, _, err := cli.ImageInspectWithRaw(ctx, containerJSON.Image)
	if err != nil {
		return nil, err
	}
	imageSize := fmt.Sprintf("%.2f MB", float64(imageInfo.Size)/1024/1024)

	props := map[string]string{
		"XDisplay":    xdisplay,
		"Shell":       containerJSON.Path,
		"Privileged":  fmt.Sprintf("%v", containerJSON.HostConfig.Privileged),
		"NetworkMode": string(containerJSON.HostConfig.NetworkMode),
		"ImageName":   containerJSON.Config.Image,
		"ImageHash":   imageInfo.ID,
		"Bindings":    strings.Join(containerJSON.HostConfig.Binds, ","),
		"ExtraHosts":  strings.Join(containerJSON.HostConfig.ExtraHosts, ","),
		"Size":        imageSize,
	}

	return props, nil
}

func DockerExec(containerIdentifier string, WorkingDir string) {
    ctx := context.Background()
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        common.PrintErrorMessage(err)
        return
    }
    defer cli.Close()

    if containerIdentifier == "" {
        labelKey := "org.container.project"
        labelValue := "rfswift"
        containerIdentifier = latestDockerID(labelKey, labelValue)
    }

    if err := cli.ContainerStart(ctx, containerIdentifier, container.StartOptions{}); err != nil {
        common.PrintErrorMessage(err)
        return
    }

    common.PrintSuccessMessage(fmt.Sprintf("Container '%s' started successfully", containerIdentifier))

    // Get container properties and name
    props, err := getContainerProperties(ctx, cli, containerIdentifier)
    if err != nil {
        common.PrintErrorMessage(err)
        return
    }

    containerJSON, err := cli.ContainerInspect(ctx, containerIdentifier)
    if err != nil {
        common.PrintErrorMessage(err)
        return
    }
    containerName := strings.TrimPrefix(containerJSON.Name, "/")

    size := props["Size"]
    printContainerProperties(ctx, cli, containerName, props, size)

    // Create exec configuration
    execConfig := types.ExecConfig{
        AttachStdin:  true,
        AttachStdout: true,
        AttachStderr: true,
        Tty:          true,
        Cmd:          []string{dockerObj.shell},
        WorkingDir:   WorkingDir,
    }

    // Create exec instance
    execID, err := cli.ContainerExecCreate(ctx, containerIdentifier, execConfig)
    if err != nil {
        common.PrintErrorMessage(fmt.Errorf("failed to create exec instance: %v", err))
        return
    }

    // Attach to the exec instance
    attachResp, err := cli.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{Tty: true})
    if err != nil {
        common.PrintErrorMessage(fmt.Errorf("failed to attach to exec instance: %v", err))
        return
    }
    defer attachResp.Close()

    // Setup raw terminal
    inFd, inIsTerminal := term.GetFdInfo(os.Stdin)
    outFd, outIsTerminal := term.GetFdInfo(os.Stdout)

    if inIsTerminal {
        state, err := term.SetRawTerminal(inFd)
        if err != nil {
            common.PrintErrorMessage(fmt.Errorf("failed to set raw terminal: %v", err))
            return
        }
        defer term.RestoreTerminal(inFd, state)
    }

    // Start the exec instance
    if err := cli.ContainerExecStart(ctx, execID.ID, types.ExecStartCheck{Tty: true}); err != nil {
        common.PrintErrorMessage(fmt.Errorf("failed to start exec instance: %v", err))
        return
    }

    // Handle terminal resize
    go func() {
        switch runtime.GOOS {
        case "linux", "darwin":
            sigchan := make(chan os.Signal, 1)
            signal.Notify(sigchan, syscallsigwin())
            defer signal.Stop(sigchan)

            for range sigchan {
                if outIsTerminal {
                    if size, err := term.GetWinsize(outFd); err == nil {
                        cli.ContainerExecResize(ctx, execID.ID, container.ResizeOptions{
                            Height: uint(size.Height),
                            Width:  uint(size.Width),
                        })
                    }
                }
            }
        case "windows":
            ticker := time.NewTicker(500 * time.Millisecond)
            defer ticker.Stop()

            var lastHeight, lastWidth uint16
            for range ticker.C {
                if outIsTerminal {
                    if size, err := term.GetWinsize(outFd); err == nil {
                        if size.Height != lastHeight || size.Width != lastWidth {
                            cli.ContainerExecResize(ctx, execID.ID, container.ResizeOptions{
                                Height: uint(size.Height),
                                Width:  uint(size.Width),
                            })
                            lastHeight = size.Height
                            lastWidth = size.Width
                        }
                    }
                }
            }
        }
    }()

    // Trigger initial resize
    if outIsTerminal {
        if size, err := term.GetWinsize(outFd); err == nil {
            cli.ContainerExecResize(ctx, execID.ID, container.ResizeOptions{
                Height: uint(size.Height),
                Width:  uint(size.Width),
            })
        }
    }

    // Handle I/O
    outputDone := make(chan error)
    go func() {
        _, err := io.Copy(os.Stdout, attachResp.Reader)
        outputDone <- err
    }()

    go func() {
        if inIsTerminal {
            io.Copy(attachResp.Conn, os.Stdin)
        } else {
            io.Copy(attachResp.Conn, os.Stdin)
        }
        attachResp.CloseWrite()
    }()

    select {
    case err := <-outputDone:
        if err != nil {
            common.PrintErrorMessage(fmt.Errorf("error in output processing: %v", err))
        }
    }

    common.PrintSuccessMessage(fmt.Sprintf("Shell session in container '%s' ended", containerName))
}

func DockerRun(containerName string) {
	/*
	 *   Create a container with a specific name and run it
	 */
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	bindings := combineBindings(dockerObj.x11forward, dockerObj.usbforward, dockerObj.extrabinding)
	extrahosts := splitAndCombine(dockerObj.extrahosts)
	dockerenv := combineEnv(dockerObj.xdisplay, dockerObj.pulse_server, dockerObj.extraenv)

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:        dockerObj.imagename,
		Cmd:          []string{dockerObj.shell},
		Env:          dockerenv,
		OpenStdin:    true,
		StdinOnce:    false,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Labels: map[string]string{
			"org.container.project": "rfswift",
		},
	}, &container.HostConfig{
		NetworkMode: container.NetworkMode(dockerObj.network_mode),
		Binds:       bindings,
		Privileged:  true,
		ExtraHosts:  extrahosts,
	}, &network.NetworkingConfig{}, nil, containerName)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	waiter, err := cli.ContainerAttach(ctx, resp.ID, container.AttachOptions{
		Stderr: true,
		Stdout: true,
		Stdin:  true,
		Stream: true,
	})
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer waiter.Close()

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		common.PrintErrorMessage(err)
		return
	}

	props, err := getContainerProperties(ctx, cli, resp.ID)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	size := props["Size"]
	printContainerProperties(ctx, cli, containerName, props, size)

	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' started successfully", containerName))

	handleIOStreams(waiter)

	fd := int(os.Stdin.Fd())
	if terminal.IsTerminal(fd) {
		oldState, err := terminal.MakeRaw(fd)
		if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		defer terminal.Restore(fd, oldState)

		go resizeTty(ctx, cli, resp.ID, fd)
		go readAndWriteInput(waiter)
	}

	waitForContainer(ctx, cli, resp.ID)
}

func execCommandInContainer(ctx context.Context, cli *client.Client, contid, WorkingDir string) {
	execShell := []string{}
	if dockerObj.shell != "" {
		execShell = append(execShell, strings.Split(dockerObj.shell, " ")...)
	}

	optionsCreate := types.ExecConfig{
		WorkingDir:   WorkingDir,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Detach:       false,
		Privileged:   true,
		Tty:          true,
		Cmd:          execShell,
	}

	rst, err := cli.ContainerExecCreate(ctx, contid, optionsCreate)
	if err != nil {
		panic(err)
	}

	optionsStartCheck := types.ExecStartCheck{
		Detach: false,
		Tty:    true,
	}

	response, err := cli.ContainerExecAttach(ctx, rst.ID, optionsStartCheck)
	if err != nil {
		panic(err)
	}
	defer response.Close()

	handleIOStreams(response)
	waitForContainer(ctx, cli, contid)
}

func attachAndInteract(ctx context.Context, cli *client.Client, contid string) {
	response, err := cli.ContainerAttach(ctx, contid, container.AttachOptions{
		Stderr: true,
		Stdout: true,
		Stdin:  true,
		Stream: true,
	})
	if err != nil {
		panic(err)
	}
	defer response.Close()

	handleIOStreams(response)

	fd := int(os.Stdin.Fd())
	if terminal.IsTerminal(fd) {
		oldState, err := terminal.MakeRaw(fd)
		if err != nil {
			panic(err)
		}
		defer terminal.Restore(fd, oldState)

		go resizeTty(ctx, cli, contid, fd)
		go readAndWriteInput(response)
	}

	waitForContainer(ctx, cli, contid)
}

func handleIOStreams(response types.HijackedResponse) {
	go io.Copy(os.Stdout, response.Reader)
	go io.Copy(os.Stderr, response.Reader)
	go io.Copy(response.Conn, os.Stdin)
}

func readAndWriteInput(response types.HijackedResponse) {
	reader := bufio.NewReaderSize(os.Stdin, 4096) // Increased buffer size for larger inputs
	for {
		input, err := reader.ReadByte()
		if err != nil {
			return
		}
		response.Conn.Write([]byte{input})
	}
}

func waitForContainer(ctx context.Context, cli *client.Client, contid string) {
	statusCh, errCh := cli.ContainerWait(ctx, contid, container.WaitConditionNextExit)
	select {
	case err := <-errCh:
		if err != nil {
			panic(err)
		}
	case <-statusCh:
	}
}

func combineBindings(x11forward, usbforward, extrabinding string) []string {
	bindings := append(strings.Split(x11forward, ","), strings.Split(usbforward, ",")...)
	if extrabinding != "" {
		bindings = append(bindings, strings.Split(extrabinding, ",")...)
	}
	return bindings
}

func splitAndCombine(commaSeparated string) []string {
	if commaSeparated == "" {
		return []string{}
	}
	return strings.Split(commaSeparated, ",")
}

func combineEnv(xdisplay, pulse_server, extraenv string) []string {
	dockerenv := append(strings.Split(xdisplay, ","), "PULSE_SERVER="+pulse_server)
	if extraenv != "" {
		dockerenv = append(dockerenv, strings.Split(extraenv, ",")...)
	}
	return dockerenv
}

func DockerCommit(contid string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	if err := cli.ContainerStart(ctx, contid, container.StartOptions{}); err != nil {
		panic(err)
	}

	commitResp, err := cli.ContainerCommit(ctx, contid, container.CommitOptions{Reference: dockerObj.imagename})
	if err != nil {
		panic(err)
	}
	fmt.Println(commitResp.ID)
}

func DockerPull(imageref string, imagetag string) {
	/* Pulls an image from a registry
	   in(1): string Image reference
	   in(2): string Image tag target
	*/

	if imagetag == "" { // if tag is empty, keep same tag
		imagetag = imageref
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	out, err := cli.ImagePull(ctx, imageref, image.PullOptions{})
	if err != nil {
		panic(err)
	}
	defer out.Close()

	fd, isTerminal := term.GetFdInfo(os.Stdout)
	jsonDecoder := json.NewDecoder(out)

	for {
		var msg jsonmessage.JSONMessage
		if err := jsonDecoder.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}

		if isTerminal {
			_ = jsonmessage.DisplayJSONMessagesStream(out, os.Stdout, fd, isTerminal, nil)
		} else {
			fmt.Println(msg)
		}
	}

	err = cli.ImageTag(ctx, imageref, imagetag)
	if err != nil {
		panic(err)
	}
}

func DockerTag(imageref string, imagetag string) {
	/* Rename an image to another name
	   in(1): string Image reference
	   in(2): string Image tag target
	*/
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	err = cli.ImageTag(ctx, imageref, imagetag)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("[+] Image renamed!")
	}
}

func DockerRename(currentIdentifier string, newName string) {
	/* Rename a container by ID or name
	   in(1): string current container ID or name
	   in(2): string new container name
	*/
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	// Attempt to find the container by the identifier (name or ID)
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		panic(err)
	}

	var containerID string
	for _, container := range containers {
		if container.ID == currentIdentifier || container.Names[0] == "/"+currentIdentifier {
			containerID = container.ID
			break
		}
	}

	if containerID == "" {
		log.Fatalf("Container with ID or name '%s' not found.", currentIdentifier)
	}

	// Rename the container
	err = cli.ContainerRename(ctx, containerID, newName)
	if err != nil {
		panic(err)
	} else {
		fmt.Printf("[+] Container '%s' renamed to '%s'!\n", currentIdentifier, newName)
	}
}

func DockerRemove(containerIdentifier string) {
	/* Remove a container by ID or name
	   in(1): string container ID or name
	*/
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	// Attempt to find the container by the identifier (name or ID)
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	var containerID string
	for _, container := range containers {
		if container.ID == containerIdentifier || container.Names[0] == "/"+containerIdentifier {
			containerID = container.ID
			break
		}
	}

	if containerID == "" {
		common.PrintErrorMessage(fmt.Errorf("container with ID or name '%s' not found", containerIdentifier))
		return
	}

	// Remove the container
	err = cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	if err != nil {
		common.PrintErrorMessage(err)
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Container '%s' removed successfully", containerIdentifier))
	}
}

func ListImages(labelKey string, labelValue string) ([]image.Summary, error) {
	/* List RF Swift Images
	   in(1): string labelKey
	   in(2): string labelValue
	   out: Tuple ImageSummary, error
	*/
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	// Filter images by the specified image label
	imagesFilters := filters.NewArgs()
	imagesFilters.Add("label", fmt.Sprintf("%s=%s", labelKey, labelValue))

	images, err := cli.ImageList(ctx, image.ListOptions{
		All:     true,
		Filters: imagesFilters,
	})
	if err != nil {
		return nil, err
	}

	// Only display images with RepoTags
	var filteredImages []image.Summary
	for _, image := range images {
		if len(image.RepoTags) > 0 {
			filteredImages = append(filteredImages, image)
		}
	}

	return filteredImages, nil
}

func PrintImagesTable(labelKey string, labelValue string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Error creating Docker client: %v", err)
	}
	defer cli.Close()

	images, err := ListImages(labelKey, labelValue)
	if err != nil {
		log.Fatalf("Error listing images: %v", err)
	}

	rfutils.ClearScreen()

	// Prepare table data
	tableData := [][]string{}
	maxStatusLength := 0
	for _, image := range images {
		for _, repoTag := range image.RepoTags {
			repoTagParts := strings.Split(repoTag, ":")
			repository := repoTagParts[0]
			tag := repoTagParts[1]

			// Check image status
			isUpToDate, isCustom, err := checkImageStatus(ctx, cli, repository, tag)
			var status string
			if err != nil {
				status = "Error"
			} else if isCustom {
				status = "Custom"
			} else if isUpToDate {
				status = "Up to date"
			} else {
				status = "Obsolete"
			}

			if len(status) > maxStatusLength {
				maxStatusLength = len(status)
			}

			created := time.Unix(image.Created, 0).Format(time.RFC3339)
			size := fmt.Sprintf("%.2f MB", float64(image.Size)/1024/1024)

			tableData = append(tableData, []string{
				repository,
				tag,
				image.ID[:12],
				created,
				size,
				status,
			})
		}
	}

	headers := []string{"Repository", "Tag", "Image ID", "Created", "Size", "Status"}
	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80 // default width if terminal size cannot be determined
	}

	columnWidths := make([]int, len(headers))
	for i, header := range headers {
		columnWidths[i] = len(header)
	}
	for _, row := range tableData {
		for i, col := range row {
			if len(col) > columnWidths[i] {
				columnWidths[i] = len(col)
			}
		}
	}

	// Ensure the "Status" column is wide enough
	columnWidths[len(columnWidths)-1] = max(columnWidths[len(columnWidths)-1], maxStatusLength)

	// Adjust column widths to fit the terminal width
	totalWidth := len(headers) + 1 // Adding 1 for the left border
	for _, w := range columnWidths {
		totalWidth += w + 2 // Adding 2 for padding
	}

	if totalWidth > width {
		excess := totalWidth - width
		for i := range columnWidths[:len(columnWidths)-1] { // Don't reduce the last (Status) column
			reduction := excess / (len(columnWidths) - 1)
			if columnWidths[i] > reduction {
				columnWidths[i] -= reduction
				excess -= reduction
			}
		}
		totalWidth = width
	}

	yellow := "\033[33m"
	white := "\033[37m"
	reset := "\033[0m"
	title := "📦 RF Swift Images"

	fmt.Printf("%s%s%s%s%s\n", yellow, strings.Repeat(" ", 2), title, strings.Repeat(" ", totalWidth-2-len(title)), reset)
	fmt.Print(white)

	printHorizontalBorder(columnWidths, "┌", "┬", "┐")
	printRow(headers, columnWidths, "│")
	printHorizontalBorder(columnWidths, "├", "┼", "┤")

	for i, row := range tableData {
		printRowWithColor(row, columnWidths, "│")
		if i < len(tableData)-1 {
			printHorizontalBorder(columnWidths, "├", "┼", "┤")
		}
	}

	printHorizontalBorder(columnWidths, "└", "┴", "┘")

	fmt.Print(reset)
	fmt.Println()
}

func printRowWithColor(row []string, columnWidths []int, separator string) {
	fmt.Print(separator)
	for i, col := range row {
		if i == len(row)-1 { // Status column
			status := col
			color := ""
			switch status {
			case "Custom":
				color = "\033[33m" // Yellow
			case "Up to date":
				color = "\033[32m" // Green
			case "Obsolete":
				color = "\033[31m" // Red
			case "Error":
				color = "\033[31m" // Red
			}
			fmt.Printf(" %s%-*s\033[0m ", color, columnWidths[i], status)
		} else {
			fmt.Printf(" %-*s ", columnWidths[i], truncateString(col, columnWidths[i]))
		}
		fmt.Print(separator)
	}
	fmt.Println()
}

func stripAnsiCodes(s string) string {
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansi.ReplaceAllString(s, "")
}

func DeleteImage(imageIDOrTag string) error {
    ctx := context.Background()
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        common.PrintErrorMessage(fmt.Errorf("failed to create Docker client: %v", err))
        return err
    }
    defer cli.Close()

    common.PrintInfoMessage(fmt.Sprintf("Attempting to delete image: %s", imageIDOrTag))

    // List all images
    images, err := cli.ImageList(ctx, image.ListOptions{All: true})
    if err != nil {
        common.PrintErrorMessage(fmt.Errorf("failed to list images: %v", err))
        return err
    }

    var imageToDelete image.Summary
    imageFound := false

    for _, img := range images {
        // Check if the full image ID matches
        if img.ID == "sha256:"+imageIDOrTag || img.ID == imageIDOrTag {
            imageToDelete = img
            imageFound = true
            break
        }

        // Check if any RepoTags match exactly
        for _, tag := range img.RepoTags {
            if tag == imageIDOrTag {
                imageToDelete = img
                imageFound = true
                break
            }
        }

        // If image is found by tag, break the outer loop
        if imageFound {
            break
        }
    }

    if !imageFound {
        common.PrintErrorMessage(fmt.Errorf("image not found: %s", imageIDOrTag))
        common.PrintInfoMessage("Available images:")
        for _, img := range images {
            common.PrintInfoMessage(fmt.Sprintf("ID: %s, Tags: %v", strings.TrimPrefix(img.ID, "sha256:"), img.RepoTags))
        }
        return fmt.Errorf("image not found: %s", imageIDOrTag)
    }

    imageID := imageToDelete.ID
    common.PrintInfoMessage(fmt.Sprintf("Found image to delete: ID: %s, Tags: %v", strings.TrimPrefix(imageID, "sha256:"), imageToDelete.RepoTags))

    // Ask for user confirmation
    reader := bufio.NewReader(os.Stdin)
    common.PrintWarningMessage(fmt.Sprintf("Are you sure you want to delete this image? (y/n): "))
    response, err := reader.ReadString('\n')
    if err != nil {
        common.PrintErrorMessage(fmt.Errorf("failed to read user input: %v", err))
        return err
    }
    response = strings.ToLower(strings.TrimSpace(response))
    if response != "y" && response != "yes" {
        common.PrintInfoMessage("Image deletion cancelled by user.")
        return nil
    }

    // Find and remove containers using the image
    containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
    if err != nil {
        common.PrintErrorMessage(fmt.Errorf("failed to list containers: %v", err))
        return err
    }

    for _, icontainer := range containers {
        if icontainer.ImageID == imageID {
            common.PrintWarningMessage(fmt.Sprintf("Removing container: %s", icontainer.ID[:12]))
            if err := cli.ContainerRemove(ctx, icontainer.ID, container.RemoveOptions{Force: true}); err != nil {
                common.PrintWarningMessage(fmt.Sprintf("Failed to remove container %s: %v", icontainer.ID[:12], err))
            }
        }
    }

    // Attempt to delete the image
    _, err = cli.ImageRemove(ctx, imageID, image.RemoveOptions{Force: true, PruneChildren: true})
    if err != nil {
        common.PrintErrorMessage(fmt.Errorf("failed to delete image %s: %v", imageIDOrTag, err))
        return err
    }

    common.PrintSuccessMessage(fmt.Sprintf("Successfully deleted image: %s", imageIDOrTag))
    return nil
}