package dock

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"time"

	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
	"github.com/moby/term"
	"golang.org/x/crypto/ssh/terminal"

	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
)

type HostConfigFull struct {
	Binds                []string                 `json:"Binds"`
	ContainerIDFile      string                   `json:"ContainerIDFile"`
	LogConfig            LogConfig                `json:"LogConfig"`
	NetworkMode          string                   `json:"NetworkMode"`
	PortBindings         map[string][]PortBinding `json:"PortBindings"`
	RestartPolicy        RestartPolicy            `json:"RestartPolicy"`
	AutoRemove           bool                     `json:"AutoRemove"`
	VolumeDriver         string                   `json:"VolumeDriver"`
	VolumesFrom          []string                 `json:"VolumesFrom"`
	ConsoleSize          []int                    `json:"ConsoleSize"`
	CapAdd               []string                 `json:"CapAdd"`
	CapDrop              []string                 `json:"CapDrop"`
	CgroupnsMode         string                   `json:"CgroupnsMode"`
	Dns                  []string                 `json:"Dns"`
	DnsOptions           []string                 `json:"DnsOptions"`
	DnsSearch            []string                 `json:"DnsSearch"`
	ExtraHosts           []string                 `json:"ExtraHosts"`
	GroupAdd             []string                 `json:"GroupAdd"`
	IpcMode              string                   `json:"IpcMode"`
	Cgroup               string                   `json:"Cgroup"`
	Links                []string                 `json:"Links"`
	OomScoreAdj          int                      `json:"OomScoreAdj"`
	PidMode              string                   `json:"PidMode"`
	Privileged           bool                     `json:"Privileged"`
	PublishAllPorts      bool                     `json:"PublishAllPorts"`
	ReadonlyRootfs       bool                     `json:"ReadonlyRootfs"`
	SecurityOpt          []string                 `json:"SecurityOpt"`
	UTSMode              string                   `json:"UTSMode"`
	UsernsMode           string                   `json:"UsernsMode"`
	ShmSize              int64                    `json:"ShmSize"`
	Runtime              string                   `json:"Runtime"`
	Isolation            string                   `json:"Isolation"`
	CpuShares            int64                    `json:"CpuShares"`
	Memory               int64                    `json:"Memory"`
	NanoCpus             int64                    `json:"NanoCpus"`
	CgroupParent         string                   `json:"CgroupParent"`
	BlkioWeight          uint16                   `json:"BlkioWeight"`
	BlkioWeightDevice    []ThrottleDevice         `json:"BlkioWeightDevice"`
	BlkioDeviceReadBps   []ThrottleDevice         `json:"BlkioDeviceReadBps"`
	BlkioDeviceWriteBps  []ThrottleDevice         `json:"BlkioDeviceWriteBps"`
	BlkioDeviceReadIOps  []ThrottleDevice         `json:"BlkioDeviceReadIOps"`
	BlkioDeviceWriteIOps []ThrottleDevice         `json:"BlkioDeviceWriteIOps"`
	CpuPeriod            int64                    `json:"CpuPeriod"`
	CpuQuota             int64                    `json:"CpuQuota"`
	CpuRealtimePeriod    int64                    `json:"CpuRealtimePeriod"`
	CpuRealtimeRuntime   int64                    `json:"CpuRealtimeRuntime"`
	CpusetCpus           string                   `json:"CpusetCpus"`
	CpusetMems           string                   `json:"CpusetMems"`
	Devices              []DeviceMapping          `json:"Devices"`
	DeviceCgroupRules    []string                 `json:"DeviceCgroupRules"`
	DeviceRequests       []DeviceRequest          `json:"DeviceRequests"`
	MemoryReservation    int64                    `json:"MemoryReservation"`
	MemorySwap           int64                    `json:"MemorySwap"`
	MemorySwappiness     *int                     `json:"MemorySwappiness"`
	OomKillDisable       *bool                    `json:"OomKillDisable"`
	PidsLimit            *int64                   `json:"PidsLimit"`
	Ulimits              []Ulimit                 `json:"Ulimits"`
	CpuCount             int64                    `json:"CpuCount"`
	CpuPercent           int64                    `json:"CpuPercent"`
	IOMaximumIOps        int64                    `json:"IOMaximumIOps"`
	IOMaximumBandwidth   int64                    `json:"IOMaximumBandwidth"`
	MaskedPaths          []string                 `json:"MaskedPaths"`
	ReadonlyPaths        []string                 `json:"ReadonlyPaths"`
}

// Supporting structs
type LogConfig struct {
	Type   string            `json:"Type"`
	Config map[string]string `json:"Config"`
}

type RestartPolicy struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

type PortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type ThrottleDevice struct {
	Path string `json:"Path"`
	Rate uint64 `json:"Rate"`
}

type DeviceMapping struct {
	PathOnHost        string `json:"PathOnHost"`
	PathInContainer   string `json:"PathInContainer"`
	CgroupPermissions string `json:"CgroupPermissions"`
}

type DeviceRequest struct {
	Driver       string            `json:"Driver"`
	Count        int               `json:"Count"`
	DeviceIDs    []string          `json:"DeviceIDs"`
	Capabilities [][]string        `json:"Capabilities"`
	Options      map[string]string `json:"Options"`
}

type Ulimit struct {
	Name string `json:"Name"`
	Hard int64  `json:"Hard"`
	Soft int64  `json:"Soft"`
}

var inout chan []byte

type DockerInst struct {
	net           string
	privileged    bool
	xdisplay      string
	x11forward    string
	usbforward    string
	usbdevice     string
	shell         string
	imagename     string
	repotag       string
	extrabinding  string
	entrypoint    string
	extrahosts    string
	extraenv      string
	pulse_server  string
	network_mode  string
	exposed_ports string
	binded_ports  string
}

var dockerObj = DockerInst{net: "host",
	privileged:    true,
	xdisplay:      "DISPLAY=:0",
	entrypoint:    "/bin/bash",
	x11forward:    "/tmp/.X11-unix:/tmp/.X11-unix",
	usbforward:    "/dev/bus/usb:/dev/bus/usb",
	extrabinding:  "/dev/ttyACM0:/dev/ttyACM0,/run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/dev/snd:/dev/snd,/dev/dri:/dev/dri,/dev/input:/dev/input", // Some more if needed /run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/dev/snd:/dev/snd,/dev/dri:/dev/dri
	imagename:     "myrfswift:latest",
	repotag:       "penthertz/rfswift",
	extrahosts:    "",
	extraenv:      "",
	network_mode:  "host",
	exposed_ports: "",
	binded_ports:  "",
	pulse_server:  "tcp:localhost:34567",
	shell:         "/bin/bash"} // Instance with default values

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
	dockerObj.repotag = config.General.RepoTag
	dockerObj.shell = config.Container.Shell
	dockerObj.network_mode = config.Container.Network
	dockerObj.exposed_ports = config.Container.ExposedPorts
	dockerObj.binded_ports = config.Container.PortBindings
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
		{"Exposed Ports", props["ExposedPorts"]},
		{"Port Bindings", props["PortBindings"]},
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

func convertPortBindingsToString(portBindings nat.PortMap) string {
	var result []string

	for port, bindings := range portBindings {
		for _, binding := range bindings {
			// Format: HostIP:HostPort -> ContainerPort/Protocol
			entry := fmt.Sprintf("%s:%s -> %s", binding.HostIP, binding.HostPort, port)
			result = append(result, entry)
		}
	}

	return strings.Join(result, ", ")
}

func convertExposedPortsToString(exposedPorts nat.PortSet) string {
	var result []string

	// Iterate through the PortSet (a map where keys are the exposed ports)
	for port := range exposedPorts {
		result = append(result, string(port)) // Convert the nat.Port to string
	}

	return strings.Join(result, ", ")
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
		"XDisplay":     xdisplay,
		"Shell":        containerJSON.Path,
		"Privileged":   fmt.Sprintf("%v", containerJSON.HostConfig.Privileged),
		"NetworkMode":  string(containerJSON.HostConfig.NetworkMode),
		"ExposedPorts": convertExposedPortsToString(containerJSON.Config.ExposedPorts),
		"PortBindings": convertPortBindingsToString(containerJSON.HostConfig.PortBindings),
		"ImageName":    containerJSON.Config.Image,
		"ImageHash":    imageInfo.ID,
		"Bindings":     strings.Join(containerJSON.HostConfig.Binds, ","),
		"ExtraHosts":   strings.Join(containerJSON.HostConfig.ExtraHosts, ","),
		"Size":         imageSize,
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

func ParseExposedPorts(exposedPortsStr string) nat.PortSet {
	exposedPorts := nat.PortSet{}

	if exposedPortsStr == "" {
		return exposedPorts
	}

	// Split by commas to get individual ports
	portEntries := strings.Split(exposedPortsStr, ",")
	for _, entry := range portEntries {
		port := strings.TrimSpace(entry) // Remove extra spaces
		if port == "" {
			continue
		}

		// Add to nat.PortSet (e.g., "80/tcp")
		exposedPorts[nat.Port(port)] = struct{}{}
	}

	return exposedPorts
}

func ParseBindedPorts(bindedPortsStr string) nat.PortMap {
	portBindings := nat.PortMap{}

	if bindedPortsStr == "" || bindedPortsStr == "\"\"" {
		return portBindings
	}
	common.PrintSuccessMessage(fmt.Sprintf("Binded: '%s'", bindedPortsStr))

	// Split the input by ',' to get individual bindings
	portEntries := strings.Split(bindedPortsStr, ",")
	for _, entry := range portEntries {
		// Expected format: containerPort[:hostAddress:]hostPort/protocol (e.g., 80:127.0.0.1:8080/tcp)
		parts := strings.Split(entry, ":")
		if len(parts) < 2 || len(parts) > 3 {
			fmt.Printf("Invalid binded port format: %s (expected containerPort[:hostAddress:]hostPort/protocol)\n", entry)
			continue
		}

		var containerPortProto, hostPort, hostAddress string

		// Handle the optional hostAddress
		if len(parts) == 3 {
			containerPortProto = strings.TrimSpace(parts[0]) // e.g., 80
			hostAddress = strings.TrimSpace(parts[1])        // e.g., 127.0.0.1
			hostPort = strings.TrimSpace(parts[2])           // e.g., 8080/tcp
		} else {
			containerPortProto = strings.TrimSpace(parts[0]) // e.g., 80
			hostPort = strings.TrimSpace(parts[1])           // e.g., 8080/tcp
		}

		// Split hostPort into hostPort and protocol
		hostPortParts := strings.Split(hostPort, "/")
		if len(hostPortParts) != 2 {
			fmt.Printf("Invalid port format: %s (expected hostPort/protocol)\n", hostPort)
			continue
		}

		hostPortValue := strings.TrimSpace(hostPortParts[0]) // e.g., 8080
		protocol := strings.TrimSpace(hostPortParts[1])      // e.g., tcp

		// Rearrange to containerPort/protocol (e.g., 80/tcp)
		portKey := nat.Port(containerPortProto)

		// Add the binding to the PortMap
		portBindings[portKey] = append(portBindings[portKey], nat.PortBinding{
			HostIP:   hostAddress, // Optional host address
			HostPort: fmt.Sprintf("%s/%s", hostPortValue, protocol),
		})
	}

	return portBindings
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

	if !strings.Contains(dockerObj.imagename, ":") {
		// Prepend Config.General.RepoTag if the format is missing
		dockerObj.imagename = fmt.Sprintf("%s:%s", dockerObj.repotag, dockerObj.imagename)
	}

	bindings := combineBindings(dockerObj.x11forward, dockerObj.usbforward, dockerObj.extrabinding)
	extrahosts := splitAndCombine(dockerObj.extrahosts)
	dockerenv := combineEnv(dockerObj.xdisplay, dockerObj.pulse_server, dockerObj.extraenv)
	exposedPorts := ParseExposedPorts(dockerObj.exposed_ports)
	bindedPorts := ParseBindedPorts(dockerObj.binded_ports)

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:        dockerObj.imagename,
		Cmd:          []string{dockerObj.shell},
		Env:          dockerenv,
		ExposedPorts: exposedPorts,
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
		NetworkMode:  container.NetworkMode(dockerObj.network_mode),
		Binds:        bindings,
		Privileged:   true,
		ExtraHosts:   extrahosts,
		PortBindings: bindedPorts,
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

	if !strings.Contains(imageref, ":") {
		// Prepend Config.General.RepoTag if the format is missing
		imageref = fmt.Sprintf("%s:%s", dockerObj.repotag, imageref)
	}

	if imagetag == "" { // if tag is empty, keep same tag
		imagetag = imageref
	}

	common.PrintInfoMessage(fmt.Sprintf("Pulling image from: %s", imageref))

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
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Image '%s' installed successfully", imagetag))
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

func DockerInstallScript(containerIdentifier, scriptName, functionScript string) error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Check if the container is running; if not, start it
	containerJSON, err := cli.ContainerInspect(ctx, containerIdentifier)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %v", err)
	}

	if containerJSON.State.Status != "running" {
		if err := cli.ContainerStart(ctx, containerIdentifier, container.StartOptions{}); err != nil {
			return fmt.Errorf("failed to start container: %v", err)
		}
	}

	// Step 1: Run "apt update" with clock-based loading indicator
	common.PrintInfoMessage("Running 'apt update'...")
	if err := showLoadingIndicator(ctx, func() error {
		return execCommand(ctx, cli, containerIdentifier, []string{"/bin/bash", "-c", "apt update"})
	}, "apt update"); err != nil {
		return err
	}

	// Step 2: Run "apt --fix-broken install" with clock-based loading indicator
	common.PrintInfoMessage("Running 'apt --fix-broken install'...")
	if err := showLoadingIndicator(ctx, func() error {
		return execCommand(ctx, cli, containerIdentifier, []string{"/bin/bash", "-c", "apt --fix-broken install -y"})
	}, "apt --fix-broken install"); err != nil {
		return err
	}

	// Step 3: Run the provided script with clock-based loading indicator
	common.PrintInfoMessage(fmt.Sprintf("Running script './%s %s'...", scriptName, functionScript))
	if err := showLoadingIndicator(ctx, func() error {
		return execCommand(ctx, cli, containerIdentifier, []string{"/bin/bash", "-c", fmt.Sprintf("./%s %s", scriptName, functionScript)}, "/root/scripts")
	}, fmt.Sprintf("script './%s %s'", scriptName, functionScript)); err != nil {
		return err
	}

	return nil
}

// execCommand executes a command in the container, capturing only errors if any
func execCommand(ctx context.Context, cli *client.Client, containerID string, cmd []string, workingDir ...string) error {
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}

	// Optional working directory
	if len(workingDir) > 0 {
		execConfig.WorkingDir = workingDir[0]
	}

	execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec instance: %v", err)
	}

	attachResp, err := cli.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return fmt.Errorf("failed to attach to exec instance: %v", err)
	}
	defer attachResp.Close()

	// Capture only error messages, suppressing standard output
	_, err = io.Copy(io.Discard, attachResp.Reader)
	return err
}

// showLoadingIndicator displays a loading animation with a rotating clock icon while the command runs
func showLoadingIndicator(ctx context.Context, commandFunc func() error, stepName string) error {
	done := make(chan error)
	go func() {
		done <- commandFunc()
	}()

	// Clock emojis to create the rotating clock animation
	clockEmojis := []string{"🕛", "🕐", "🕑", "🕒", "🕓", "🕔", "🕕", "🕖", "🕗", "🕘", "🕙", "🕚"}
	i := 0
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			if err != nil {
				common.PrintErrorMessage(fmt.Errorf("Error during %s: %v", stepName, err))
				return err
			}
			fmt.Printf("\n")
			common.PrintSuccessMessage(fmt.Sprintf("%s completed", stepName))
			return nil
		case <-ticker.C:
			fmt.Printf("\r%s %s", clockEmojis[i%len(clockEmojis)], stepName)
			i++
		}
	}
}

func UpdateMountBinding(containerName string, source string, target string, add bool) {
	var timeout = 10 // Stop timeout

	// Check if the system is Windows
	if runtime.GOOS == "windows" {
		title := "Unsupported on Windows"
		message := `This function is not supported on Windows.
However, you can achieve similar functionality by using the following commands:
- "rfswift commit" to create a new image with a new tag.
- "rfswift remove" to remove the existing container.
- "rfswift run" to run a container with new bindings.`

		rfutils.DisplayNotification(title, message, "warning")
		os.Exit(1) // Exit since this function is not supported on Windows
	}

	if source == "" {
		source = target
		common.PrintWarningMessage(fmt.Sprintf("Source is empty. Defaulting source to target: %s", target))
	}

	ctx := context.Background()

	common.PrintInfoMessage("Fetching container ID...")
	containerID := getContainerIDByName(ctx, containerName)
	if containerID == "" {
		common.PrintErrorMessage(fmt.Errorf("container %s not found", containerName))
		os.Exit(1)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Container ID: %s", containerID))

	// Stop the container
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error when instantiating a client"))
		os.Exit(1)
	}
	common.PrintInfoMessage("Stopping the container...")

	// Attempt graceful stop
	if err := showLoadingIndicator(ctx, func() error {
		return cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	}, "Stopping the container..."); err != nil {
		common.PrintErrorMessage(fmt.Errorf("Failed to stop the container gracefully: %v", err))
		os.Exit(1)
	}

	// Check if the container is still running
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error inspecting container: %v", err))
		os.Exit(1)
	}
	if containerJSON.State.Running {
		common.PrintWarningMessage("Container is still running. Forcing stop...")
		err = cli.ContainerKill(ctx, containerID, "SIGKILL")
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("Failed to force stop the container: %v", err))
			os.Exit(1)
		}
		common.PrintSuccessMessage("Container forcibly stopped.")
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Container '%s' stopped", containerID))
	}

	// Load and update hostconfig.json
	common.PrintInfoMessage("Determining hostconfig.json path...")
	hostConfigPath, err := GetHostConfigPath(containerID)
	if err != nil {
		common.PrintErrorMessage(err)
		os.Exit(1)
	}
	common.PrintSuccessMessage(fmt.Sprintf("HostConfig path: %s", hostConfigPath))

	common.PrintInfoMessage("Loading hostconfig.json...")
	var hostConfig HostConfigFull
	if err := loadJSON(hostConfigPath, &hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to load hostconfig.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("HostConfig loaded successfully.")

	// Load and update config.v2.json
	common.PrintInfoMessage("Determining config.v2.json path...")
	configV2Path := strings.Replace(hostConfigPath, "hostconfig.json", "config.v2.json", 1)
	common.PrintInfoMessage(fmt.Sprintf("Loading config.v2.json from: %s", configV2Path))
	var configV2 map[string]interface{}
	if err := loadJSON(configV2Path, &configV2); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to load config.v2.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("config.v2.json loaded successfully.")

	// Update mounts in both files
	common.PrintInfoMessage("Updating mounts...")
	newMount := fmt.Sprintf("%s:%s", source, target)
	if add {
		if !ocontains(hostConfig.Binds, newMount) {
			hostConfig.Binds = append(hostConfig.Binds, newMount)
			addMountPoint(configV2, source, target)
			common.PrintSuccessMessage(fmt.Sprintf("Added mount: %s", newMount))
		} else {
			common.PrintWarningMessage("Mount already exists.")
		}
	} else {
		hostConfig.Binds = removeFromSlice(hostConfig.Binds, newMount)
		removeMountPoint(configV2, target)
		common.PrintSuccessMessage(fmt.Sprintf("Removed mount: %s", newMount))
	}

	// Save changes
	common.PrintInfoMessage("Saving updated hostconfig.json...")
	if err := saveJSON(hostConfigPath, hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to save hostconfig.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("hostconfig.json updated successfully.")

	common.PrintInfoMessage("Saving updated config.v2.json...")
	if err := saveJSON(configV2Path, configV2); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to save config.v2.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("config.v2.json updated successfully.")

	// Restart the container
	if err := showLoadingIndicator(ctx, func() error {
		return RestartDockerService()
	}, "Restarting Docker service..."); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to restart Docker service: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("Docker service restarted successfully.")
}

func addMountPoint(config map[string]interface{}, source string, target string) {
	mountPoints, ok := config["MountPoints"].(map[string]interface{})
	if !ok {
		mountPoints = make(map[string]interface{})
		config["MountPoints"] = mountPoints
	}

	mountPoints[target] = map[string]interface{}{
		"Source":      source,
		"Destination": target,
		"RW":          true,
		"Type":        "bind",
		"Propagation": "rprivate",
		"Spec": map[string]string{
			"Type":   "bind",
			"Source": source,
			"Target": target,
		},
		"SkipMountpointCreation": false,
	}
}

func removeMountPoint(config map[string]interface{}, target string) {
	mountPoints, ok := config["MountPoints"].(map[string]interface{})
	if !ok {
		return
	}

	delete(mountPoints, target)
}

func ocontains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func loadJSON(path string, v interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func saveJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

func removeFromSlice(slice []string, item string) []string {
	newSlice := []string{}
	for _, s := range slice {
		if s != item {
			newSlice = append(newSlice, s)
		}
	}
	return newSlice
}

func getContainerIDByName(ctx context.Context, containerName string) string {
	cli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
	for _, container := range containers {
		for _, name := range container.Names {
			if strings.TrimPrefix(name, "/") == containerName {
				return container.ID
			}
		}
	}
	return ""
}

func DockerStop(containerIdentifier string) {
	ctx := context.Background()

	// Initialize Docker client
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	// Retrieve the latest container if no identifier is provided
	if containerIdentifier == "" {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		containerIdentifier = latestDockerID(labelKey, labelValue)
		if containerIdentifier == "" {
			common.PrintErrorMessage(fmt.Errorf("no running containers found with label %s=%s", labelKey, labelValue))
			return
		}
	}

	// Inspect the container to get its current state
	containerJSON, err := cli.ContainerInspect(ctx, containerIdentifier)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return
	}

	containerName := strings.TrimPrefix(containerJSON.Name, "/")
	if !containerJSON.State.Running {
		common.PrintSuccessMessage(fmt.Sprintf("Container '%s' is already stopped", containerName))
		return
	}

	// Stop the container
	timeout := 10 // Grace period in seconds before force stop
	if err := cli.ContainerStop(ctx, containerIdentifier, container.StopOptions{Timeout: &timeout}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to stop container: %v", err))
		return
	}

	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' stopped successfully", containerName))
}
