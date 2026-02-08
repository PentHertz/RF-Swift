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
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
	"archive/tar"
	"path/filepath"
	"context"
	"gopkg.in/yaml.v3"
	"compress/gzip"
	"net/http"
	"strconv"
	
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
	devices       string
	caps          string
	seccomp       string
	cgroups       string
	ulimits       string
    realtime      bool
}

var dockerObj = DockerInst{net: "host",
	privileged:    false,
	xdisplay:      "DISPLAY=:0",
	entrypoint:    "/bin/bash",
	x11forward:    "/tmp/.X11-unix:/tmp/.X11-unix",
	usbforward:    "",
	extrabinding:  "/run/dbus/system_bus_socket:/run/dbus/system_bus_socket",
	imagename:     "myrfswift:latest",
	repotag:       "penthertz/rfswift_noble",
	extrahosts:    "",
	extraenv:      "",
	network_mode:  "host",
	exposed_ports: "",
	binded_ports:  "",
	pulse_server:  "tcp:localhost:34567",
	devices:       "/dev/snd:/dev/snd,/dev/dri:/dev/dri,/dev/input:/dev/input",
	caps:          "SYS_RAWIO,NET_ADMIN,SYS_TTY_CONFIG,SYS_ADMIN",
	seccomp:       "unconfined",
	cgroups:       "c *:* rwm", 
	ulimits:       "",
	realtime:      false,
	shell:         "/bin/bash",
}

// Recipe structures
type BuildRecipe struct {
	Name      string            `yaml:"name"`
	BaseImage string            `yaml:"base_image"`
	Tag       string            `yaml:"tag"`
	Context   string            `yaml:"context"`
	Labels    map[string]string `yaml:"labels"`
	Steps     []BuildStep       `yaml:"steps"`
}

type BuildStep struct {
	Type      string          `yaml:"type"`
	Commands  []string        `yaml:"commands"`
	Items     []CopyItem      `yaml:"items"`
	Path      string          `yaml:"path"`
	Name      string          `yaml:"name"`
	Script    string          `yaml:"script"`
	Functions []string        `yaml:"functions"`
	Paths     []string        `yaml:"paths"`
	AptClean  bool            `yaml:"apt_clean"`
}

type CopyItem struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
}

var loggingPID int
var loggingFile string
var loggingTool string

// setTerminalTitle sets the terminal window title
func setTerminalTitle(title string) {
	fmt.Printf("\033]0;%s\007", title)
}

// bindExistsByPrefix checks if a bind "src:dst" already exists in the slice,
// ignoring trailing mount options (e.g., ":rw,rprivate,nosuid,rbind").
func bindExistsByPrefix(binds []string, mount string) bool {
	for _, b := range binds {
		if b == mount || strings.HasPrefix(b, mount+":") {
			return true
		}
	}
	return false
}

// removeBindByPrefix removes binds matching "src:dst" regardless of trailing
// mount options appended by Podman.
func removeBindByPrefix(binds []string, mount string) []string {
	var result []string
	for _, b := range binds {
		if b == mount || strings.HasPrefix(b, mount+":") {
			continue
		}
		result = append(result, b)
	}
	return result
}

func init() {
	updateDockerObjFromConfig()
}

// DockerSetUlimits sets ulimits for the container
func DockerSetUlimits(ulimits string) {
	dockerObj.ulimits = ulimits
}

// DockerAddUlimit adds an ulimit to existing ulimits
func DockerAddUlimit(ulimit string) {
	if ulimit == "" {
		return
	}
	if dockerObj.ulimits == "" {
		dockerObj.ulimits = ulimit
	} else {
		dockerObj.ulimits = dockerObj.ulimits + "," + ulimit
	}
}

// DockerSetRealtime enables realtime mode (sets SYS_NICE cap + rtprio ulimit)
func DockerSetRealtime(enabled bool) {
	dockerObj.realtime = enabled
}

// parseUlimitsFromString parses ulimit string into Docker ulimit format
// Format: "name=soft:hard" or "name=value" (where soft=hard=value)
// Examples: "rtprio=95", "memlock=-1", "rtprio=95:95,memlock=-1:-1"
func parseUlimitsFromString(ulimitsStr string) []*container.Ulimit {
	var ulimits []*container.Ulimit

	if ulimitsStr == "" {
		return ulimits
	}

	entries := strings.Split(ulimitsStr, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Parse name=value or name=soft:hard
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			common.PrintWarningMessage(fmt.Sprintf("Invalid ulimit format: %s (expected name=value)", entry))
			continue
		}

		name := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		var soft, hard int64

		// Check if it's soft:hard format
		if strings.Contains(valueStr, ":") {
			valueParts := strings.Split(valueStr, ":")
			if len(valueParts) != 2 {
				common.PrintWarningMessage(fmt.Sprintf("Invalid ulimit value format: %s", valueStr))
				continue
			}

			var err error
			// Handle -1 for unlimited
			if valueParts[0] == "-1" || valueParts[0] == "unlimited" {
				soft = -1
			} else {
				soft, err = strconv.ParseInt(valueParts[0], 10, 64)
				if err != nil {
					common.PrintWarningMessage(fmt.Sprintf("Invalid soft limit: %s", valueParts[0]))
					continue
				}
			}

			if valueParts[1] == "-1" || valueParts[1] == "unlimited" {
				hard = -1
			} else {
				hard, err = strconv.ParseInt(valueParts[1], 10, 64)
				if err != nil {
					common.PrintWarningMessage(fmt.Sprintf("Invalid hard limit: %s", valueParts[1]))
					continue
				}
			}
		} else {
			// Single value: soft = hard
			var err error
			if valueStr == "-1" || valueStr == "unlimited" {
				soft = -1
				hard = -1
			} else {
				soft, err = strconv.ParseInt(valueStr, 10, 64)
				if err != nil {
					common.PrintWarningMessage(fmt.Sprintf("Invalid ulimit value: %s", valueStr))
					continue
				}
				hard = soft
			}
		}

		ulimits = append(ulimits, &container.Ulimit{
			Name: name,
			Soft: soft,
			Hard: hard,
		})
	}

	return ulimits
}

// convertUlimitsToString converts Docker ulimits to string format
func convertUlimitsToString(ulimits []*container.Ulimit) string {
	if len(ulimits) == 0 {
		return ""
	}

	var parts []string
	for _, ulimit := range ulimits {
		if ulimit.Soft == ulimit.Hard {
			if ulimit.Soft == -1 {
				parts = append(parts, fmt.Sprintf("%s=unlimited", ulimit.Name))
			} else {
				parts = append(parts, fmt.Sprintf("%s=%d", ulimit.Name, ulimit.Soft))
			}
		} else {
			softStr := fmt.Sprintf("%d", ulimit.Soft)
			hardStr := fmt.Sprintf("%d", ulimit.Hard)
			if ulimit.Soft == -1 {
				softStr = "unlimited"
			}
			if ulimit.Hard == -1 {
				hardStr = "unlimited"
			}
			parts = append(parts, fmt.Sprintf("%s=%s:%s", ulimit.Name, softStr, hardStr))
		}
	}
	return strings.Join(parts, ",")
}

// getRealtimeUlimits returns the ulimits needed for realtime SDR operations
func getRealtimeUlimits() []*container.Ulimit {
	return []*container.Ulimit{
		{
			Name: "rtprio",
			Soft: 95,
			Hard: 95,
		},
		{
			Name: "memlock",
			Soft: -1, // unlimited
			Hard: -1, // unlimited
		},
		{
			Name: "nice",
			Soft: 40, // allows nice values from -20 to 19
			Hard: 40,
		},
	}
}

// getUlimitsForContainer prepares ulimits for container creation
func getUlimitsForContainer() []*container.Ulimit {
	var ulimits []*container.Ulimit

	// If realtime mode is enabled, add realtime ulimits
	if dockerObj.realtime {
		ulimits = append(ulimits, getRealtimeUlimits()...)

		// Ensure SYS_NICE capability is added
		if !strings.Contains(dockerObj.caps, "SYS_NICE") {
			if dockerObj.caps == "" {
				dockerObj.caps = "SYS_NICE"
			} else {
				dockerObj.caps = dockerObj.caps + ",SYS_NICE"
			}
		}
		common.PrintInfoMessage("Realtime mode enabled: rtprio=95, memlock=unlimited, nice=40, SYS_NICE capability")
	}

	// Parse additional ulimits from string
	if dockerObj.ulimits != "" {
		customUlimits := parseUlimitsFromString(dockerObj.ulimits)

		// Merge: custom ulimits override realtime defaults
		for _, custom := range customUlimits {
			found := false
			for i, existing := range ulimits {
				if existing.Name == custom.Name {
					ulimits[i] = custom
					found = true
					break
				}
			}
			if !found {
				ulimits = append(ulimits, custom)
			}
		}
	}

	return ulimits
}

// UpdateUlimit adds or removes an ulimit from a container
func UpdateUlimit(containerID string, ulimitName string, ulimitValue string, add bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get container info first
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	// Parse existing ulimits
	existingUlimits := parseUlimitsFromString(props["Ulimits"])

	if add {
		// Build ulimit to add
		ulimitEntry := fmt.Sprintf("%s=%s", ulimitName, ulimitValue)

		// Check if already exists with same name
		found := false
		for i, ul := range existingUlimits {
			if ul.Name == ulimitName {
				// Update existing ulimit
				newUlimits := parseUlimitsFromString(ulimitEntry)
				if len(newUlimits) > 0 {
					existingUlimits[i] = newUlimits[0]
				}
				found = true
				common.PrintInfoMessage(fmt.Sprintf("Updating ulimit '%s' to '%s' on container '%s'", ulimitName, ulimitValue, containerName))
				break
			}
		}

		if !found {
			newUlimits := parseUlimitsFromString(ulimitEntry)
			existingUlimits = append(existingUlimits, newUlimits...)
			common.PrintInfoMessage(fmt.Sprintf("Adding ulimit '%s=%s' to container '%s'", ulimitName, ulimitValue, containerName))
		}
	} else {
		// Remove ulimit
		newUlimits := []*container.Ulimit{}
		found := false
		for _, ul := range existingUlimits {
			if ul.Name != ulimitName {
				newUlimits = append(newUlimits, ul)
			} else {
				found = true
			}
		}

		if !found {
			common.PrintWarningMessage(fmt.Sprintf("Ulimit '%s' not found in container '%s'", ulimitName, containerName))
			return nil
		}

		existingUlimits = newUlimits
		common.PrintInfoMessage(fmt.Sprintf("Removing ulimit '%s' from container '%s'", ulimitName, containerName))
	}

	// Update the container
	props["Ulimits"] = convertUlimitsToString(existingUlimits)

	return recreateContainerWithProperties(ctx, cli, containerID, props)
}

// EnableRealtimeMode enables realtime mode on an existing container
func EnableRealtimeMode(containerID string) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get container info
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	common.PrintInfoMessage(fmt.Sprintf("Enabling realtime mode on container '%s'", containerName))
	common.PrintInfoMessage("This will add: SYS_NICE capability, rtprio=95, memlock=unlimited, nice=40")

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	// Add SYS_NICE capability if not present
	caps := props["Caps"]
	if !strings.Contains(caps, "SYS_NICE") {
		if caps == "" {
			caps = "SYS_NICE"
		} else {
			caps = caps + ",SYS_NICE"
		}
		props["Caps"] = caps
	}

	// Parse existing ulimits and add realtime ones
	existingUlimits := parseUlimitsFromString(props["Ulimits"])
	realtimeUlimits := getRealtimeUlimits()

	// Merge ulimits (update existing or add new)
	for _, rtUlimit := range realtimeUlimits {
		found := false
		for i, existing := range existingUlimits {
			if existing.Name == rtUlimit.Name {
				existingUlimits[i] = rtUlimit
				found = true
				break
			}
		}
		if !found {
			existingUlimits = append(existingUlimits, rtUlimit)
		}
	}

	props["Ulimits"] = convertUlimitsToString(existingUlimits)

	err = recreateContainerWithProperties(ctx, cli, containerID, props)
	if err != nil {
		return err
	}

	common.PrintSuccessMessage("Realtime mode enabled successfully!")
	common.PrintInfoMessage("You can now use chrt and nice commands inside the container for SDR operations")
	common.PrintInfoMessage("Test with: ulimit -r (should show 95)")
	return nil
}

// DisableRealtimeMode disables realtime mode on an existing container
func DisableRealtimeMode(containerID string) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get container info
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	common.PrintInfoMessage(fmt.Sprintf("Disabling realtime mode on container '%s'", containerName))

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	// Remove SYS_NICE capability
	caps := strings.Split(props["Caps"], ",")
	newCaps := []string{}
	for _, cap := range caps {
		cap = strings.TrimSpace(cap)
		if cap != "SYS_NICE" && cap != "" {
			newCaps = append(newCaps, cap)
		}
	}
	props["Caps"] = strings.Join(newCaps, ",")

	// Remove realtime ulimits
	existingUlimits := parseUlimitsFromString(props["Ulimits"])
	realtimeNames := map[string]bool{"rtprio": true, "memlock": true, "nice": true}

	newUlimits := []*container.Ulimit{}
	for _, ul := range existingUlimits {
		if !realtimeNames[ul.Name] {
			newUlimits = append(newUlimits, ul)
		}
	}

	props["Ulimits"] = convertUlimitsToString(newUlimits)

	err = recreateContainerWithProperties(ctx, cli, containerID, props)
	if err != nil {
		return err
	}

	common.PrintSuccessMessage("Realtime mode disabled successfully!")
	return nil
}

// ListContainerUlimits displays the ulimits for a container
func ListContainerUlimits(containerID string) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get container info
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	// Get ulimits
	ulimits := containerJSON.HostConfig.Ulimits

	if len(ulimits) == 0 {
		common.PrintInfoMessage(fmt.Sprintf("Container '%s' has no custom ulimits set", containerName))
	} else {
		fmt.Printf("Ulimits for container '%s':\n", containerName)
		for _, ul := range ulimits {
			softStr := fmt.Sprintf("%d", ul.Soft)
			hardStr := fmt.Sprintf("%d", ul.Hard)
			if ul.Soft == -1 {
				softStr = "unlimited"
			}
			if ul.Hard == -1 {
				hardStr = "unlimited"
			}
			fmt.Printf("  • %s: soft=%s, hard=%s\n", ul.Name, softStr, hardStr)
		}
	}

	// Check if SYS_NICE capability is present
	hasSysNice := false
	for _, cap := range containerJSON.HostConfig.CapAdd {
		if cap == "SYS_NICE" {
			hasSysNice = true
			break
		}
	}

	// Check if realtime mode is effectively enabled
	hasRtprio := false
	hasMemlock := false
	for _, ul := range ulimits {
		if ul.Name == "rtprio" && ul.Soft > 0 {
			hasRtprio = true
		}
		if ul.Name == "memlock" && ul.Soft == -1 {
			hasMemlock = true
		}
	}

	fmt.Println()
	if hasSysNice && hasRtprio && hasMemlock {
		common.PrintSuccessMessage("Realtime mode: ENABLED")
	} else {
		common.PrintInfoMessage("Realtime mode: DISABLED")
		if !hasSysNice {
			common.PrintInfoMessage("  - Missing SYS_NICE capability")
		}
		if !hasRtprio {
			common.PrintInfoMessage("  - Missing rtprio ulimit")
		}
		if !hasMemlock {
			common.PrintInfoMessage("  - Missing memlock=unlimited ulimit")
		}
	}

	return nil
}

// formatVersionsMultiLine formats versions into multiple lines with a max per line
func formatVersionsMultiLine(versions []string, maxPerLine int, maxWidth int) []string {
	if len(versions) == 0 {
		return []string{"-"}
	}

	var lines []string
	var currentLine strings.Builder
	countOnLine := 0

	for i, v := range versions {
		// Check if adding this version would exceed width or count
		separator := ""
		if countOnLine > 0 {
			separator = ", "
		}
		
		testLen := currentLine.Len() + len(separator) + len(v)
		
		if countOnLine >= maxPerLine || (maxWidth > 0 && testLen > maxWidth) {
			// Start new line
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
			}
			currentLine.Reset()
			countOnLine = 0
			separator = ""
		}

		if countOnLine > 0 {
			currentLine.WriteString(", ")
		}
		currentLine.WriteString(v)
		countOnLine++

		// Last item
		if i == len(versions)-1 && currentLine.Len() > 0 {
			lines = append(lines, currentLine.String())
		}
	}

	if len(lines) == 0 {
		return []string{"-"}
	}

	return lines
}

// printTableWithMultiLineSupport prints a table where cells can have multiple lines
func printTableWithMultiLineSupport(headers []string, rows [][]interface{}, columnWidths []int, title string, titleColor string) {
	white := "\033[37m"
	reset := "\033[0m"

	// Calculate total width
	totalWidth := 1
	for _, w := range columnWidths {
		totalWidth += w + 3
	}

	// Print title
	fmt.Printf("%s%s%s%s%s\n", titleColor, strings.Repeat(" ", 2), title, strings.Repeat(" ", totalWidth-2-len(title)), reset)
	fmt.Print(white)

	// Print top border
	printHorizontalBorder(columnWidths, "┌", "┬", "┐")

	// Print headers
	headerStrings := make([]string, len(headers))
	for i, h := range headers {
		headerStrings[i] = h
	}
	printRow(headerStrings, columnWidths, "│")
	printHorizontalBorder(columnWidths, "├", "┼", "┤")

	// Print rows with multi-line support
	for rowIdx, row := range rows {
		// Convert row to string slices (each cell can be []string for multi-line)
		cellLines := make([][]string, len(row))
		maxLines := 1

		for colIdx, cell := range row {
			switch v := cell.(type) {
			case string:
				cellLines[colIdx] = []string{v}
			case []string:
				if len(v) == 0 {
					cellLines[colIdx] = []string{""}
				} else {
					cellLines[colIdx] = v
				}
			default:
				cellLines[colIdx] = []string{fmt.Sprintf("%v", v)}
			}
			if len(cellLines[colIdx]) > maxLines {
				maxLines = len(cellLines[colIdx])
			}
		}

		// Print each line of this row
		for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
			fmt.Print("│")
			for colIdx, lines := range cellLines {
				content := ""
				if lineIdx < len(lines) {
					content = lines[lineIdx]
				}
				
				// Apply color for specific columns (status, version)
				color := getColumnColor(colIdx, content, len(row))
				
				if color != "" {
					fmt.Printf(" %s%-*s%s ", color, columnWidths[colIdx], truncateString(content, columnWidths[colIdx]), reset)
				} else {
					fmt.Printf(" %-*s ", columnWidths[colIdx], truncateString(content, columnWidths[colIdx]))
				}
				fmt.Print("│")
			}
			fmt.Println()
		}

		// Print row separator (except for last row)
		if rowIdx < len(rows)-1 {
			printHorizontalBorder(columnWidths, "├", "┼", "┤")
		}
	}

	// Print bottom border
	printHorizontalBorder(columnWidths, "└", "┴", "┘")
	fmt.Print(reset)
	fmt.Println()
}

// getColumnColor returns the color for a specific column value
func getColumnColor(colIdx int, content string, totalCols int) string {
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	cyan := "\033[36m"

	// Status column (usually second to last or specific index)
	statusKeywords := map[string]string{
		"Up to date": green,
		"Obsolete":   red,
		"Custom":     yellow,
		"No network": yellow,
		"Error":      red,
	}

	if color, ok := statusKeywords[content]; ok {
		return color
	}

	// Version column - if it starts with "v" or contains version-like pattern
	if strings.HasPrefix(content, "v") || strings.Contains(content, ".") {
		// Check if it looks like a version
		if len(content) > 0 && content != "-" {
			return cyan
		}
	}

	return ""
}

// normalizeImageName ensures image has proper repo:tag format
func normalizeImageName(imageName string) string {
	if imageName == "" {
		return imageName
	}
	if !strings.Contains(imageName, ":") {
		normalized := fmt.Sprintf("%s:%s", dockerObj.repotag, imageName)
		common.PrintInfoMessage(fmt.Sprintf("Using full image reference: %s", normalized))
		return normalized
	}
	return imageName
}


// getRemoteImageDigest fetches the digest for a specific tag from Docker Hub
func getRemoteImageDigest(repo, tag, architecture string) (string, error) {
    var digest string
    
    // Normalize tag to include architecture suffix
    normalizedTag := normalizeTagForRemote(tag, architecture)
    
    err := showLoadingIndicatorWithReturn(func() error {
        url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/?page_size=100", repo)
        client := &http.Client{Timeout: 10 * time.Second}
        resp, err := client.Get(url)
        if err != nil {
            return err
        }
        defer resp.Body.Close()

        if resp.StatusCode == http.StatusNotFound {
            return fmt.Errorf("tag not found")
        } else if resp.StatusCode != http.StatusOK {
            return fmt.Errorf("failed to get tags: %s", resp.Status)
        }

        body, err := io.ReadAll(resp.Body)
        if err != nil {
            return err
        }

        var response DockerHubResponse
        if err := json.Unmarshal(body, &response); err != nil {
            return err
        }

        for _, hubTag := range response.Results {
            if hubTag.Name == normalizedTag {
                if strings.HasPrefix(hubTag.Name, "cache_") {
                    continue
                }
                if hubTag.MediaType != "application/vnd.oci.image.index.v1+json" {
                    continue
                }
                
                digest = hubTag.Digest
                return nil
            }
        }

        return fmt.Errorf("tag not found")
    }, fmt.Sprintf("Checking Docker Hub for '%s' (%s)", tag, architecture))

    return digest, err
}

func updateDockerObjFromConfigFixed() {
	config, err := rfutils.ReadOrCreateConfig(common.ConfigFileByPlatform())
	if err != nil {
		log.Printf("Error reading config: %v. Using default values.", err)
		return
	}

	// Update dockerObj with values from config — only override if non-empty
	dockerObj.imagename = config.General.ImageName
	dockerObj.repotag = config.General.RepoTag
	dockerObj.shell = config.Container.Shell
	dockerObj.network_mode = config.Container.Network
	dockerObj.exposed_ports = config.Container.ExposedPorts
	dockerObj.binded_ports = config.Container.PortBindings
	dockerObj.xdisplay = config.Container.XDisplay
	dockerObj.extrahosts = config.Container.ExtraHost
	dockerObj.extraenv = config.Container.ExtraEnv
	dockerObj.devices = config.Container.Devices
	dockerObj.pulse_server = config.Audio.PulseServer
	dockerObj.privileged = strings.ToLower(config.Container.Privileged) == "true"
	dockerObj.caps = config.Container.Caps
	dockerObj.seccomp = config.Container.Seccomp

	// Only override cgroups if the config file actually specifies something.
	// This preserves the built-in default "c *:* rwm" when config is empty.
	if config.Container.Cgroups != "" {
		dockerObj.cgroups = config.Container.Cgroups
	}

	// Handle bindings - include ALL bindings from config
	var bindings []string
	var x11Bindings []string

	for _, binding := range config.Container.Bindings {
		if strings.Contains(binding, ".X11-unix") {
			x11Bindings = append(x11Bindings, binding)
		} else if strings.Contains(binding, "/dev/bus/usb") {
			dockerObj.usbforward = binding
			bindings = append(bindings, binding)
		} else {
			bindings = append(bindings, binding)
		}
	}

	if len(x11Bindings) > 0 {
		dockerObj.x11forward = strings.Join(x11Bindings, ",")
	}

	dockerObj.extrabinding = strings.Join(bindings, ",")
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
	dockerObj.xdisplay = config.Container.XDisplay
	dockerObj.extrahosts = config.Container.ExtraHost
	dockerObj.extraenv = config.Container.ExtraEnv
	dockerObj.devices = config.Container.Devices
	dockerObj.pulse_server = config.Audio.PulseServer
	dockerObj.privileged = strings.ToLower(config.Container.Privileged) == "true"
	dockerObj.caps = config.Container.Caps
	dockerObj.seccomp = config.Container.Seccomp
	if config.Container.Cgroups != "" {
	    dockerObj.cgroups = config.Container.Cgroups
	}

	// Handle bindings - include ALL bindings from config
	var bindings []string
	var x11Bindings []string

	for _, binding := range config.Container.Bindings {
		if strings.Contains(binding, ".X11-unix") {
			x11Bindings = append(x11Bindings, binding)
		} else if strings.Contains(binding, "/dev/bus/usb") {
			dockerObj.usbforward = binding
			bindings = append(bindings, binding) // Include USB in bindings too
		} else {
			bindings = append(bindings, binding)
		}
	}

	// Set X11 forward
	if len(x11Bindings) > 0 {
		dockerObj.x11forward = strings.Join(x11Bindings, ",")
	}

	// Set extra bindings
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
	// Podman uses fully qualified names (docker.io/penthertz/...)
	imageName = strings.TrimPrefix(imageName, "docker.io/")
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

func digestMatches(localDigests []string, remoteDigest string) bool {
	for _, d := range localDigests {
		if d == remoteDigest {
			return true
		}
	}
	return false
}

func checkImageStatusWithCache(ctx context.Context, cli *client.Client, repo, tag string, architecture string, cachedVersionsByRepo RepoVersionMap) (bool, bool, error) {
	if common.Disconnected {
		return false, true, nil
	}

	// Check if this is an official image
	fullImageName := fmt.Sprintf("%s:%s", repo, tag)
	if !IsOfficialImage(fullImageName) {
		return false, true, nil // Custom image
	}

	// Get local image info
	localImage, _, err := cli.ImageInspectWithRaw(ctx, fullImageName)
	if err != nil {
		return false, true, err
	}

	// Get local digests (Podman may store multiple)
	var localDigests []string
	for _, repoDigest := range localImage.RepoDigests {
		if idx := strings.Index(repoDigest, "@"); idx != -1 {
			localDigests = append(localDigests, repoDigest[idx+1:])
		}
	}

	// Parse tag to check if it's a versioned tag (e.g., reversing_0.0.7)
	baseName, version := parseTagVersion(tag)

	// Get versions for THIS SPECIFIC REPO
	repoVersions, ok := cachedVersionsByRepo[repo]
	if !ok || len(repoVersions) == 0 {
		// Repo not found in cache - likely custom image
		return false, true, nil
	}

	versions, ok := repoVersions[baseName]
	if !ok || len(versions) == 0 {
		// Base name not found in this repo - likely custom image
		return false, true, nil
	}

	// Find the latest non-"latest" version (highest semver) for THIS REPO
	var latestVersion string
	var latestVersionDigest string
	for _, v := range versions {
		if v.Version != "latest" {
			if latestVersion == "" {
				// First non-latest version is the highest due to sorting
				latestVersion = v.Version
				latestVersionDigest = v.Digest
			}
			break
		}
	}

	// If it's a versioned tag (e.g., reversing_0.0.7)
	if version != "" {
		// Check if this version is the latest for this repo
		if latestVersion != "" && compareVersions(version, latestVersion) < 0 {
			// There's a newer version available in this repo = Obsolete
			return false, false, nil
		}

		// This is the latest version (or equal), check if digest matches
		for _, v := range versions {
			if v.Version == version {
				if len(localDigests) > 0 && digestMatches(localDigests, v.Digest) {
					return true, false, nil // Up-to-date
				}
				// Same version but different digest = Obsolete (rebuilt)
				return false, false, nil
			}
		}

		// Version not found in remote - custom local version
		return false, true, nil
	}

	// For non-versioned tags (like "sdr_light", "reversing"):
	// Find which version the local image matches by digest
	var matchedVersion string
	for _, v := range versions {
		if digestMatches(localDigests, v.Digest) {
			if v.Version != "latest" {
				matchedVersion = v.Version
			}
			break
		}
	}

	// If we found a matching version, check if it's the latest for this repo
	if matchedVersion != "" {
		if latestVersion != "" && compareVersions(matchedVersion, latestVersion) < 0 {
			// There's a newer version in this repo = Obsolete
			return false, false, nil
		}
		// Matched version is the latest for this repo = Up to date
		return true, false, nil
	}

	// No version match found by digest - compare with latest digest directly
	if latestVersionDigest != "" && len(localDigests) > 0 {
		if digestMatches(localDigests, latestVersionDigest) {
			return true, false, nil // Up-to-date
		}
		return false, false, nil // Obsolete
	}

	// Fallback: check "latest" tag digest
	for _, v := range versions {
		if v.Version == "latest" {
			if len(localDigests) > 0 && digestMatches(localDigests, v.Digest) {
				return true, false, nil
			}
			return false, false, nil
		}
	}

	// Could not determine - assume custom
	return false, true, nil
}

func checkImageStatus(ctx context.Context, cli *client.Client, repo, tag string) (bool, bool, error) {
	if common.Disconnected {
		return false, true, nil
	}
	architecture := getArchitecture()

	// Check if this is an official image
	fullImageName := fmt.Sprintf("%s:%s", repo, tag)
	if !IsOfficialImage(fullImageName) {
		return false, true, nil // Custom image
	}

	// Fetch versions by repo
	cachedVersionsByRepo := GetAllRemoteVersionsByRepo(architecture)

	return checkImageStatusWithCache(ctx, cli, repo, tag, architecture, cachedVersionsByRepo)
}

// getLocalImageDigest gets the digest for a local image
func getLocalImageDigest(ctx context.Context, cli *client.Client, imageName string) string {
	imageInspect, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return ""
	}

	for _, repoDigest := range imageInspect.RepoDigests {
		if idx := strings.Index(repoDigest, "@"); idx != -1 {
			return repoDigest[idx+1:]
		}
	}

	return ""
}

func getLocalImageDigests(ctx context.Context, cli *client.Client, imageName string) []string {
	imageInspect, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return nil
	}
	var digests []string
	for _, repoDigest := range imageInspect.RepoDigests {
		if idx := strings.Index(repoDigest, "@"); idx != -1 {
			digests = append(digests, repoDigest[idx+1:])
		}
	}
	return digests
}

// Modified printContainerProperties function for dock/dock.go
// This version displays the image version next to the image name in the container summary

func printContainerProperties(ctx context.Context, cli *client.Client, containerName string, props map[string]string, size string) {
	white := "\033[37m"
	blue := "\033[34m"
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	cyan := "\033[36m"
	reset := "\033[0m"

	// Determine if the image is up-to-date, obsolete, or custom
	repo, tag := parseImageName(props["ImageName"])
	isUpToDate, isCustom, err := checkImageStatus(ctx, cli, repo, tag)
	if err != nil {
		if err.Error() != "tag not found" {
			log.Printf("Error checking image status: %v", err)
		}
	}

	// Try to detect the version
	versionDisplay := ""
	architecture := getArchitecture()
	
	// First check if the tag already contains a version
	baseName, existingVersion := parseTagVersion(tag)
	if existingVersion != "" {
		versionDisplay = existingVersion
	} else if !common.Disconnected {
		// Try to find version by matching digest with remote versions
		fullImageName := fmt.Sprintf("%s:%s", repo, tag)
		localDigest := getLocalImageDigests(ctx, cli, fullImageName)
		if len(localDigest) > 0 {
			remoteVersionsByRepo := GetAllRemoteVersionsByRepo(architecture)
			if repoVersions, ok := remoteVersionsByRepo[repo]; ok {
				if versions, ok := repoVersions[baseName]; ok {
					matchedVersion := GetVersionForDigests(versions, localDigest)
					if matchedVersion != "" && matchedVersion != "latest" {
						versionDisplay = matchedVersion
					}
				}
			}
		}
	}

	// Build image status string with version if available
	imageNameWithVersion := props["ImageName"]
	if versionDisplay != "" {
		imageNameWithVersion = fmt.Sprintf("%s %sv%s%s", props["ImageName"], cyan, versionDisplay, reset)
	}

	imageStatus := fmt.Sprintf("%s (Custom)", imageNameWithVersion)
	if common.Disconnected {
		imageStatus = fmt.Sprintf("%s (No network)", imageNameWithVersion)
	}
	imageStatusColor := yellow
	if !isCustom {
		if isUpToDate {
			imageStatus = fmt.Sprintf("%s (Up to date)", imageNameWithVersion)
			imageStatusColor = green
		} else {
			imageStatus = fmt.Sprintf("%s (Obsolete)", imageNameWithVersion)
			imageStatusColor = red
		}
	}

	seccompValue := props["Seccomp"]
	if seccompValue == "" {
		seccompValue = "(Default)"
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
		{"Devices", props["Devices"]},
		{"Capabilities", props["Caps"]},
		{"Seccomp profile", seccompValue},
		{"Cgroup rules", props["Cgroups"]},
		{"Ulimits", props["Ulimits"]},
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
	cli, err := NewEngineClient()
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	// Set up container filters for labels only
	// We'll handle image ancestor and name/ID filtering manually
	containerFilters := filters.NewArgs()
	if labelKey != "" && labelValue != "" {
		containerFilters.Add("label", fmt.Sprintf("%s=%s", labelKey, labelValue))
	}

	// Get container list
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Limit:   15,
		Filters: containerFilters,
	})
	if err != nil {
		panic(err)
	}

	// Create maps to store image mappings
	imageIDToNames := make(map[string][]string)
	hashToNames := make(map[string][]string)

	// Get all images to build a mapping of image IDs to all their tags
	images, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		panic(err)
	}

	// Build image ID to names mapping
	for _, img := range images {
		shortID := img.ID[7:19] // Get a shortened version of the SHA256 hash
		fullHash := img.ID[7:]  // Remove "sha256:" prefix but keep full hash

		// Store mappings if image has tags
		if len(img.RepoTags) > 0 {
			imageIDToNames[img.ID] = img.RepoTags
			imageIDToNames[shortID] = img.RepoTags
			hashToNames[fullHash] = img.RepoTags
		}
	}

	//rfutils.ClearScreen()
	tableData := [][]string{}

	// Filter containers by image, name or ID (if ifilter is provided)
	filteredContainers := []types.Container{}

	if ifilter != "" {
		lowerFilter := strings.ToLower(ifilter)
		for _, container := range containers {
			// Check if image name contains the filter (original behavior)
			if strings.Contains(strings.ToLower(container.Image), lowerFilter) {
				filteredContainers = append(filteredContainers, container)
				continue
			}

			// Check if container ID (full or short) contains the filter
			if strings.Contains(strings.ToLower(container.ID), lowerFilter) ||
				strings.Contains(strings.ToLower(container.ID[:12]), lowerFilter) {
				filteredContainers = append(filteredContainers, container)
				continue
			}

			// Check if any container name contains the filter
			for _, name := range container.Names {
				// Remove leading slash from name if it exists
				cleanName := name
				if len(name) > 0 && name[0] == '/' {
					cleanName = name[1:]
				}

				if strings.Contains(strings.ToLower(cleanName), lowerFilter) {
					filteredContainers = append(filteredContainers, container)
					break
				}
			}
		}
	} else {
		filteredContainers = containers
	}

	for _, container := range filteredContainers {
		created := time.Unix(container.Created, 0).Format(time.RFC3339)

		// Get the container image ID and associate with tags
		containerImageID := container.ImageID
		shortImageID := ""
		if len(containerImageID) >= 19 {
			shortImageID = containerImageID[7:19] // shortened SHA256
		} else if len(containerImageID) > 7 {
			shortImageID = containerImageID[7:] // Use whatever is available after "sha256:"
		} else {
			shortImageID = containerImageID // Use as-is if too short
		}

		// Get the display image name
		imageTag := container.Image

		// Check if this is a SHA256 hash
		isSHA256 := strings.HasPrefix(imageTag, "sha256:")

		// If this is a SHA256 hash, try to find a friendly name
		if isSHA256 {
			hashPart := imageTag[7:] // Remove "sha256:" prefix
			// Check if we have a friendly name for this hash
			if tags, ok := hashToNames[hashPart]; ok && len(tags) > 0 {
				imageTag = tags[0] // Use the first tag
			} else if tags, ok := imageIDToNames[containerImageID]; ok && len(tags) > 0 {
				imageTag = tags[0] // Fallback to container image ID mapping
			}
		}

		// Check if this is a renamed image (date format: -DDMMYYYY)
		isRenamed := false
		if len(imageTag) > 9 { // Make sure string is long enough before checking suffix
			suffix := imageTag[len(imageTag)-9:]
			if len(suffix) > 0 && suffix[0] == '-' {
				// Check if the rest is a date format
				datePattern := true
				for i := 1; i < 9; i++ {
					if i < 9 && (suffix[i] < '0' || suffix[i] > '9') {
						datePattern = false
						break
					}
				}
				isRenamed = datePattern
			}
		}

		// Prepare the display string
		imageDisplay := imageTag

		// For SHA256 or renamed images, show hash for clarity
		if isSHA256 || isRenamed {
			imageDisplay = fmt.Sprintf("%s (%s)", imageTag, shortImageID)
		}

		containerName := ""
		if len(container.Names) > 0 {
		    containerName = container.Names[0]
		    if len(containerName) > 0 && containerName[0] == '/' {
		        containerName = containerName[1:]
		    }
		} else {
		    containerName = container.ID[:12] // fallback to short ID
		}
		containerID := container.ID[:12]
		command := container.Command

		// Truncate command if too long
		if len(command) > 30 {
			command = command[:27] + "..."
		}

		tableData = append(tableData, []string{
			created,
			imageDisplay,
			containerName,
			containerID,
			command,
		})
	}

	headers := []string{"Created", "Image Tag (ID)", "Container Name", "Container ID", "Command"}
	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80
	}

	// Calculate column widths
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

	// Adjust column widths to fit terminal
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

	// Print fancy table
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

func latestDockerID(labelKey string, labelValue string) string {
	/* Get latest Docker container ID by image label
	   in(1): string label key
	   in(2): string label value
	   out: string container ID
	*/
	ctx := context.Background()
	cli, err := NewEngineClient()
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

func convertPortBindingsToRoundTrip(portBindings nat.PortMap) string {
    var result []string
    for port, bindings := range portBindings {
        for _, binding := range bindings {
            // Format: containerPort/protocol:hostAddress:hostPort
            // or containerPort/protocol:hostPort
            if binding.HostIP != "" && binding.HostIP != "0.0.0.0" {
                result = append(result, fmt.Sprintf("%s:%s:%s", port, binding.HostIP, binding.HostPort))
            } else {
                result = append(result, fmt.Sprintf("%s:%s", port, binding.HostPort))
            }
        }
    }
    return strings.Join(result, ";;")
}

func convertExposedPortsToString(exposedPorts nat.PortSet) string {
	var result []string

	// Iterate through the PortSet (a map where keys are the exposed ports)
	for port := range exposedPorts {
		result = append(result, string(port)) // Convert the nat.Port to string
	}

	return strings.Join(result, ", ")
}

func convertDevicesToString(devices []container.DeviceMapping) string {
	deviceStrings := make([]string, len(devices))
	for i, device := range devices {
		deviceStrings[i] = fmt.Sprintf("%s:%s", device.PathOnHost, device.PathInContainer)
	}
	return strings.Join(deviceStrings, ",")
}

func convertCapsToString(caps []string) string {
	if len(caps) == 0 {
		return ""
	}
	return strings.Join(caps, ",")
}

func convertSecurityOptToString(securityOpts []string) string {
	if len(securityOpts) == 0 {
		return ""
	}

	// Look specifically for seccomp profile
	for _, opt := range securityOpts {
		if strings.HasPrefix(opt, "seccomp=") {
			// Extract just the profile value after "seccomp="
			return strings.TrimPrefix(opt, "seccomp=")
		}
	}

	// If no seccomp profile found, return empty string or join all options
	return ""
}

// getDisplayImageName returns the user-facing image name.
// After container recreation, Config.Image is the committed temp image ID.
// We store the original name in a label so the summary looks clean.
func getDisplayImageName(containerJSON types.ContainerJSON) string {
	// Check for original image label first (set during recreation)
	if label, ok := containerJSON.Config.Labels["org.rfswift.original_image"]; ok && label != "" {
		return label
	}
	return containerJSON.Config.Image
}

func getExposedPortsFromLabel(containerJSON types.ContainerJSON) string {
	if label, ok := containerJSON.Config.Labels["org.rfswift.exposed_ports"]; ok {
		if label == "none" {
			return ""
		}
		return label
	}
	return convertExposedPortsToString(containerJSON.Config.ExposedPorts)
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

	cgroupRules := strings.Join(containerJSON.HostConfig.DeviceCgroupRules, ",")
	if cgroupRules == "" {
		if label, ok := containerJSON.Config.Labels["org.rfswift.cgroup_rules"]; ok {
			cgroupRules = label
		}
	}

	props := map[string]string{
		"XDisplay":     xdisplay,
		"Shell":        containerJSON.Path,
		"Privileged":   fmt.Sprintf("%v", containerJSON.HostConfig.Privileged),
		"NetworkMode":  string(containerJSON.HostConfig.NetworkMode),
		"ExposedPorts": getExposedPortsFromLabel(containerJSON),
		"PortBindings": convertPortBindingsToRoundTrip(containerJSON.HostConfig.PortBindings),
		"ImageName":    getDisplayImageName(containerJSON),
		"ImageHash":    imageInfo.ID,
		"Bindings":     strings.Join(containerJSON.HostConfig.Binds, ";;"),
		"ExtraHosts":   strings.Join(containerJSON.HostConfig.ExtraHosts, ","),
		"Size":         imageSize,
		"Devices":      convertDevicesToString(containerJSON.HostConfig.Devices),
		"Caps":         convertCapsToString(containerJSON.HostConfig.CapAdd),
		"Seccomp":      convertSecurityOptToString(containerJSON.HostConfig.SecurityOpt),
		"Cgroups":      cgroupRules,
	}

	// Get ulimits
    var ulimitStrs []string
    for _, ulimit := range containerJSON.HostConfig.Ulimits {
        if ulimit.Soft == ulimit.Hard {
            if ulimit.Soft == -1 {
                ulimitStrs = append(ulimitStrs, fmt.Sprintf("%s=unlimited", ulimit.Name))
            } else {
                ulimitStrs = append(ulimitStrs, fmt.Sprintf("%s=%d", ulimit.Name, ulimit.Soft))
            }
        } else {
            softStr := fmt.Sprintf("%d", ulimit.Soft)
            hardStr := fmt.Sprintf("%d", ulimit.Hard)
            if ulimit.Soft == -1 {
                softStr = "unlimited"
            }
            if ulimit.Hard == -1 {
                hardStr = "unlimited"
            }
            ulimitStrs = append(ulimitStrs, fmt.Sprintf("%s=%s:%s", ulimit.Name, softStr, hardStr))
        }
    }
    props["Ulimits"] = strings.Join(ulimitStrs, ",")

	return props, nil
}

func DockerExec(containerIdentifier string, WorkingDir string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
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

	// Determine shell to use:
	// Priority: 1) explicitly set via CLI (-e flag) if different from default
	//           2) container's original shell (from containerJSON.Path)
	//           3) fallback to /bin/bash
	shellToUse := dockerObj.shell

	// If shell is empty or default, prefer container's configured shell
	if shellToUse == "" || shellToUse == "/bin/bash" {
		containerShell := containerJSON.Path
		if containerShell != "" {
			shellToUse = containerShell
		}
	}

	// Final fallback
	if shellToUse == "" {
		shellToUse = "/bin/bash"
	}

	// Create exec configuration
	execConfig := container.ExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{shellToUse},
		WorkingDir:   WorkingDir,
	}

	// Create exec instance
	execID, err := cli.ContainerExecCreate(ctx, containerIdentifier, execConfig)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to create exec instance: %v", err))
		return
	}

	// Attach to the exec instance
	attachResp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{Tty: true})
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
	// NOTE: Podman's compat API implicitly starts the exec session during Attach,
	// so calling ExecStart again causes "exec session state improper". Skip it.
	if GetEngine().Type() != EnginePodman {
		if err := cli.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{Tty: true}); err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to start exec instance: %v", err))
			return
		}
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

	// Support both ";;" (internal round-trip) and "," (CLI input) delimiters
	var portEntries []string
	if strings.Contains(bindedPortsStr, ";;") {
		portEntries = strings.Split(bindedPortsStr, ";;")
	} else {
		portEntries = strings.Split(bindedPortsStr, ",")
	}
	for _, entry := range portEntries {
		// Expected format: containerPort/protocol:hostPort or containerPort/protocol:hostAddress:hostPort
		// Example: 80/tcp:8080 or 80/tcp:127.0.0.1:8080
		entry = strings.TrimSpace(entry)
		parts := strings.Split(entry, ":")
		if len(parts) < 2 || len(parts) > 3 {
			fmt.Printf("Invalid binded port format: %s (expected containerPort/protocol:hostPort or containerPort/protocol:hostAddress:hostPort)\n", entry)
			continue
		}

		var containerPortProto, hostPort, hostAddress string

		// Parse containerPort/protocol (e.g., "80/tcp")
		containerPortProto = strings.TrimSpace(parts[0])

		// Validate that containerPortProto contains a protocol
		if !strings.Contains(containerPortProto, "/") {
			fmt.Printf("Invalid container port format: %s (expected format: port/protocol, e.g., 80/tcp)\n", containerPortProto)
			continue
		}

		// Handle the optional hostAddress
		if len(parts) == 3 {
			hostAddress = strings.TrimSpace(parts[1]) // e.g., 127.0.0.1
			hostPort = strings.TrimSpace(parts[2])    // e.g., 8080
		} else {
			hostAddress = ""                       // No specific host address (binds to all interfaces)
			hostPort = strings.TrimSpace(parts[1]) // e.g., 8080
		}

		// containerPortProto is already in format "80/tcp"
		portKey := nat.Port(containerPortProto)

		// Add the binding to the PortMap
		// HostPort should be JUST the port number, not including protocol
		portBindings[portKey] = append(portBindings[portKey], nat.PortBinding{
			HostIP:   hostAddress, // Optional host address (empty means 0.0.0.0)
			HostPort: hostPort,    // Just the port number, NO protocol
		})
	}

	return portBindings
}

func getDeviceMappingsFromString(devicesStr string) []container.DeviceMapping {
	var devices []container.DeviceMapping

	if devicesStr == "" {
		return devices
	}

	devicesList := strings.Split(devicesStr, ",")
	for _, deviceMapping := range devicesList {
		parts := strings.Split(deviceMapping, ":")
		if len(parts) == 2 {
			devices = append(devices, container.DeviceMapping{
				PathOnHost:        parts[0],
				PathInContainer:   parts[1],
				CgroupPermissions: "rwm",
			})
		}
	}

	return devices
}

func DockerRun(containerName string) {
	/*
	 *   Create a container with a specific name and run it
	 */
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	dockerObj.imagename = normalizeImageName(dockerObj.imagename)

	bindings := combineBindings(dockerObj.x11forward, dockerObj.extrabinding)
	extrahosts := splitAndCombine(dockerObj.extrahosts)
	dockerenv := combineEnv(dockerObj.xdisplay, dockerObj.pulse_server, dockerObj.extraenv)
	exposedPorts := ParseExposedPorts(dockerObj.exposed_ports)
	bindedPorts := ParseBindedPorts(dockerObj.binded_ports)

	// Prepare host config based on privileged flag
	hostConfig := &container.HostConfig{
		NetworkMode:  container.NetworkMode(dockerObj.network_mode),
		Binds:        bindings,
		ExtraHosts:   extrahosts,
		PortBindings: bindedPorts,
		Privileged:   dockerObj.privileged,
	}

	// Handle ulimits
	ulimits := getUlimitsForContainer()
	if len(ulimits) > 0 {
		hostConfig.Resources.Ulimits = ulimits
	}

	// If not in privileged mode, add device permissions
	if !dockerObj.privileged {
		devices := getDeviceMappingsFromString(dockerObj.devices)

		if dockerObj.usbforward != "" {
			parts := strings.Split(dockerObj.usbforward, ":")
			if len(parts) == 2 {
				devices = append(devices, container.DeviceMapping{
					PathOnHost:        parts[0],
					PathInContainer:   parts[1],
					CgroupPermissions: "rwm",
				})
			}
		}

		// ── Hotplug-aware device filtering ─────────────────────────────
		//
		// If a /dev subtree is already bind-mounted (e.g. /dev/bus/usb),
		// individual device entries under that subtree are redundant and
		// prevent USB hotplug from working (they are static snapshots of
		// the device nodes that existed at container creation time).
		//
		// We remove them here and rely on the bind mount (filesystem
		// visibility) + cgroup device rule (kernel access) instead.
		//
		bindSet := make(map[string]bool)
		for _, b := range bindings {
			parts := strings.SplitN(b, ":", 3)
			if len(parts) >= 2 {
				bindSet[parts[1]] = true // destination (container) path
			}
		}

		var filteredDevices []container.DeviceMapping
		for _, dev := range devices {
			covered := false
			for bindPath := range bindSet {
				if strings.HasPrefix(dev.PathOnHost, bindPath+"/") || dev.PathOnHost == bindPath {
					covered = true
					break
				}
			}
			if !covered {
				filteredDevices = append(filteredDevices, dev)
			}
		}

		hostConfig.Devices = filteredDevices

		// ── Cgroup rules ───────────────────────────────────────────────
		if dockerObj.cgroups != "" {
			rules := strings.Split(dockerObj.cgroups, ",")
			// Fix permission order: "rmw" → "rwm"
			for i, rule := range rules {
				rules[i] = strings.TrimSpace(rule)
				rules[i] = strings.Replace(rules[i], "rmw", "rwm", 1)
			}
			hostConfig.DeviceCgroupRules = rules
		}

		// Auto-inject cgroup device rules for bind-mounted /dev subtrees
		// so that hotplug works out-of-the-box without needing explicit
		// --cgroups flags for common device classes.
		devMajorRules := map[string]string{
			"/dev/bus/usb": "c 189:* rwm",
			"/dev/snd":     "c 116:* rwm",
			"/dev/dri":     "c 226:* rwm",
			"/dev/input":   "c 13:* rwm",
			"/dev/vhci":    "c 137:* rwm",
		}

		existingRules := make(map[string]bool)
		for _, rule := range hostConfig.DeviceCgroupRules {
			existingRules[rule] = true
		}

		for bindPath := range bindSet {
			if rule, ok := devMajorRules[bindPath]; ok && !existingRules[rule] {
				hostConfig.DeviceCgroupRules = append(hostConfig.DeviceCgroupRules, rule)
				existingRules[rule] = true
			}
		}

		// Also inject cgroup rules for remaining individual device entries
		for _, dev := range filteredDevices {
			for prefix, rule := range devMajorRules {
				if strings.HasPrefix(dev.PathOnHost, prefix) && !existingRules[rule] {
					hostConfig.DeviceCgroupRules = append(hostConfig.DeviceCgroupRules, rule)
					existingRules[rule] = true
				}
			}
		}

		// ── Seccomp ────────────────────────────────────────────────────
		if dockerObj.seccomp != "" {
			seccompOpts := strings.Split(dockerObj.seccomp, ",")
			for i, opt := range seccompOpts {
				if !strings.Contains(opt, "=") {
					seccompOpts[i] = "seccomp=" + opt
				}
			}
			hostConfig.SecurityOpt = seccompOpts
		}

		// ── Capabilities ───────────────────────────────────────────────
		if dockerObj.caps != "" {
			hostConfig.CapAdd = strings.Split(dockerObj.caps, ",")
		}
	}

	containerLabels := map[string]string{
		"org.container.project": "rfswift",
	}
	if len(hostConfig.DeviceCgroupRules) > 0 {
		containerLabels["org.rfswift.cgroup_rules"] = strings.Join(hostConfig.DeviceCgroupRules, ",")
	}
	if dockerObj.exposed_ports == "" {
		containerLabels["org.rfswift.exposed_ports"] = "none"
	} else {
		containerLabels["org.rfswift.exposed_ports"] = dockerObj.exposed_ports
	}

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
		Labels:       containerLabels,
	}, hostConfig, &network.NetworkingConfig{}, nil, containerName)

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

	optionsCreate := container.ExecOptions{
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

	optionsStartCheck := container.ExecStartOptions{
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

func combineBindings(x11forward, extrabinding string) []string {
	var bindings []string

	if extrabinding != "" {
		bindings = append(bindings, strings.Split(extrabinding, ",")...)
	}
	if x11forward != "" {
		bindings = append(bindings, strings.Split(x11forward, ",")...)
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
	cli, err := NewEngineClient()
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
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	// Get current architecture
	architecture := getArchitecture()

	imageref = normalizeImageName(imageref)

	// Parse the image reference to get repo and tag
	parts := strings.Split(imageref, ":")
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 {
		tag = parts[1]
	}

	// Check if this is an official image that might need architecture suffix
	isOfficial := IsOfficialImage(imageref)

	// For official images, ALWAYS use architecture suffix
	actualPullRef := imageref
	if isOfficial && architecture != "" {
		// Check if tag already has an architecture suffix
		hasArchSuffix := strings.HasSuffix(tag, "_amd64") ||
			strings.HasSuffix(tag, "_arm64") ||
			strings.HasSuffix(tag, "_riscv64") ||
			strings.HasSuffix(tag, "_arm")

		if !hasArchSuffix {
			// Append architecture to the tag - this is required for official images
			actualPullRef = fmt.Sprintf("%s:%s_%s", repo, tag, architecture)
			common.PrintInfoMessage(fmt.Sprintf("Detected architecture: %s, pulling %s", architecture, actualPullRef))
		} else {
			common.PrintInfoMessage(fmt.Sprintf("Using architecture-specific tag: %s", actualPullRef))
		}
	} else if isOfficial && architecture == "" {
		common.PrintErrorMessage(fmt.Errorf("cannot determine system architecture for official image"))
		return
	}

	// Set the display tag (without architecture suffix for cleaner naming)
	if imagetag == "" {
		// Use clean tag name without architecture suffix
		imagetag = fmt.Sprintf("%s:%s", repo, tag)
	}

	// Check if the image exists locally
	localInspect, _, err := cli.ImageInspectWithRaw(ctx, imagetag)
	localExists := err == nil
	localDigest := ""
	if localExists {
		localDigest = localInspect.ID
	}

	// Pull the image from remote using the architecture-specific reference
	common.PrintInfoMessage(fmt.Sprintf("Pulling image from: %s", actualPullRef))
	out, err := cli.ImagePull(ctx, actualPullRef, image.PullOptions{})
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer out.Close()

	// Process pull output
	fd, isTerminal := term.GetFdInfo(os.Stdout)
	jsonDecoder := json.NewDecoder(out)
	for {
		var msg jsonmessage.JSONMessage
		if err := jsonDecoder.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		if isTerminal {
			_ = jsonmessage.DisplayJSONMessagesStream(out, os.Stdout, fd, isTerminal, nil)
		} else {
			fmt.Println(msg)
		}
	}

	// Get information about the pulled image
	remoteInspect, _, err := cli.ImageInspectWithRaw(ctx, actualPullRef)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	// Compare local and remote images
	if localExists && localDigest != remoteInspect.ID {
		common.PrintInfoMessage("The pulled image is different from the local one.")
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Do you want to rename the old image with a date tag? (y/n): ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "y" || response == "yes" {
			currentTime := time.Now()
			dateTag := fmt.Sprintf("%s-%02d%02d%d", imagetag, currentTime.Day(), currentTime.Month(), currentTime.Year())
			err = cli.ImageTag(ctx, localDigest, dateTag)
			if err != nil {
				common.PrintErrorMessage(err)
				return
			}
			common.PrintSuccessMessage(fmt.Sprintf("Old image '%s' retagged as '%s'", imagetag, dateTag))
		}
	}

	// Tag the pulled image with the clean name (without architecture suffix)
	if imagetag != actualPullRef {
		err = cli.ImageTag(ctx, remoteInspect.ID, imagetag)
		if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		common.PrintSuccessMessage(fmt.Sprintf("Image tagged as '%s'", imagetag))

		// Remove the original architecture-suffixed tag to avoid duplicates in local listing
		if IsOfficialImage(actualPullRef) {
			_, err = cli.ImageRemove(ctx, actualPullRef, image.RemoveOptions{Force: false})
			if err != nil {
				// Only log if it's not a "tag not found" or "in use" error
				if !strings.Contains(err.Error(), "No such image") && !strings.Contains(err.Error(), "image is referenced") {
					log.Printf("Note: Could not remove architecture-suffixed tag %s: %v", actualPullRef, err)
				}
			} else {
				common.PrintInfoMessage(fmt.Sprintf("Removed architecture-suffixed tag: %s", actualPullRef))
			}
		}
	}

	common.PrintSuccessMessage(fmt.Sprintf("Image '%s' installed successfully", imagetag))
}

func DockerTag(imageref string, imagetag string) {
	/* Rename an image to another name
	   in(1): string Image reference
	   in(2): string Image tag target
	*/
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	// Normalize source image reference
	imageref = normalizeImageName(imageref)

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
	cli, err := NewEngineClient()
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
		if container.ID == currentIdentifier || (len(container.Names) > 0 && container.Names[0] == "/"+currentIdentifier) {
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
	cli, err := NewEngineClient()
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
		if container.ID == containerIdentifier || (len(container.Names) > 0 && container.Names[0] == "/"+containerIdentifier) {
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
	cli, err := NewEngineClient()
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

func PrintImagesTable(labelKey string, labelValue string, showVersions bool, filterImage string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		log.Fatalf("Error creating Docker client: %v", err)
	}
	defer cli.Close()

	images, err := ListImages(labelKey, labelValue)
	if err != nil {
		log.Fatalf("Error listing images: %v", err)
	}

	rfutils.ClearScreen()

	// Fetch remote versions ONCE for all checks - BY REPO
	architecture := getArchitecture()
	var remoteVersionsByRepo RepoVersionMap
	if !common.Disconnected {
		remoteVersionsByRepo = GetAllRemoteVersionsByRepo(architecture)
	} else {
		remoteVersionsByRepo = make(RepoVersionMap)
	}

	// Prepare table data
	tableData := [][]string{}
	maxStatusLength := 0
	maxVersionLength := 0

	for _, image := range images {
		for _, repoTag := range image.RepoTags {
			repoTagParts := strings.Split(repoTag, ":")
			if len(repoTagParts) != 2 {
				continue
			}
			repository := repoTagParts[0]
			tag := repoTagParts[1]

			// Apply filter if specified
			if filterImage != "" && !strings.Contains(strings.ToLower(tag), strings.ToLower(filterImage)) {
				continue
			}

			// Check image status using cached versions BY REPO
			isUpToDate, isCustom, err := checkImageStatusWithCache(ctx, cli, repository, tag, architecture, remoteVersionsByRepo)
			var status string
			if err != nil {
				status = "Error"
			} else if isCustom {
				status = "Custom"
				if common.Disconnected {
					status = "No network"
				}
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

			// Get version info from cached data FOR THIS REPO
			versionDisplay := ""
			if showVersions {
				baseName, existingVersion := parseTagVersion(tag)

				// If tag already has a version, display it
				if existingVersion != "" {
					versionDisplay = existingVersion
				} else {
					// Get local image digest and find matching version in THIS REPO
					localDigest := getLocalImageDigests(ctx, cli, repoTag)
					if len(localDigest) > 0 {
						if repoVersions, ok := remoteVersionsByRepo[repository]; ok {
							if versions, ok := repoVersions[baseName]; ok {
								matchedVersion := GetVersionForDigests(versions, localDigest)
								if matchedVersion != "" {
									versionDisplay = matchedVersion
								}
							}
						}
					}
				}

				if versionDisplay == "" {
					versionDisplay = "-"
				}

				if len(versionDisplay) > maxVersionLength {
					maxVersionLength = len(versionDisplay)
				}
			}

			row := []string{
				repository,
				tag,
				image.ID[7:19], // sha256: prefix removed, first 12 chars
				created,
				size,
				status,
			}

			if showVersions {
				row = append(row, versionDisplay)
			}

			tableData = append(tableData, row)
		}
	}

	// Build headers
	headers := []string{"Repository", "Tag", "Image ID", "Created", "Size", "Status"}
	if showVersions {
		headers = append(headers, "Version")
	}

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

	// Ensure Status column is wide enough
	statusIdx := 5
	columnWidths[statusIdx] = max(columnWidths[statusIdx], maxStatusLength)

	// Ensure Version column is wide enough if present
	if showVersions && maxVersionLength > 0 {
		versionIdx := 6
		columnWidths[versionIdx] = max(columnWidths[versionIdx], maxVersionLength)
	}

	// Adjust column widths
	totalWidth := len(headers) + 1
	for _, w := range columnWidths {
		totalWidth += w + 2
	}

	if totalWidth > width {
		excess := totalWidth - width
		colsToAdjust := len(columnWidths) - 2
		for i := range columnWidths[:colsToAdjust] {
			reduction := excess / colsToAdjust
			if columnWidths[i] > reduction {
				columnWidths[i] -= reduction
				if columnWidths[i] < 5 {
					columnWidths[i] = 5
				}
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
		printRowWithColorAndVersion(row, columnWidths, "│", showVersions)
		if i < len(tableData)-1 {
			printHorizontalBorder(columnWidths, "├", "┼", "┤")
		}
	}

	printHorizontalBorder(columnWidths, "└", "┴", "┘")

	fmt.Print(reset)
	fmt.Println()
}

// max helper function (if not already present)
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// printRowWithColorAndVersion prints a row with status and version colors
func printRowWithColorAndVersion(row []string, columnWidths []int, separator string, showVersions bool) {
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	cyan := "\033[36m"
	reset := "\033[0m"

	fmt.Print(separator)
	for i, col := range row {
		color := ""

		if i == 5 { // Status column
			switch col {
			case "Custom", "No network":
				color = yellow
			case "Up to date":
				color = green
			case "Obsolete", "Error":
				color = red
			}
		} else if showVersions && i == 6 && col != "-" { // Version column
			color = cyan
		}

		if color != "" {
			fmt.Printf(" %s%-*s%s ", color, columnWidths[i], truncateString(col, columnWidths[i]), reset)
		} else {
			fmt.Printf(" %-*s ", columnWidths[i], truncateString(col, columnWidths[i]))
		}
		fmt.Print(separator)
	}
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
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to create Docker client: %v", err))
		return err
	}
	defer cli.Close()

	// Normalize image reference (but not if it looks like an ID)
	if !strings.HasPrefix(imageIDOrTag, "sha256:") && len(imageIDOrTag) != 12 && len(imageIDOrTag) != 64 {
		imageIDOrTag = normalizeImageName(imageIDOrTag)
	}

	common.PrintInfoMessage(fmt.Sprintf("Attempting to delete image: %s", imageIDOrTag))

	// List all images
	images, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to list images: %v", err))
		return err
	}

	var imageToDelete image.Summary
	imageFound := false

	// Get current architecture for matching
	architecture := getArchitecture()

	for _, img := range images {
		// Check if the full image ID matches
		if img.ID == "sha256:"+imageIDOrTag || img.ID == imageIDOrTag {
			imageToDelete = img
			imageFound = true
			break
		}

		// Check if any RepoTags match
		for _, tag := range img.RepoTags {
			normalizedTag := tag

			// If the input doesn't contain ":", prepend the repo
			if !strings.Contains(imageIDOrTag, ":") {
				imageIDOrTag = fmt.Sprintf("%s:%s", dockerObj.repotag, imageIDOrTag)
			}

			// Check for exact match first
			if normalizedTag == imageIDOrTag {
				imageToDelete = img
				imageFound = true
				break
			}

			// For official images, also check with architecture suffix
			if IsOfficialImage(imageIDOrTag) {
				// Extract repo and tag from the search term
				parts := strings.Split(imageIDOrTag, ":")
				if len(parts) == 2 {
					repo := parts[0]
					searchTag := parts[1]

					// Try matching with architecture suffix
					tagWithArch := fmt.Sprintf("%s:%s_%s", repo, searchTag, architecture)
					if normalizedTag == tagWithArch {
						imageToDelete = img
						imageFound = true
						break
					}
				}
			}

			// Also check if the stored tag (which might have arch suffix) matches
			// when we strip the architecture suffix from it
			cleanTag := normalizedTag
			parts := strings.Split(cleanTag, ":")
			if len(parts) == 2 {
				tagPart := parts[1]
				cleanedTagPart := removeArchitectureSuffix(tagPart)
				cleanTag = fmt.Sprintf("%s:%s", parts[0], cleanedTagPart)

				if cleanTag == imageIDOrTag {
					imageToDelete = img
					imageFound = true
					break
				}
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
			// Display tags with clean names
			displayTags := []string{}
			for _, tag := range img.RepoTags {
				parts := strings.Split(tag, ":")
				if len(parts) == 2 {
					cleanTagPart := removeArchitectureSuffix(parts[1])
					displayTags = append(displayTags, fmt.Sprintf("%s:%s", parts[0], cleanTagPart))
				} else {
					displayTags = append(displayTags, tag)
				}
			}
			common.PrintInfoMessage(fmt.Sprintf("ID: %s, Tags: %v", strings.TrimPrefix(img.ID, "sha256:"), displayTags))
		}
		return fmt.Errorf("image not found: %s", imageIDOrTag)
	}

	imageID := imageToDelete.ID

	// Display clean tag names in the confirmation
	displayTags := []string{}
	for _, tag := range imageToDelete.RepoTags {
		parts := strings.Split(tag, ":")
		if len(parts) == 2 {
			cleanTagPart := removeArchitectureSuffix(parts[1])
			displayTags = append(displayTags, fmt.Sprintf("%s:%s", parts[0], cleanTagPart))
		} else {
			displayTags = append(displayTags, tag)
		}
	}

	common.PrintInfoMessage(fmt.Sprintf("Found image to delete: ID: %s, Tags: %v", strings.TrimPrefix(imageID, "sha256:"), displayTags))

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
	cli, err := NewEngineClient()
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

	// Step 4: Run "ldconfig"
	common.PrintInfoMessage("Running 'ldconfig'...")
	if err := showLoadingIndicator(ctx, func() error {
		return execCommand(ctx, cli, containerIdentifier, []string{"/bin/bash", "-c", "ldconfig"})
	}, "ldconfig"); err != nil {
		return err
	}

	return nil
}

// execCommand executes a command in the container, capturing only errors if any
func execCommand(ctx context.Context, cli *client.Client, containerID string, cmd []string, workingDir ...string) error {
	execConfig := container.ExecOptions{
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

	attachResp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
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
	var timeout = 10

	// Check if the system is Windows
	if runtime.GOOS == "windows" {
		title := "Unsupported on Windows"
		message := `This function is not supported on Windows.
However, you can achieve similar functionality by using the following commands:
- "rfswift commit" to create a new image with a new tag.
- "rfswift remove" to remove the existing container.
- "rfswift run" to run a container with new bindings.`

		rfutils.DisplayNotification(title, message, "warning")
		os.Exit(1)
	}

	if source == "" {
		source = target
		common.PrintWarningMessage(fmt.Sprintf("Source is empty. Defaulting source to target: %s", target))
	}

	// Check if source (host mount point) exists when adding a new binding
	if add {
		if _, err := os.Stat(source); os.IsNotExist(err) {
			common.PrintErrorMessage(fmt.Errorf("host mount point does not exist: %s", source))
			common.PrintInfoMessage("Please create the directory first or check the path")
			os.Exit(1)
		} else if err != nil {
			common.PrintErrorMessage(fmt.Errorf("error checking host mount point: %v", err))
			os.Exit(1)
		}
		common.PrintSuccessMessage(fmt.Sprintf("Verified host mount point exists: %s", source))
	}

	ctx := context.Background()

	common.PrintInfoMessage("Fetching container ID...")
	containerID := getContainerIDByName(ctx, containerName)
	if containerID == "" {
		common.PrintErrorMessage(fmt.Errorf("container %s not found", containerName))
		os.Exit(1)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Container ID: %s", containerID))

	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error when instantiating a client"))
		os.Exit(1)
	}

	// Stop the container
	common.PrintInfoMessage("Stopping the container...")
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

	newMount := fmt.Sprintf("%s:%s", source, target)

	// ─── Engine-aware code path ────────────────────────────────────────
	//
	// Docker:  edit hostconfig.json + config.v2.json on disk, restart daemon
	// Podman:  recreate the container with updated bind mounts (no direct edit)
	//
	if !EngineSupportsDirectConfigEdit() {
		// ── Podman path: container recreation ──────────────────────────
		common.PrintInfoMessage(fmt.Sprintf("%s does not support direct config editing — using container recreation", GetEngine().Name()))

		// Get current container config for recreation
		inspectData, err := cli.ContainerInspect(ctx, containerID)
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
			os.Exit(1)
		}

		// Update binds (use prefix matching for Podman — binds may have
		// trailing options like ":rw,rprivate,nosuid,rbind")
		currentBinds := inspectData.HostConfig.Binds
		if add {
			if !bindExistsByPrefix(currentBinds, newMount) {
				currentBinds = append(currentBinds, newMount)
				common.PrintSuccessMessage(fmt.Sprintf("Adding mount: %s", newMount))
			} else {
				common.PrintWarningMessage("Mount already exists.")
				return
			}
		} else {
			currentBinds = removeBindByPrefix(currentBinds, newMount)
			common.PrintSuccessMessage(fmt.Sprintf("Removing mount: %s", newMount))
		}

		// Recreate container with updated binds
		if err := recreateContainerWithUpdatedBinds(ctx, cli, containerName, containerID, inspectData, currentBinds); err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to recreate container: %v", err))
			os.Exit(1)
		}

		common.PrintSuccessMessage("Container recreated with updated mount bindings.")
		return
	}

	// ── Docker path: direct config file editing ───────────────────────
	common.PrintInfoMessage("Determining hostconfig.json path...")
	hostConfigPath, err := EngineGetHostConfigPath(containerID)
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

	// Restart the engine service
	engineName := GetEngine().Name()
	if err := showLoadingIndicator(ctx, func() error {
		return EngineRestartService()
	}, fmt.Sprintf("Restarting %s service...", engineName)); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to restart %s service: %v", engineName, err))
		os.Exit(1)
	}
	common.PrintSuccessMessage(fmt.Sprintf("%s service restarted successfully.", engineName))
}


// podmanCreateViaCLI creates a container using the native Podman CLI instead of
// the Docker compat API. This is necessary because the compat API silently
// ignores DeviceCgroupRules — a known Podman limitation.
// Returns the new container ID.
func podmanCreateViaCLI(name string, imageName string, cfg *container.Config, hc *container.HostConfig) (string, error) {
	args := []string{"create", "--name", name}

	// TTY and stdin — critical to keep the container alive
	if cfg.Tty {
		args = append(args, "-t")
	}
	if cfg.OpenStdin {
		args = append(args, "-i")
	}

	// Bind mounts
	for _, b := range hc.Binds {
		args = append(args, "-v", b)
	}

	// Devices
	for _, d := range hc.Devices {
		devStr := d.PathOnHost + ":" + d.PathInContainer
		if d.CgroupPermissions != "" {
			devStr += ":" + d.CgroupPermissions
		}
		args = append(args, "--device", devStr)
	}

	// Device cgroup rules — the whole reason we're using CLI
	for _, rule := range hc.DeviceCgroupRules {
		args = append(args, "--device-cgroup-rule", rule)
	}

	// Network mode
	if hc.NetworkMode != "" {
		args = append(args, "--network", string(hc.NetworkMode))
	}

	// Extra hosts
	for _, h := range hc.ExtraHosts {
		args = append(args, "--add-host", h)
	}

	// Environment variables
	for _, e := range cfg.Env {
		args = append(args, "-e", e)
	}

	// Labels
	for k, v := range cfg.Labels {
		args = append(args, "-l", k+"="+v)
	}

	// Capabilities
	for _, cap := range hc.CapAdd {
		args = append(args, "--cap-add", cap)
	}
	for _, cap := range hc.CapDrop {
		args = append(args, "--cap-drop", cap)
	}

	// Hostname
	if cfg.Hostname != "" {
		args = append(args, "--hostname", cfg.Hostname)
	}

	// Ulimits
	for _, u := range hc.Ulimits {
		args = append(args, "--ulimit", fmt.Sprintf("%s=%d:%d", u.Name, u.Soft, u.Hard))
	}

	// Security options
	for _, s := range hc.SecurityOpt {
		args = append(args, "--security-opt", s)
	}

	// IPC mode
	if hc.IpcMode != "" {
		args = append(args, "--ipc", string(hc.IpcMode))
	}

	// PID mode
	if hc.PidMode != "" {
		args = append(args, "--pid", string(hc.PidMode))
	}

	// Privileged
	if hc.Privileged {
		args = append(args, "--privileged")
	}

	// Tmpfs mounts
	for path, opts := range hc.Tmpfs {
		if opts != "" {
			args = append(args, "--tmpfs", path+":"+opts)
		} else {
			args = append(args, "--tmpfs", path)
		}
	}

	// Entrypoint — must be JSON array for multi-element entrypoints
	if len(cfg.Entrypoint) > 0 {
		epJSON, _ := json.Marshal(cfg.Entrypoint)
		args = append(args, "--entrypoint", string(epJSON))
	}

	// Working dir
	if cfg.WorkingDir != "" {
		args = append(args, "-w", cfg.WorkingDir)
	}

	// User
	if cfg.User != "" {
		args = append(args, "--user", cfg.User)
	}

	// Exposed ports
	for port := range cfg.ExposedPorts {
	    args = append(args, "--expose", string(port))
	}

	// Port bindings
	for port, bindings := range hc.PortBindings {
	    for _, binding := range bindings {
	        hostPart := binding.HostPort
	        if binding.HostIP != "" {
	            hostPart = binding.HostIP + ":" + hostPart
	        }
	        args = append(args, "-p", hostPart+":"+string(port))
	    }
	}

	// Image (positional, must come before cmd)
	args = append(args, imageName)

	// Command
	if len(cfg.Cmd) > 0 {
		args = append(args, cfg.Cmd...)
	}

	cmd := exec.Command("podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}

	containerID := strings.TrimSpace(string(output))
	return containerID, nil
}
// cleanupStaleTempImages removes any leftover rfswift_rebind_tmp_<name>:* images
// from previous recreations. These can't be deleted right after creation (the new
// container references them), but once that container is stopped/removed for the
// NEXT recreation, they become orphaned and can be cleaned up.
func cleanupStaleTempImages(ctx context.Context, cli *client.Client, containerName string) {
	prefix := fmt.Sprintf("rfswift_rebind_tmp_%s:", containerName)
	images, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return
	}
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if strings.HasPrefix(tag, prefix) {
				_, err := cli.ImageRemove(ctx, img.ID, image.RemoveOptions{Force: false, PruneChildren: true})
				if err == nil {
					common.PrintSuccessMessage(fmt.Sprintf("Cleaned up stale temp image: %s", tag))
				}
				// If still in use, skip silently — will be cleaned next time
			}
		}
	}
}

// sanitizeHostConfigForPodman normalizes a Docker-inspected HostConfig so that
// Podman's stricter compat API and crun runtime accept it without errors.
func sanitizeHostConfigForPodman(hc *container.HostConfig) {
	if hc == nil {
		return
	}

	// 1. Device permissions — Podman rejects empty CgroupPermissions
	for i := range hc.Devices {
		if hc.Devices[i].CgroupPermissions == "" {
			hc.Devices[i].CgroupPermissions = "rwm"
		}
	}

	// 2. MemorySwappiness — crun rejects on cgroup v2
	swappiness := int64(-1)
	hc.MemorySwappiness = &swappiness

	// 3. KernelMemory — deprecated, rejected by Podman
	hc.KernelMemory = 0
	hc.KernelMemoryTCP = 0

	// 4. PidsLimit — Podman rejects 0 (use -1 for unlimited)
	if hc.PidsLimit != nil && *hc.PidsLimit == 0 {
		unlimited := int64(-1)
		hc.PidsLimit = &unlimited
	}

	// 5. OomKillDisable — crun rejects on cgroup v2
	hc.OomKillDisable = nil

	// 6. DeviceCgroupRules — remove empty strings and fix permission order
	var cleanRules []string
	for _, rule := range hc.DeviceCgroupRules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		// Fix common permission order mistake: "rmw" → "rwm"
		rule = strings.Replace(rule, "rmw", "rwm", 1)
		cleanRules = append(cleanRules, rule)
	}
	hc.DeviceCgroupRules = cleanRules

	// 7. Deduplicate Binds
	hc.Binds = deduplicateBinds(hc.Binds)

	// 8. Build a set of bind-mounted destination paths
	bindDests := make(map[string]bool)
	for _, bind := range hc.Binds {
		dest := parseBindDestination(bind)
		bindDests[dest] = true
	}

	// 9. Remove Binds that conflict with Devices (same destination path)
	//    Podman rejects "duplicate mount destination" when a path appears
	//    in both Devices and Binds — but only for exact matches.
	if len(hc.Devices) > 0 && len(hc.Binds) > 0 {
		deviceDests := make(map[string]bool)
		for _, dev := range hc.Devices {
			deviceDests[dev.PathInContainer] = true
		}
		var deduplicatedBinds []string
		for _, bind := range hc.Binds {
			dest := parseBindDestination(bind)
			if deviceDests[dest] {
				continue // skip — already covered by Devices
			}
			deduplicatedBinds = append(deduplicatedBinds, bind)
		}
		hc.Binds = deduplicatedBinds
	}

	// 10. USB / device hotplug support
	//
	//     When a /dev subtree is bind-mounted (e.g. /dev/bus/usb), the bind
	//     gives filesystem visibility for new device nodes.  However, cgroup v2
	//     still blocks open() unless an explicit device-cgroup rule is present.
	//
	//     Additionally, individual device entries under a bind-mounted subtree
	//     are counter-productive: they are static snapshots captured at container
	//     creation time and break when devices are unplugged/replugged (the new
	//     device node number won't match the frozen mapping).
	//
	//     Strategy:
	//       • Inject the correct cgroup major-number rule
	//       • Remove individual Device entries that fall under the bind mount
	//
	devMajorRules := map[string]string{
		"/dev/bus/usb": "c 189:* rwm", // USB
		"/dev/snd":     "c 116:* rwm", // ALSA sound
		"/dev/dri":     "c 226:* rwm", // DRI / GPU
		"/dev/input":   "c 13:* rwm",  // Input devices (evdev, mice, js)
		"/dev/vhci":    "c 137:* rwm", // USB/IP VHCI
	}

	existingRules := make(map[string]bool)
	for _, rule := range hc.DeviceCgroupRules {
		existingRules[rule] = true
	}

	for prefix, rule := range devMajorRules {
		if !bindDests[prefix] {
			continue
		}

		// Inject cgroup rule if missing
		if !existingRules[rule] {
			hc.DeviceCgroupRules = append(hc.DeviceCgroupRules, rule)
			existingRules[rule] = true
		}

		// Remove individual Device entries under this prefix — the bind
		// mount + cgroup rule handles them, and keeping static entries
		// prevents hotplug from working.
		var cleanDevices []container.DeviceMapping
		for _, dev := range hc.Devices {
			if strings.HasPrefix(dev.PathOnHost, prefix) {
				continue // covered by bind + cgroup
			}
			cleanDevices = append(cleanDevices, dev)
		}
		hc.Devices = cleanDevices
	}

	// 11. Also inject cgroup rules for device entries that are NOT covered
	//     by a bind mount (standalone --device mappings).  This ensures
	//     that existing device access keeps working after recreation.
	for _, dev := range hc.Devices {
		for prefix, rule := range devMajorRules {
			if strings.HasPrefix(dev.PathOnHost, prefix) && !existingRules[rule] {
				hc.DeviceCgroupRules = append(hc.DeviceCgroupRules, rule)
				existingRules[rule] = true
			}
		}
	}
}

// deduplicateBinds removes bind entries with duplicate destinations.
// Keeps the last occurrence (so newly added binds take precedence).
func deduplicateBinds(binds []string) []string {
	seen := make(map[string]int) // destination → index in result
	var result []string

	for _, bind := range binds {
		dest := parseBindDestination(bind)
		if idx, exists := seen[dest]; exists {
			// Replace the earlier entry with this one
			result[idx] = bind
		} else {
			seen[dest] = len(result)
			result = append(result, bind)
		}
	}
	return result
}

// parseBindDestination extracts the destination (container) path from a bind string.
// Formats: "source:dest" or "source:dest:opts"
func parseBindDestination(bind string) string {
	parts := strings.SplitN(bind, ":", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return bind // bare path
}

// recreateContainerWithUpdatedBinds handles the Podman code path:
// commit current state → remove old container → create new one with updated binds.
func recreateContainerWithUpdatedBinds(ctx context.Context, cli *client.Client, containerName string, containerID string, inspectData types.ContainerJSON, newBinds []string) error {
	// 0. Clean up any stale temp images from previous recreations
	cleanupStaleTempImages(ctx, cli, containerName)

	// 1. Commit the current container state to a temporary image
	tempImageTag := fmt.Sprintf("rfswift_rebind_tmp_%s:%s", containerName, time.Now().Format("20060102150405"))
	common.PrintInfoMessage(fmt.Sprintf("Committing container state to temporary image: %s", tempImageTag))

	parts := strings.SplitN(tempImageTag, ":", 2)
	commitResp, err := cli.ContainerCommit(ctx, containerID, container.CommitOptions{
		Reference: tempImageTag,
		Comment:   "RF Swift: temporary image for mount binding update",
		Pause:     true,
	})
	if err != nil {
		return fmt.Errorf("failed to commit container: %v", err)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Committed as: %s (ID: %s)", tempImageTag, commitResp.ID[:12]))
	_ = parts // used for reference naming

	// 2. Rebuild container config with updated binds
	oldConfig := inspectData.Config
	oldHostConfig := inspectData.HostConfig

	// Apply updated binds
	oldHostConfig.Binds = newBinds

	// ── Sanitize HostConfig for Podman cgroup v2 compat ──
	// Podman's inspect returns these fields but rejects them on create.
	for i := range oldHostConfig.Devices {
		if oldHostConfig.Devices[i].CgroupPermissions == "" {
			oldHostConfig.Devices[i].CgroupPermissions = "rwm"
		}
	}
	oldHostConfig.Resources.MemorySwappiness = nil
	oldHostConfig.Resources.KernelMemory = 0
	if oldHostConfig.Resources.PidsLimit != nil && *oldHostConfig.Resources.PidsLimit == 0 {
		oldHostConfig.Resources.PidsLimit = nil
	}
	oldHostConfig.Resources.OomKillDisable = nil

	// ── KEY FIX: create from committed image, not the original base image ──
	// inspectData.Config.Image points to the original (e.g. sdr_light).
	// We must create from the committed snapshot to preserve the filesystem
	// (installed packages, user files, etc.).
	//
	// After a previous recreation, Config.Image will be a temp tag — in that
	// case, read the REAL original from the label we stored last time.
	originalImageName := oldConfig.Image
	if label, ok := oldConfig.Labels["org.rfswift.original_image"]; ok && label != "" {
		originalImageName = label // preserve the true original across multiple recreations
	}
	oldConfig.Image = tempImageTag // ← use committed snapshot

	// Store original image name + cgroup rules in labels for display purposes.
	// Podman's compat API doesn't return DeviceCgroupRules in inspect, so we
	// persist them as a label sidecar.
	if oldConfig.Labels == nil {
		oldConfig.Labels = make(map[string]string)
	}
	oldConfig.Labels["org.rfswift.original_image"] = originalImageName

	// ── CRITICAL: restore cgroup rules from label into HostConfig ──
	// Podman's inspect returns DeviceCgroupRules as empty, but we stored
	// the real rules in a label. We must inject them back so the recreated
	// container actually gets the cgroup rules applied (not just displayed).
	if len(oldHostConfig.DeviceCgroupRules) == 0 {
		if label, ok := oldConfig.Labels["org.rfswift.cgroup_rules"]; ok && label != "" {
			oldHostConfig.DeviceCgroupRules = strings.Split(label, ",")
		}
	}
	// Update the label with current rules (may have been restored above)
	if len(oldHostConfig.DeviceCgroupRules) > 0 {
		oldConfig.Labels["org.rfswift.cgroup_rules"] = strings.Join(oldHostConfig.DeviceCgroupRules, ",")
	}

	oldConfig.Labels["org.rfswift.exposed_ports"] = convertExposedPortsToString(oldConfig.ExposedPorts)

	// ── USB bind-mount sanitization ──
	// When /dev/bus/usb is bind-mounted, individual /dev/bus/usb/* device
	// entries must be removed (they create specific allow rules that conflict
	// with the wildcard cgroup rule needed for hotplug). Also ensure the
	// USB cgroup rule c 189:* rwm is present.
	// This mirrors the same sanitization done in DockerRun().
	hasUSBBind := false
	for _, b := range newBinds {
		if strings.HasPrefix(b, "/dev/bus/usb:") || strings.HasPrefix(b, "/dev/bus/usb/") || b == "/dev/bus/usb" {
			hasUSBBind = true
			break
		}
	}
	if hasUSBBind {
		// Remove individual USB device entries — the bind mount covers them
		var cleanDevices []container.DeviceMapping
		for _, d := range oldHostConfig.Devices {
			if !strings.HasPrefix(d.PathOnHost, "/dev/bus/usb/") {
				cleanDevices = append(cleanDevices, d)
			}
		}
		oldHostConfig.Devices = cleanDevices

		// Ensure c 189:* rwm rule is present for USB hotplug
		usbRule := "c 189:* rwm"
		hasUSBRule := false
		for _, r := range oldHostConfig.DeviceCgroupRules {
			if r == usbRule {
				hasUSBRule = true
				break
			}
		}
		if !hasUSBRule {
			oldHostConfig.DeviceCgroupRules = append(oldHostConfig.DeviceCgroupRules, usbRule)
		}
		// Update label
		oldConfig.Labels["org.rfswift.cgroup_rules"] = strings.Join(oldHostConfig.DeviceCgroupRules, ",")
	}

	// 3. Create a temporary-named container first, then swap
	tempContainerName := fmt.Sprintf("%s_rfswift_tmp_%d", containerName, time.Now().UnixNano())
	common.PrintInfoMessage(fmt.Sprintf("Creating new container (temp: %s)...", tempContainerName))

	var newContainerID string

	// ── Podman: use native CLI for creation when cgroup rules are present ──
	// The Docker compat API silently ignores DeviceCgroupRules in ContainerCreate.
	// We must use `podman create` directly to guarantee the rules are applied.
	if len(oldHostConfig.DeviceCgroupRules) > 0 && !EngineSupportsDirectConfigEdit() {
		containerID, err := podmanCreateViaCLI(tempContainerName, tempImageTag, oldConfig, oldHostConfig)
		if err != nil {
			return fmt.Errorf("failed to create new container via Podman CLI: %v", err)
		}
		newContainerID = containerID
	} else {
		resp, err := cli.ContainerCreate(ctx,
			oldConfig,
			oldHostConfig,
			nil, // networking config — will be reattached
			nil, // platform
			tempContainerName,
		)
		if err != nil {
			return fmt.Errorf("failed to create new container: %v", err)
		}
		newContainerID = resp.ID
	}
	common.PrintSuccessMessage(fmt.Sprintf("New container created: %s", newContainerID[:12]))

	// 4. Remove old container (safe — new one already exists)
	common.PrintInfoMessage("Removing old container...")
	err = cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	if err != nil {
		return fmt.Errorf("failed to remove old container: %v", err)
	}
	common.PrintSuccessMessage("Old container removed.")

	// 5. Rename temp container to original name
	common.PrintInfoMessage(fmt.Sprintf("Renaming container to '%s'...", containerName))
	if err := cli.ContainerRename(ctx, newContainerID, containerName); err != nil {
		return fmt.Errorf("failed to rename container: %v", err)
	}

	// 6. Start the new container
	common.PrintInfoMessage("Starting new container...")
	if err := cli.ContainerStart(ctx, newContainerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start new container: %v", err)
	}
	common.PrintSuccessMessage("Container started with updated mount bindings.")

	// NOTE: We intentionally do NOT delete the temp image here.
	// The new container references it, so Podman would refuse anyway.
	// It will be cleaned up at the START of the next recreation
	// (see cleanupStaleTempImages above).

	return nil
}

// execCommandWithOutput executes a command and returns its output
func execCommandWithOutput(ctx context.Context, cli *client.Client, containerID string, cmd []string) (string, error) {
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}

	execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec instance: %v", err)
	}

	attachResp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec instance: %v", err)
	}
	defer attachResp.Close()

	// Read the output
	output, err := io.ReadAll(attachResp.Reader)
	if err != nil {
		return "", err
	}

	return string(output), nil
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

func UpdateDeviceBinding(containerName string, deviceHost string, deviceContainer string, add bool) {
	var timeout = 10 // Stop timeout

	// Check if the system is Windows
	if runtime.GOOS == "windows" {
		title := "Unsupported on Windows"
		message := `This function is not supported on Windows.
However, you can achieve similar functionality by using the following commands:
- "rfswift commit" to create a new image with a new tag.
- "rfswift remove" to remove the existing container.
- "rfswift run" to run a container with new device bindings.`

		rfutils.DisplayNotification(title, message, "warning")
		os.Exit(1) // Exit since this function is not supported on Windows
	}

	if deviceHost == "" {
		deviceHost = deviceContainer
		common.PrintWarningMessage(fmt.Sprintf("Host device path is empty. Defaulting to container device path: %s", deviceContainer))
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
	cli, err := NewEngineClient()
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

	// ─── Engine-aware code path ────────────────────────────────────────
	//
	// Docker:  edit hostconfig.json + config.v2.json on disk, restart daemon
	// Podman:  recreate the container with updated devices (no direct edit)
	//
	if !EngineSupportsDirectConfigEdit() {
		// ── Podman path: container recreation ──────────────────────────
		common.PrintInfoMessage(fmt.Sprintf("%s does not support direct config editing — using container recreation", GetEngine().Name()))

		// Get current container config for recreation
		inspectData, err := cli.ContainerInspect(ctx, containerID)
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
			os.Exit(1)
		}

		// Update devices in the inspected HostConfig
		currentDevices := inspectData.HostConfig.Devices
		if add {
			if !deviceExistsInSlice(currentDevices, deviceHost, deviceContainer) {
				newDevice := container.DeviceMapping{
					PathOnHost:        deviceHost,
					PathInContainer:   deviceContainer,
					CgroupPermissions: "rwm",
				}
				inspectData.HostConfig.Devices = append(currentDevices, newDevice)
				common.PrintSuccessMessage(fmt.Sprintf("Adding device: %s to %s", deviceHost, deviceContainer))
			} else {
				common.PrintWarningMessage("Device mapping already exists.")
				return
			}
		} else {
			inspectData.HostConfig.Devices = removeDeviceMappingFromSlice(currentDevices, deviceHost, deviceContainer)
			common.PrintSuccessMessage(fmt.Sprintf("Removing device: %s from %s", deviceHost, deviceContainer))
		}

		// Recreate container — pass current binds unchanged, devices are
		// already updated in inspectData.HostConfig.Devices
		if err := recreateContainerWithUpdatedBinds(ctx, cli, containerName, containerID, inspectData, inspectData.HostConfig.Binds); err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to recreate container: %v", err))
			os.Exit(1)
		}

		common.PrintSuccessMessage("Container recreated with updated device bindings.")
		return
	}

	// ── Docker path: direct config file editing ───────────────────────

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

	// Update devices in both files
	common.PrintInfoMessage("Updating devices...")
	if add {
		if !deviceExists(hostConfig.Devices, deviceHost, deviceContainer) {
			newDevice := DeviceMapping{
				PathOnHost:        deviceHost,
				PathInContainer:   deviceContainer,
				CgroupPermissions: "rwm", // Default to read, write, mknod permissions
			}
			hostConfig.Devices = append(hostConfig.Devices, newDevice)
			addDeviceMapping(configV2, deviceHost, deviceContainer)
			common.PrintSuccessMessage(fmt.Sprintf("Added device: %s to %s", deviceHost, deviceContainer))
		} else {
			common.PrintWarningMessage("Device mapping already exists.")
		}
	} else {
		hostConfig.Devices = removeDeviceFromSlice(hostConfig.Devices, deviceHost, deviceContainer)
		removeDeviceMapping(configV2, deviceHost, deviceContainer)
		common.PrintSuccessMessage(fmt.Sprintf("Removed device: %s from %s", deviceHost, deviceContainer))
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

// deviceExistsInSlice checks if a device mapping already exists using the
// Docker SDK's container.DeviceMapping type (used in Podman recreation path).
func deviceExistsInSlice(devices []container.DeviceMapping, hostPath, containerPath string) bool {
	for _, d := range devices {
		if d.PathOnHost == hostPath && d.PathInContainer == containerPath {
			return true
		}
	}
	return false
}

// removeDeviceMappingFromSlice removes a device mapping by host+container path
// using the Docker SDK's container.DeviceMapping type (used in Podman recreation path).
func removeDeviceMappingFromSlice(devices []container.DeviceMapping, hostPath, containerPath string) []container.DeviceMapping {
	var result []container.DeviceMapping
	for _, d := range devices {
		if d.PathOnHost == hostPath && d.PathInContainer == containerPath {
			continue
		}
		result = append(result, d)
	}
	return result
}


// UpdateCapability adds or removes a capability from a container
func UpdateCapability(containerID string, capability string, add bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get container info first
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}
	
	// Parse existing capabilities
	var capabilities []string
	if props["Caps"] != "" {
		capabilities = strings.Split(props["Caps"], ",")
	}

	// Add or remove the capability
	if add {
		// Check if already exists
		found := false
		for _, cap := range capabilities {
			if strings.TrimSpace(cap) == capability {
				found = true
				break
			}
		}
		
		if found {
			common.PrintInfoMessage(fmt.Sprintf("Capability '%s' already exists in container '%s'", capability, containerName))
			return nil
		}
		
		capabilities = append(capabilities, capability)
		common.PrintInfoMessage(fmt.Sprintf("Adding capability '%s' to container '%s'", capability, containerName))
	} else {
		// Remove capability
		newCapabilities := []string{}
		found := false
		for _, cap := range capabilities {
			if strings.TrimSpace(cap) != capability {
				newCapabilities = append(newCapabilities, cap)
			} else {
				found = true
			}
		}
		
		if !found {
			common.PrintWarningMessage(fmt.Sprintf("Capability '%s' not found in container '%s'", capability, containerName))
			return nil
		}
		
		capabilities = newCapabilities
		common.PrintInfoMessage(fmt.Sprintf("Removing capability '%s' from container '%s'", capability, containerName))
	}

	// Update the container
	props["Caps"] = strings.Join(capabilities, ",")
	
	return recreateContainerWithProperties(ctx, cli, containerID, props)
}

// UpdateCgroupRule adds or removes a cgroup rule from a container
func UpdateCgroupRule(containerID string, rule string, add bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get container info first
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}
	
	// Parse existing cgroup rules
	var cgroupRules []string
	if props["Cgroups"] != "" {
		cgroupRules = strings.Split(props["Cgroups"], ",")
	}

	// Add or remove the rule
	if add {
		// Check if already exists
		found := false
		for _, r := range cgroupRules {
			if strings.TrimSpace(r) == rule {
				found = true
				break
			}
		}
		
		if found {
			common.PrintInfoMessage(fmt.Sprintf("Cgroup rule '%s' already exists in container '%s'", rule, containerName))
			return nil
		}
		
		cgroupRules = append(cgroupRules, rule)
		common.PrintInfoMessage(fmt.Sprintf("Adding cgroup rule '%s' to container '%s'", rule, containerName))
	} else {
		// Remove rule
		newRules := []string{}
		found := false
		for _, r := range cgroupRules {
			if strings.TrimSpace(r) != rule {
				newRules = append(newRules, r)
			} else {
				found = true
			}
		}
		
		if !found {
			common.PrintWarningMessage(fmt.Sprintf("Cgroup rule '%s' not found in container '%s'", rule, containerName))
			return nil
		}
		
		cgroupRules = newRules
		common.PrintInfoMessage(fmt.Sprintf("Removing cgroup rule '%s' from container '%s'", rule, containerName))
	}

	// Update the container
	props["Cgroups"] = strings.Join(cgroupRules, ",")
	
	return recreateContainerWithProperties(ctx, cli, containerID, props)
}

// UpdateExposedPort adds or removes an exposed port from a container
func UpdateExposedPort(containerID string, port string, add bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get container info first
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	// Parse existing exposed ports
	exposedPortsStr := props["ExposedPorts"]
	var exposedPorts []string
	if exposedPortsStr != "" {
		exposedPorts = strings.Split(exposedPortsStr, ",")
		// Trim spaces
		for i := range exposedPorts {
			exposedPorts[i] = strings.TrimSpace(exposedPorts[i])
		}
	}

	// Add or remove the port
	if add {
		// Check if already exists
		found := false
		for _, p := range exposedPorts {
			if p == port {
				found = true
				break
			}
		}

		if found {
			common.PrintInfoMessage(fmt.Sprintf("Port '%s' already exposed in container '%s'", port, containerName))
			return nil
		}

		exposedPorts = append(exposedPorts, port)
		common.PrintInfoMessage(fmt.Sprintf("Exposing port '%s' on container '%s'", port, containerName))
	} else {
		// Remove port
		newPorts := []string{}
		found := false
		for _, p := range exposedPorts {
			if p != port {
				newPorts = append(newPorts, p)
			} else {
				found = true
			}
		}

		if !found {
			common.PrintWarningMessage(fmt.Sprintf("Port '%s' not found in container '%s'", port, containerName))
			return nil
		}

		exposedPorts = newPorts
		common.PrintInfoMessage(fmt.Sprintf("Removing exposed port '%s' from container '%s'", port, containerName))
	}

	// Update the container
	props["ExposedPorts"] = strings.Join(exposedPorts, ",")

	return recreateContainerWithProperties(ctx, cli, containerID, props)
}

// UpdatePortBinding adds or removes a port binding from a container
func UpdatePortBinding(containerID string, binding string, add bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get container info first
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	// Parse existing port bindings
	portBindingsStr := props["PortBindings"]
	var portBindings []string
	if portBindingsStr != "" {
		portBindings = strings.Split(portBindingsStr, ";;")
		// Trim spaces
		for i := range portBindings {
			portBindings[i] = strings.TrimSpace(portBindings[i])
		}
	}

	// Add or remove the binding
	if add {
		// Check if already exists
		found := false
		for _, b := range portBindings {
			if b == binding {
				found = true
				break
			}
		}

		if found {
			common.PrintInfoMessage(fmt.Sprintf("Port binding '%s' already exists in container '%s'", binding, containerName))
			return nil
		}

		portBindings = append(portBindings, binding)
		common.PrintInfoMessage(fmt.Sprintf("Adding port binding '%s' to container '%s'", binding, containerName))
	} else {
		// Remove binding
		newBindings := []string{}
		found := false
		for _, b := range portBindings {
			if b != binding {
				newBindings = append(newBindings, b)
			} else {
				found = true
			}
		}

		if !found {
			common.PrintWarningMessage(fmt.Sprintf("Port binding '%s' not found in container '%s'", binding, containerName))
			return nil
		}

		portBindings = newBindings
		common.PrintInfoMessage(fmt.Sprintf("Removing port binding '%s' from container '%s'", binding, containerName))
	}

	// Update the container
	props["PortBindings"] = strings.Join(portBindings, ";;")

	return recreateContainerWithProperties(ctx, cli, containerID, props)
}

func recreateContainerWithProperties(ctx context.Context, cli *client.Client, containerID string, props map[string]string) error {
	// Get fresh container info
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}

	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	common.PrintInfoMessage(fmt.Sprintf("Updating container '%s'", containerName))

	// Stop the container
	common.PrintInfoMessage("Stopping container...")
	timeout := 10
	if err := cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		// Container might already be stopped — not fatal
		common.PrintWarningMessage(fmt.Sprintf("Stop returned: %v (may already be stopped)", err))
	}

	// ── 0. Clean up stale temp images from previous recreations ──
	cleanupStaleTempImages(ctx, cli, containerName)

	// ── 1. Commit the container state to a temporary image ──
	// This preserves installed packages, user files, etc.
	tempImageTag := fmt.Sprintf("rfswift_rebind_tmp_%s:%s", containerName, time.Now().Format("20060102150405"))
	common.PrintInfoMessage(fmt.Sprintf("Committing container state to temporary image: %s", tempImageTag))

	commitLabels := make(map[string]string)
	for k, v := range containerJSON.Config.Labels {
		commitLabels[k] = v
	}
	if props["ExposedPorts"] == "" {
		commitLabels["org.rfswift.exposed_ports"] = "none"
	} else {
		commitLabels["org.rfswift.exposed_ports"] = props["ExposedPorts"]
	}

	commitResp, err := cli.ContainerCommit(ctx, containerID, container.CommitOptions{
		Reference: tempImageTag,
		Comment:   "RF Swift: temporary image for container property update",
		Pause:     true,
		Config: &container.Config{
			ExposedPorts: ParseExposedPorts(props["ExposedPorts"]),
			Labels:       commitLabels,
		},
	})
	
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to commit container: %v", err))
		return err
	}
	common.PrintSuccessMessage(fmt.Sprintf("Committed as: %s (ID: %s)", tempImageTag, commitResp.ID[:12]))

	// ── 2. Remove old container ──
	common.PrintInfoMessage("Removing old container...")
	if err := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to remove container: %v", err))
		return err
	}
	common.PrintSuccessMessage("Old container removed.")

	// ── 3. Rebuild container config from inspected data + prop overrides ──

	// Determine the original image name for label tracking.
	originalImageName := containerJSON.Config.Image
	if label, ok := containerJSON.Config.Labels["org.rfswift.original_image"]; ok && label != "" {
		originalImageName = label
	}

	// Parse properties for host config
	bindings := []string{}
	if props["Bindings"] != "" {
	    bindings = strings.Split(props["Bindings"], ";;")
	}

	extrahosts := []string{}
	if props["ExtraHosts"] != "" {
		extrahosts = strings.Split(props["ExtraHosts"], ",")
	}

	// Rebuild environment — preserve ALL original env vars, update DISPLAY
	var dockerenv []string
	displaySet := false
	for _, env := range containerJSON.Config.Env {
		if strings.HasPrefix(env, "DISPLAY=") {
			if props["XDisplay"] != "" {
				dockerenv = append(dockerenv, fmt.Sprintf("DISPLAY=%s", props["XDisplay"]))
			} else {
				dockerenv = append(dockerenv, env) // keep original
			}
			displaySet = true
		} else {
			dockerenv = append(dockerenv, env)
		}
	}
	if !displaySet && props["XDisplay"] != "" {
		dockerenv = append(dockerenv, fmt.Sprintf("DISPLAY=%s", props["XDisplay"]))
	}

	exposedPorts := ParseExposedPorts(props["ExposedPorts"])
	bindedPorts := ParseBindedPorts(props["PortBindings"])
	devices := getDeviceMappingsFromString(props["Devices"])

	privileged := props["Privileged"] == "true"

	hostConfig := &container.HostConfig{
		NetworkMode:  container.NetworkMode(props["NetworkMode"]),
		Binds:        bindings,
		ExtraHosts:   extrahosts,
		PortBindings: bindedPorts,
		Privileged:   privileged,
	}

	// Handle ulimits
	if props["Ulimits"] != "" {
		hostConfig.Resources.Ulimits = parseUlimitsFromString(props["Ulimits"])
	}

	if !privileged {
		hostConfig.Devices = devices

		if props["Cgroups"] != "" {
			hostConfig.DeviceCgroupRules = strings.Split(props["Cgroups"], ",")
		}
		if props["Seccomp"] != "" && props["Seccomp"] != "(Default)" {
			hostConfig.SecurityOpt = []string{"seccomp=" + props["Seccomp"]}
		}
		if props["Caps"] != "" {
			hostConfig.CapAdd = strings.Split(props["Caps"], ",")
		}
	}

	// ── Restore cgroup rules from label if inspect returned empty ──
	if len(hostConfig.DeviceCgroupRules) == 0 {
		if label, ok := containerJSON.Config.Labels["org.rfswift.cgroup_rules"]; ok && label != "" {
			hostConfig.DeviceCgroupRules = strings.Split(label, ",")
		}
	}

	// Build labels — preserve existing + update tracking labels
	containerLabels := make(map[string]string)
	for k, v := range containerJSON.Config.Labels {
		containerLabels[k] = v
	}
	containerLabels["org.container.project"] = "rfswift"
	containerLabels["org.rfswift.original_image"] = originalImageName
	if len(hostConfig.DeviceCgroupRules) > 0 {
		containerLabels["org.rfswift.cgroup_rules"] = strings.Join(hostConfig.DeviceCgroupRules, ",")
	}

	if props["ExposedPorts"] == "" {
		containerLabels["org.rfswift.exposed_ports"] = "none"
	} else {
		containerLabels["org.rfswift.exposed_ports"] = props["ExposedPorts"]
	}

	// Determine shell
	shell := props["Shell"]
	if shell == "" {
		shell = containerJSON.Path
	}
	if shell == "" {
		shell = "/bin/bash"
	}

	containerConfig := &container.Config{
		Image:        tempImageTag, // ← use committed snapshot
		Cmd:          []string{shell},
		Env:          dockerenv,
		ExposedPorts: exposedPorts,
		OpenStdin:    true,
		StdinOnce:    false,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Labels:       containerLabels,
	}

	// Preserve entrypoint
	if len(containerJSON.Config.Entrypoint) > 0 {
		containerConfig.Entrypoint = containerJSON.Config.Entrypoint
	}

	// ── Sanitize HostConfig for Podman cgroup v2 compat ──
	if !EngineSupportsDirectConfigEdit() {
		sanitizeHostConfigForPodman(hostConfig)
	}

	// ── 4. Create the new container ──
	common.PrintInfoMessage("Creating new container with updated configuration...")

	tempContainerName := fmt.Sprintf("%s_rfswift_tmp_%d", containerName, time.Now().UnixNano())

	var newContainerID string

	// Podman: use native CLI when cgroup rules are present
	if len(hostConfig.DeviceCgroupRules) > 0 && !EngineSupportsDirectConfigEdit() {
		cid, err := podmanCreateViaCLI(tempContainerName, tempImageTag, containerConfig, hostConfig)
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to create container via Podman CLI: %v", err))
			// ── ROLLBACK ──
			rollbackContainer(ctx, cli, containerName, tempImageTag, containerJSON)
			return err
		}
		newContainerID = cid
	} else {
		// ── Compat API path ──
		// CRITICAL: pass nil for networking and platform.
		// Podman's compat API rejects empty structs like &network.NetworkingConfig{}.
		resp, err := cli.ContainerCreate(ctx,
			containerConfig,
			hostConfig,
			nil, // networking — must be nil, NOT &network.NetworkingConfig{}
			nil, // platform
			tempContainerName,
		)
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to create new container: %v", err))
			// ── ROLLBACK ──
			rollbackContainer(ctx, cli, containerName, tempImageTag, containerJSON)
			return err
		}
		newContainerID = resp.ID
	}
	common.PrintSuccessMessage(fmt.Sprintf("New container created: %s", newContainerID[:12]))

	// ── 5. Rename temp container to original name ──
	common.PrintInfoMessage(fmt.Sprintf("Renaming container to '%s'...", containerName))
	if err := cli.ContainerRename(ctx, newContainerID, containerName); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to rename container: %v", err))
		return err
	}

	// ── 6. Start the new container ──
	common.PrintInfoMessage("Starting new container...")
	if err := cli.ContainerStart(ctx, newContainerID, container.StartOptions{}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to start new container: %v", err))
		return err
	}

	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' updated successfully!", containerName))
	return nil
}

// rollbackContainer attempts to recreate a container from the committed image
// when the primary creation fails. This prevents leaving the user with a
// deleted container and no way to recover (except manually).
func rollbackContainer(ctx context.Context, cli *client.Client, containerName string, tempImageTag string, originalJSON types.ContainerJSON) {
	common.PrintWarningMessage("Creation failed — attempting rollback from committed image...")

	// Try to create a basic container from the committed image with minimal config
	// (avoid the fields that may have caused the original failure)
	rollbackHostConfig := &container.HostConfig{
	    NetworkMode: originalJSON.HostConfig.NetworkMode,
	    Binds:       originalJSON.HostConfig.Binds,
	    Privileged:  originalJSON.HostConfig.Privileged,
	    CapAdd:      originalJSON.HostConfig.CapAdd,
	    SecurityOpt: originalJSON.HostConfig.SecurityOpt,
	}

	// Sanitize for Podman if needed
	if !EngineSupportsDirectConfigEdit() {
		sanitizeHostConfigForPodman(rollbackHostConfig)
	}

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:     tempImageTag,
			OpenStdin: true,
			Tty:       true,
			Labels:    originalJSON.Config.Labels,
			Env:       originalJSON.Config.Env,
		},
		rollbackHostConfig,
		nil, nil,
		containerName,
	)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("rollback also failed: %v", err))
		common.PrintWarningMessage(fmt.Sprintf("Your container state is preserved in image: %s", tempImageTag))
		common.PrintWarningMessage(fmt.Sprintf("Manual recovery: podman create --name %s -it %s", containerName, tempImageTag))
		return
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		common.PrintWarningMessage(fmt.Sprintf("Rollback container created but failed to start: %v", err))
		common.PrintWarningMessage(fmt.Sprintf("Try manually: podman start %s", containerName))
	} else {
		common.PrintSuccessMessage("Rollback successful — container restored with original configuration")
		common.PrintInfoMessage("The requested change was NOT applied. Please check the error above and retry.")
	}
}

// Check if a device mapping already exists
func deviceExists(devices []DeviceMapping, hostPath string, containerPath string) bool {
	for _, device := range devices {
		if device.PathOnHost == hostPath && device.PathInContainer == containerPath {
			return true
		}
	}
	return false
}

// Remove a device mapping from a slice
func removeDeviceFromSlice(devices []DeviceMapping, hostPath string, containerPath string) []DeviceMapping {
	var result []DeviceMapping
	for _, device := range devices {
		if device.PathOnHost != hostPath || device.PathInContainer != containerPath {
			result = append(result, device)
		}
	}
	return result
}

// Add a device mapping to config.v2.json
func addDeviceMapping(config map[string]interface{}, hostPath string, containerPath string) {
	// Check if "HostConfig" exists in the config
	hostConfig, ok := config["HostConfig"].(map[string]interface{})
	if !ok {
		// Create HostConfig if it doesn't exist
		hostConfig = make(map[string]interface{})
		config["HostConfig"] = hostConfig
	}

	// Get existing devices or create new devices array
	devices, ok := hostConfig["Devices"].([]interface{})
	if !ok {
		devices = make([]interface{}, 0)
	}

	// Create a new device mapping
	newDevice := map[string]interface{}{
		"PathOnHost":        hostPath,
		"PathInContainer":   containerPath,
		"CgroupPermissions": "rwm", // Default permissions
	}

	// Check if the device already exists
	exists := false
	for _, device := range devices {
		if deviceMap, ok := device.(map[string]interface{}); ok {
			if deviceMap["PathOnHost"] == hostPath && deviceMap["PathInContainer"] == containerPath {
				exists = true
				break
			}
		}
	}

	// Add the new device mapping if it doesn't exist
	if !exists {
		devices = append(devices, newDevice)
		hostConfig["Devices"] = devices
	}
}

// Remove a device mapping from config.v2.json
func removeDeviceMapping(config map[string]interface{}, hostPath string, containerPath string) {
	// Check if "HostConfig" exists in the config
	hostConfig, ok := config["HostConfig"].(map[string]interface{})
	if !ok {
		return // No host config
	}

	// Get existing devices
	devices, ok := hostConfig["Devices"].([]interface{})
	if !ok {
		return // No devices
	}

	// Filter out the device to remove
	var updatedDevices []interface{}
	for _, device := range devices {
		if deviceMap, ok := device.(map[string]interface{}); ok {
			if deviceMap["PathOnHost"] != hostPath || deviceMap["PathInContainer"] != containerPath {
				updatedDevices = append(updatedDevices, device)
			}
		}
	}

	// Update the devices list
	hostConfig["Devices"] = updatedDevices
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
	cli, err := NewEngineClient()
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

func DockerUpgrade(containerIdentifier string, repositoriesToPreserve string, newImage string) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get latest container if not specified
	if containerIdentifier == "" {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		containerIdentifier = latestDockerID(labelKey, labelValue)
		if containerIdentifier == "" {
			return fmt.Errorf("no container found with label")
		}
	}

	// Get container info
	containerJSON, err := cli.ContainerInspect(ctx, containerIdentifier)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}

	containerName := strings.TrimPrefix(containerJSON.Name, "/")
	originalImage := containerJSON.Config.Image

	// Determine new image
	if newImage == "" {
		// If no image specified, use the current image's latest version
		repo, _ := parseImageName(originalImage)
		newImage = fmt.Sprintf("%s:latest", repo)
	} else {
		// Normalize the provided image name
		newImage = normalizeImageName(newImage)
	}

	common.PrintInfoMessage("═══════════════════════════════════════")
	common.PrintInfoMessage("       Container Upgrade Process")
	common.PrintInfoMessage("═══════════════════════════════════════")
	fmt.Printf("  Container: %s\n", containerName)
	fmt.Printf("  Current image: %s\n", originalImage)
	fmt.Printf("  Target image: %s\n", newImage)
	common.PrintInfoMessage("═══════════════════════════════════════")
	fmt.Println()

	// STEP 1: Check if new image exists locally, if not pull it FIRST
	// This prevents removing the old container if the pull fails
	_, _, err = cli.ImageInspectWithRaw(ctx, newImage)
	if err != nil {
		common.PrintInfoMessage(fmt.Sprintf("Pulling image '%s'...", newImage))

		// Parse image for pulling
		parts := strings.Split(newImage, ":")
		repo := parts[0]
		tag := "latest"
		if len(parts) > 1 {
			tag = parts[1]
		}

		// Pull the image
		architecture := getArchitecture()
		isOfficial := IsOfficialImage(newImage)
		actualPullRef := newImage

		if isOfficial && architecture != "" {
			hasArchSuffix := strings.HasSuffix(tag, "_amd64") ||
				strings.HasSuffix(tag, "_arm64") ||
				strings.HasSuffix(tag, "_riscv64") ||
				strings.HasSuffix(tag, "_arm")

			if !hasArchSuffix {
				actualPullRef = fmt.Sprintf("%s:%s_%s", repo, tag, architecture)
			}
		}

		out, err := cli.ImagePull(ctx, actualPullRef, image.PullOptions{})
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to pull image: %v", err))
			common.PrintInfoMessage("Old container preserved - no changes made")
			return err
		}
		defer out.Close()

		// Process pull output
		fd, isTerminal := term.GetFdInfo(os.Stdout)
		jsonDecoder := json.NewDecoder(out)
		for {
			var msg jsonmessage.JSONMessage
			if err := jsonDecoder.Decode(&msg); err == io.EOF {
				break
			} else if err != nil {
				common.PrintErrorMessage(err)
				common.PrintInfoMessage("Old container preserved - no changes made")
				return err
			}
			if isTerminal {
				_ = jsonmessage.DisplayJSONMessagesStream(out, os.Stdout, fd, isTerminal, nil)
			}
		}

		// Tag if needed
		if newImage != actualPullRef && IsOfficialImage(actualPullRef) {
			remoteInspect, _, _ := cli.ImageInspectWithRaw(ctx, actualPullRef)
			if remoteInspect.ID != "" {
				cli.ImageTag(ctx, remoteInspect.ID, newImage)
				cli.ImageRemove(ctx, actualPullRef, image.RemoveOptions{Force: false})
			}
		}

		common.PrintSuccessMessage(fmt.Sprintf("Image '%s' pulled successfully", newImage))
		fmt.Println()
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Using local image: %s", newImage))
		fmt.Println()
	}

	// Parse repositories to preserve
	var reposToCopy []string
	if repositoriesToPreserve != "" {
		reposToCopy = strings.Split(repositoriesToPreserve, ",")
		for i, repo := range reposToCopy {
			reposToCopy[i] = strings.TrimSpace(repo)
		}
		common.PrintInfoMessage("Repositories/directories to preserve:")
		for _, repo := range reposToCopy {
			fmt.Printf("  • %s\n", repo)
		}
		fmt.Println()
	}

	// Create temporary directory to store preserved data
	var tempDir string
	var preservedData = make(map[string]string) // map[containerPath]hostTempPath
	
	if len(reposToCopy) > 0 {
		tempDir, err = os.MkdirTemp("", "rfswift-upgrade-*")
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to create temp directory: %v", err))
			return err
		}
		defer os.RemoveAll(tempDir) // Clean up on exit

		common.PrintInfoMessage(fmt.Sprintf("Created temporary storage: %s", tempDir))
		
		// Ensure container is running before copying
		if !containerJSON.State.Running {
			common.PrintInfoMessage("Starting container to copy data...")
			if err := cli.ContainerStart(ctx, containerIdentifier, container.StartOptions{}); err != nil {
				common.PrintErrorMessage(fmt.Errorf("failed to start container: %v", err))
				return err
			}
		}

		// Copy data from old container to temp directory
		for _, repoPath := range reposToCopy {
			common.PrintInfoMessage(fmt.Sprintf("Backing up: %s", repoPath))
			
			// Create subdirectory in temp for this path
			safeName := strings.ReplaceAll(strings.Trim(repoPath, "/"), "/", "_")
			hostPath := filepath.Join(tempDir, safeName)
			
			// Check if directory exists in container
			checkCmd := fmt.Sprintf("[ -d '%s' ] && echo 'exists' || echo 'not_found'", repoPath)
			exists, err := execCommandWithOutput(ctx, cli, containerIdentifier, []string{"/bin/bash", "-c", checkCmd})
			if err != nil || !strings.Contains(exists, "exists") {
				common.PrintWarningMessage(fmt.Sprintf("Directory '%s' not found in container, skipping", repoPath))
				continue
			}

			// Use docker cp to copy from container to host
			reader, _, err := cli.CopyFromContainer(ctx, containerIdentifier, repoPath)
			if err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to copy %s: %v", repoPath, err))
				continue
			}
			defer reader.Close()

			// Create the host directory
			if err := os.MkdirAll(hostPath, 0755); err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to create directory %s: %v", hostPath, err))
				continue
			}

			// Extract the tar archive
			if err := extractTarArchive(reader, hostPath); err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to extract %s: %v", repoPath, err))
				continue
			}

			preservedData[repoPath] = hostPath
			common.PrintSuccessMessage(fmt.Sprintf("Backed up: %s → %s", repoPath, hostPath))
		}
		fmt.Println()
	}

	// Create backup of current container
	currentTime := time.Now()
	backupTag := fmt.Sprintf("%s-backup-%02d%02d%d-%02d%02d%02d",
		originalImage,
		currentTime.Day(),
		currentTime.Month(),
		currentTime.Year(),
		currentTime.Hour(),
		currentTime.Minute(),
		currentTime.Second())

	common.PrintInfoMessage("Creating backup of current container...")
	_, err = cli.ContainerCommit(ctx, containerIdentifier, container.CommitOptions{
		Reference: backupTag,
	})
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to create backup: %v", err))
		return err
	}
	common.PrintSuccessMessage(fmt.Sprintf("Backup created: %s", backupTag))

	// Stop the container
	common.PrintInfoMessage("Stopping container...")
	timeout := 10
	if err := cli.ContainerStop(ctx, containerIdentifier, container.StopOptions{Timeout: &timeout}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to stop container: %v", err))
		return err
	}
	common.PrintSuccessMessage("Container stopped")

	// Remove the old container
	common.PrintInfoMessage("Removing old container...")
	if err := cli.ContainerRemove(ctx, containerIdentifier, container.RemoveOptions{Force: true}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to remove container: %v", err))
		return err
	}
	common.PrintSuccessMessage("Old container removed")

	// Get original container properties
	props := make(map[string]string)
	props["Shell"] = containerJSON.Path
	if props["Shell"] == "" {
		props["Shell"] = "/bin/bash"
	}
	props["Privileged"] = fmt.Sprintf("%v", containerJSON.HostConfig.Privileged)
	props["NetworkMode"] = string(containerJSON.HostConfig.NetworkMode)
	props["ExposedPorts"] = convertExposedPortsToString(containerJSON.Config.ExposedPorts)
	props["PortBindings"] = convertPortBindingsToRoundTrip(containerJSON.HostConfig.PortBindings)
	props["ExtraHosts"] = strings.Join(containerJSON.HostConfig.ExtraHosts, ",")
	props["Devices"] = convertDevicesToString(containerJSON.HostConfig.Devices)
	props["Caps"] = convertCapsToString(containerJSON.HostConfig.CapAdd)
	props["Seccomp"] = convertSecurityOptToString(containerJSON.HostConfig.SecurityOpt)
	props["Cgroups"] = strings.Join(containerJSON.HostConfig.DeviceCgroupRules, ",")
	props["XDisplay"] = ":0"
	for _, env := range containerJSON.Config.Env {
		if strings.HasPrefix(env, "DISPLAY=") {
			props["XDisplay"] = strings.TrimPrefix(env, "DISPLAY=")
			break
		}
	}

	// Preserve all existing bindings
	bindingsToKeep := containerJSON.HostConfig.Binds

	// Create new container with new image
	common.PrintInfoMessage("Creating new container with upgraded image...")

	extrahosts := []string{}
	if props["ExtraHosts"] != "" {
		extrahosts = strings.Split(props["ExtraHosts"], ",")
	}

	dockerenv := []string{fmt.Sprintf("DISPLAY=%s", props["XDisplay"])}
	if dockerObj.pulse_server != "" {
		dockerenv = append(dockerenv, "PULSE_SERVER="+dockerObj.pulse_server)
	}

	exposedPorts := ParseExposedPorts(props["ExposedPorts"])
	bindedPorts := ParseBindedPorts(props["PortBindings"])
	devices := getDeviceMappingsFromString(props["Devices"])

	privileged := props["Privileged"] == "true"

	// Create HostConfig WITHOUT Devices in the literal
	hostConfig := &container.HostConfig{
		NetworkMode:  container.NetworkMode(props["NetworkMode"]),
		Binds:        bindingsToKeep,
		ExtraHosts:   extrahosts,
		PortBindings: bindedPorts,
		Privileged:   privileged,
	}

	// Set devices and other settings based on privilege mode
	if !privileged {
		hostConfig.Devices = devices

		if props["Cgroups"] != "" {
			hostConfig.DeviceCgroupRules = strings.Split(props["Cgroups"], ",")
		}
		if props["Seccomp"] != "" && props["Seccomp"] != "(Default)" {
			hostConfig.SecurityOpt = []string{"seccomp=" + props["Seccomp"]}
		}
		if props["Caps"] != "" {
			hostConfig.CapAdd = strings.Split(props["Caps"], ",")
		}
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:        newImage,
		Cmd:          []string{props["Shell"]},
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
	}, hostConfig, &network.NetworkingConfig{}, nil, containerName)

	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to create new container: %v", err))
		common.PrintWarningMessage(fmt.Sprintf("Restore from backup: docker run %s", backupTag))
		return err
	}

	common.PrintSuccessMessage(fmt.Sprintf("New container '%s' created", containerName))

	// Start the new container
	common.PrintInfoMessage("Starting new container...")
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to start new container: %v", err))
		return err
	}
	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' started successfully", containerName))

	// Restore preserved data to new container
	if len(preservedData) > 0 {
		fmt.Println()
		common.PrintInfoMessage("Restoring preserved repositories to new container...")
		
		for containerPath, hostPath := range preservedData {
			common.PrintInfoMessage(fmt.Sprintf("Restoring: %s", containerPath))
			
			// Create tar archive from host path
			tarReader, err := createTarArchive(hostPath, containerPath)
			if err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to create archive for %s: %v", containerPath, err))
				continue
			}

			// Copy to new container
			err = cli.CopyToContainer(ctx, resp.ID, filepath.Dir(containerPath), tarReader, container.CopyToContainerOptions{})
			tarReader.Close()
			
			if err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to restore %s: %v", containerPath, err))
				continue
			}

			common.PrintSuccessMessage(fmt.Sprintf("Restored: %s", containerPath))
		}
	}

	// Print summary
	fmt.Println()
	common.PrintInfoMessage("═══════════════════════════════════════")
	common.PrintSuccessMessage("✓ Container upgrade completed!")
	fmt.Printf("  Container: %s\n", containerName)
	fmt.Printf("  Old image: %s\n", originalImage)
	fmt.Printf("  New image: %s\n", newImage)
	fmt.Printf("  Backup: %s\n", backupTag)
	fmt.Printf("  Host bindings: %d\n", len(bindingsToKeep))
	fmt.Printf("  Repositories restored: %d\n", len(preservedData))
	common.PrintInfoMessage("═══════════════════════════════════════")

	return nil
}

// Helper function to extract tar archive
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

// Helper function to create tar archive
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

// ExportContainer exports a container to a tar.gz file
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

// ExportImage exports one or more images to a tar.gz file
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

// ImportContainer imports a container from a tar.gz file and creates an image
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

// ImportImage imports one or more images from a tar.gz file
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


// SaveImageToFile pulls an image and saves it to a tar.gz file
func SaveImageToFile(imageName string, outputFile string, pullFirst bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Normalize image name
	imageName = normalizeImageName(imageName)

	// Check if image exists locally
	_, _, err = cli.ImageInspectWithRaw(ctx, imageName)
	imageExists := err == nil

	if !imageExists || pullFirst {
		// Need to pull the image
		if !imageExists {
			common.PrintInfoMessage(fmt.Sprintf("Image '%s' not found locally, pulling...", imageName))
		} else {
			common.PrintInfoMessage(fmt.Sprintf("Pulling latest version of '%s'...", imageName))
		}

		// Parse image name for architecture handling
		parts := strings.Split(imageName, ":")
		repo := parts[0]
		tag := "latest"
		if len(parts) > 1 {
			tag = parts[1]
		}

		// Check if this is an official image
		isOfficial := IsOfficialImage(imageName)
		architecture := getArchitecture()
		actualPullRef := imageName

		// Handle architecture suffix for official images
		if isOfficial && architecture != "" {
			hasArchSuffix := strings.HasSuffix(tag, "_amd64") ||
				strings.HasSuffix(tag, "_arm64") ||
				strings.HasSuffix(tag, "_riscv64") ||
				strings.HasSuffix(tag, "_arm")

			if !hasArchSuffix {
				actualPullRef = fmt.Sprintf("%s:%s_%s", repo, tag, architecture)
				common.PrintInfoMessage(fmt.Sprintf("Using architecture-specific tag: %s", actualPullRef))
			}
		}

		// Pull the image
		out, err := cli.ImagePull(ctx, actualPullRef, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("failed to pull image: %v", err)
		}
		defer out.Close()

		// Show pull progress
		termFd, isTerm := term.GetFdInfo(os.Stdout)
		if isTerm {
			jsonmessage.DisplayJSONMessagesStream(out, os.Stdout, termFd, isTerm, nil)
		} else {
			// Read to completion even if not displaying
			io.Copy(io.Discard, out)
		}

		// Tag if needed (for official images with architecture suffix)
		if actualPullRef != imageName && isOfficial {
			remoteInspect, _, _ := cli.ImageInspectWithRaw(ctx, actualPullRef)
			if remoteInspect.ID != "" {
				cli.ImageTag(ctx, remoteInspect.ID, imageName)
				common.PrintSuccessMessage(fmt.Sprintf("Tagged as: %s", imageName))
			}
		}

		common.PrintSuccessMessage("Image pulled successfully")
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Using local image: %s", imageName))
	}

	// Now save the image to file
	common.PrintInfoMessage(fmt.Sprintf("Saving image '%s' to %s", imageName, outputFile))

	// Save image
	reader, err := cli.ImageSave(ctx, []string{imageName})
	if err != nil {
		return fmt.Errorf("failed to save image: %v", err)
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

	common.PrintSuccessMessage(fmt.Sprintf("Image saved successfully: %s (%.2f MB)", 
		outputFile, float64(written)/(1024*1024)))
	
	// Show file info
	fileInfo, _ := os.Stat(outputFile)
	if fileInfo != nil {
		common.PrintInfoMessage(fmt.Sprintf("Compressed file size: %.2f MB", float64(fileInfo.Size())/(1024*1024)))
	}

	return nil
}

// parseDuration parses duration strings like "24h", "7d", "1m", "1y"
func parseDuration(duration string) (time.Duration, error) {
	if duration == "" {
		return 0, nil
	}

	// Extract number and unit
	var value int
	var unit string
	_, err := fmt.Sscanf(duration, "%d%s", &value, &unit)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format: %s (use format like '24h', '7d', '1m', '1y')", duration)
	}

	switch unit {
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	case "m":
		return time.Duration(value) * 30 * 24 * time.Hour, nil // Approximate month
	case "y":
		return time.Duration(value) * 365 * 24 * time.Hour, nil // Approximate year
	default:
		return 0, fmt.Errorf("invalid duration unit: %s (use h, d, m, or y)", unit)
	}
}

// CleanupAll removes both old containers and images
func CleanupAll(olderThan string, force bool, dryRun bool) error {
	common.PrintInfoMessage("Cleaning up containers and images...")
	
	if err := CleanupContainers(olderThan, force, dryRun, false); err != nil {
		return err
	}
	
	fmt.Println()
	
	if err := CleanupImages(olderThan, force, dryRun, false, true); err != nil {  // Enable pruneChildren for cleanup all
		return err
	}
	
	return nil
}

// CleanupContainers removes old containers
func CleanupContainers(olderThan string, force bool, dryRun bool, onlyStopped bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Parse duration
	duration, err := parseDuration(olderThan)
	if err != nil {
		return err
	}

	cutoffTime := time.Now().Add(-duration)

	// List containers
	containerFilters := filters.NewArgs()
	containerFilters.Add("label", "org.container.project=rfswift")
	
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: containerFilters,
	})
	if err != nil {
		return fmt.Errorf("failed to list containers: %v", err)
	}

	// Filter containers
	var toDelete []types.Container
	for _, cont := range containers {
		// Check if stopped only
		if onlyStopped && cont.State == "running" {
			continue
		}

		// Check age
		created := time.Unix(cont.Created, 0)
		if olderThan != "" && created.After(cutoffTime) {
			continue
		}

		toDelete = append(toDelete, cont)
	}

	if len(toDelete) == 0 {
		common.PrintInfoMessage("No containers to remove")
		return nil
	}

	// Display containers to delete
	cyan := "\033[36m"
	reset := "\033[0m"
	fmt.Printf("%s🗑️  Containers to remove: %d%s\n", cyan, len(toDelete), reset)
	
	for _, cont := range toDelete {
		age := time.Since(time.Unix(cont.Created, 0))
		containerName := ""
		if len(cont.Names) > 0 {
			containerName = cont.Names[0]
			if len(containerName) > 0 && containerName[0] == '/' {
				containerName = containerName[1:]
			}
		} else {
			containerName = cont.ID[:12]
		}
		
		status := cont.State
		if status == "running" {
			status = "\033[32m" + status + "\033[0m"
		} else {
			status = "\033[31m" + status + "\033[0m"
		}
		
		fmt.Printf("  • %s (%s) - Age: %s - Status: %s\n", 
			containerName, cont.ID[:12], formatAge(age), status)
	}
	fmt.Println()

	if dryRun {
		common.PrintWarningMessage("DRY RUN: No containers were actually removed")
		return nil
	}

	// Ask for confirmation
	if !force {
		reader := bufio.NewReader(os.Stdin)
		common.PrintWarningMessage(fmt.Sprintf("Are you sure you want to remove %d container(s)? (y/n): ", len(toDelete)))
		response, _ := reader.ReadString('\n')
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			common.PrintInfoMessage("Cleanup cancelled")
			return nil
		}
	}

	// Remove containers
	removed := 0
	for _, cont := range toDelete {
	    containerName := ""
	    if len(cont.Names) > 0 {
	        containerName = cont.Names[0]
	        if len(containerName) > 0 && containerName[0] == '/' {
	            containerName = containerName[1:]
	        }
	    } else {
	        containerName = cont.ID[:12]
	    }

		err := cli.ContainerRemove(ctx, cont.ID, container.RemoveOptions{Force: true})
		if err != nil {
			common.PrintWarningMessage(fmt.Sprintf("Failed to remove %s: %v", containerName, err))
		} else {
			common.PrintSuccessMessage(fmt.Sprintf("Removed container: %s", containerName))
			removed++
		}
	}

	common.PrintSuccessMessage(fmt.Sprintf("Cleanup complete: removed %d/%d container(s)", removed, len(toDelete)))
	return nil
}

// CleanupImages removes old images
func CleanupImages(olderThan string, force bool, dryRun bool, onlyDangling bool, pruneChildren bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Parse duration
	duration, err := parseDuration(olderThan)
	if err != nil {
		return err
	}

	cutoffTime := time.Now().Add(-duration)

	// List images
	imageFilters := filters.NewArgs()
	imageFilters.Add("label", "org.container.project=rfswift")
	if onlyDangling {
		imageFilters.Add("dangling", "true")
	}

	images, err := cli.ImageList(ctx, image.ListOptions{
		All:     true,
		Filters: imageFilters,
	})
	if err != nil {
		return fmt.Errorf("failed to list images: %v", err)
	}

	// Filter images
	var toDelete []image.Summary
	for _, img := range images {
		// Skip if no RepoTags (dangling) unless we want dangling only
		if !onlyDangling && len(img.RepoTags) == 0 {
			continue
		}

		// Check age
		created := time.Unix(img.Created, 0)
		if olderThan != "" && created.After(cutoffTime) {
			continue
		}

		toDelete = append(toDelete, img)
	}

	if len(toDelete) == 0 {
		common.PrintInfoMessage("No images to remove")
		return nil
	}

	// Display images to delete
	magenta := "\033[35m"
	reset := "\033[0m"
	fmt.Printf("%s🗑️  Images to remove: %d%s\n", magenta, len(toDelete), reset)
	
	// Check for child images
	var hasChildren bool
	totalDescendants := 0
	
	for _, img := range toDelete {
		age := time.Since(time.Unix(img.Created, 0))
		size := float64(img.Size) / (1024 * 1024)
		
		var displayName string
		if len(img.RepoTags) > 0 {
			displayName = img.RepoTags[0]
		} else {
			displayName = fmt.Sprintf("<none> (%s)", img.ID[7:19])
		}
		
		// Check if image has descendants (children, grandchildren, etc.)
		descendants := getAllDescendants(ctx, cli, img.ID)
		if len(descendants) > 0 {
			hasChildren = true
			totalDescendants += len(descendants)
			fmt.Printf("  • %s - Age: %s - Size: %.2f MB - ⚠️  %d descendant(s)\n", 
				displayName, formatAge(age), size, len(descendants))
		} else {
			fmt.Printf("  • %s - Age: %s - Size: %.2f MB\n", 
				displayName, formatAge(age), size)
		}
	}
	fmt.Println()

	if hasChildren {
		if pruneChildren {
			common.PrintInfoMessage(fmt.Sprintf("Will remove %d total descendant images", totalDescendants))
		} else {
			common.PrintWarningMessage("Some images have dependent descendant images. Use --prune-children to remove them first.")
		}
	}

	if dryRun {
		common.PrintWarningMessage("DRY RUN: No images were actually removed")
		return nil
	}

	// Ask for confirmation
	if !force {
		reader := bufio.NewReader(os.Stdin)
		totalToRemove := len(toDelete)
		if pruneChildren {
			totalToRemove += totalDescendants
		}
		common.PrintWarningMessage(fmt.Sprintf("Are you sure you want to remove %d image(s) in total? (y/n): ", totalToRemove))
		response, _ := reader.ReadString('\n')
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			common.PrintInfoMessage("Cleanup cancelled")
			return nil
		}
	}

	// Remove images
	removed := 0
	skipped := 0
	
	for _, img := range toDelete {
		var displayName string
		if len(img.RepoTags) > 0 {
			displayName = img.RepoTags[0]
		} else {
			displayName = img.ID[7:19]
		}

		// Check if has descendants
		descendants := getAllDescendants(ctx, cli, img.ID)
		
		if len(descendants) > 0 && !pruneChildren {
			common.PrintWarningMessage(fmt.Sprintf("Skipped %s: has %d descendant(s) (use --prune-children)", displayName, len(descendants)))
			skipped++
			continue
		}

		// Remove with all descendants if requested
		if pruneChildren {
			descendantsRemoved, err := removeImageWithDescendants(ctx, cli, img.ID, displayName)
			if err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to remove %s: %v", displayName, err))
			} else {
				common.PrintSuccessMessage(fmt.Sprintf("Removed image: %s (+ %d descendants)", displayName, descendantsRemoved))
				removed++
			}
		} else {
			// Simple removal without descendants
			_, err := cli.ImageRemove(ctx, img.ID, image.RemoveOptions{Force: true})
			if err != nil {
				if strings.Contains(err.Error(), "No such image") {
					// Already removed
					common.PrintSuccessMessage(fmt.Sprintf("Removed image: %s (cascaded)", displayName))
					removed++
				} else {
					common.PrintWarningMessage(fmt.Sprintf("Failed to remove %s: %v", displayName, err))
				}
			} else {
				common.PrintSuccessMessage(fmt.Sprintf("Removed image: %s", displayName))
				removed++
			}
		}
	}

	if skipped > 0 {
		common.PrintInfoMessage(fmt.Sprintf("Skipped %d image(s) with descendants", skipped))
	}
	common.PrintSuccessMessage(fmt.Sprintf("Cleanup complete: removed %d/%d parent image(s)", removed, len(toDelete)))
	return nil
}

// getChildImages returns all images that depend on the given parent image
func getChildImages(ctx context.Context, cli *client.Client, parentID string) []image.Summary {
	var children []image.Summary
	
	// Get all images
	allImages, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return children
	}

	// Find children
	for _, img := range allImages {
		if img.ParentID == parentID {
			children = append(children, img)
		}
	}

	return children
}

// getAllDescendants recursively gets all descendant images
func getAllDescendants(ctx context.Context, cli *client.Client, parentID string) []image.Summary {
	var descendants []image.Summary
	
	// Get direct children
	children := getChildImages(ctx, cli, parentID)
	
	for _, child := range children {
		// Add this child
		descendants = append(descendants, child)
		// Recursively get its descendants
		grandchildren := getAllDescendants(ctx, cli, child.ID)
		descendants = append(descendants, grandchildren...)
	}
	
	return descendants
}

// removeImageWithDescendants removes an image and all its descendants recursively
func removeImageWithDescendants(ctx context.Context, cli *client.Client, imageID string, displayName string) (int, error) {
	removedCount := 0
	
	// Get all descendants (children, grandchildren, etc.)
	descendants := getAllDescendants(ctx, cli, imageID)
	
	if len(descendants) > 0 {
		common.PrintInfoMessage(fmt.Sprintf("Removing %d descendant image(s) for %s...", len(descendants), displayName))
		
		// Remove descendants in reverse order (deepest first)
		for i := len(descendants) - 1; i >= 0; i-- {
			desc := descendants[i]
			
			// Check if image still exists (might have been removed as a side effect)
			_, _, err := cli.ImageInspectWithRaw(ctx, desc.ID)
			if err != nil {
				// Image doesn't exist anymore, skip it
				continue
			}
			
			var descName string
			if len(desc.RepoTags) > 0 {
				descName = desc.RepoTags[0]
			} else {
				descName = desc.ID[7:19]
			}
			
			_, err = cli.ImageRemove(ctx, desc.ID, image.RemoveOptions{Force: true, PruneChildren: true})
			if err != nil {
				// Check if error is because image doesn't exist
				if strings.Contains(err.Error(), "No such image") {
					// Already removed as a side effect
					continue
				}
				common.PrintWarningMessage(fmt.Sprintf("  Failed to remove descendant %s: %v", descName, err))
			} else {
				common.PrintSuccessMessage(fmt.Sprintf("  Removed descendant: %s", descName))
				removedCount++
			}
		}
	}
	
	// Check if parent image still exists
	_, _, err := cli.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		// Image was already removed as a side effect
		common.PrintSuccessMessage(fmt.Sprintf("Image %s was already removed (cascaded)", displayName))
		return removedCount, nil
	}
	
	// Now remove the parent image
	_, err = cli.ImageRemove(ctx, imageID, image.RemoveOptions{Force: true, PruneChildren: true})
	if err != nil && strings.Contains(err.Error(), "No such image") {
		// Already removed
		return removedCount, nil
	}
	
	return removedCount, err
}

// formatAge formats duration into human readable string
func formatAge(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	
	if days > 365 {
		years := days / 365
		remainingDays := days % 365
		if remainingDays > 0 {
			return fmt.Sprintf("%dy %dd", years, remainingDays)
		}
		return fmt.Sprintf("%dy", years)
	} else if days > 30 {
		months := days / 30
		remainingDays := days % 30
		if remainingDays > 0 {
			return fmt.Sprintf("%dmo %dd", months, remainingDays)
		}
		return fmt.Sprintf("%dmo", months)
	} else if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	} else {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

// detectLoggingTool detects which logging tool is available
func detectLoggingTool(forceScript bool) (string, error) {
	if forceScript {
		// Check if script is available
		if _, err := exec.LookPath("script"); err != nil {
			return "", fmt.Errorf("script command not found")
		}
		return "script", nil
	}

	// Try asciinema first
	if _, err := exec.LookPath("asciinema"); err == nil {
		return "asciinema", nil
	}

	// Fall back to script
	if _, err := exec.LookPath("script"); err == nil {
		return "script", nil
	}

	return "", fmt.Errorf("neither asciinema nor script command found. Install asciinema with: pip install asciinema")
}

// StartLogging starts a terminal recording session
func StartLogging(outputFile string, useScript bool) error {
	// Check if already logging
	if loggingPID != 0 {
		return fmt.Errorf("a recording session is already active (PID: %d)", loggingPID)
	}

	// Detect which tool to use
	tool, err := detectLoggingTool(useScript)
	if err != nil {
		return err
	}

	// Generate output filename if not provided
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
		// asciinema rec output.cast
		cmd = exec.Command("asciinema", "rec", outputFile)

	case "script":
		// script -q output.log
		if runtime.GOOS == "darwin" {
			// macOS script syntax
			cmd = exec.Command("script", "-q", outputFile)
		} else {
			// Linux script syntax
			cmd = exec.Command("script", "-q", "-f", outputFile)
		}
	}

	// Connect stdio
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the recording
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start recording: %v", err)
	}

	// Save state
	loggingPID = cmd.Process.Pid
	loggingFile = outputFile
	loggingTool = tool

	// Write state to file for persistence
	stateFile := filepath.Join(os.TempDir(), "rfswift-logging.state")
	state := fmt.Sprintf("%d\n%s\n%s", loggingPID, loggingFile, loggingTool)
	if err := ioutil.WriteFile(stateFile, []byte(state), 0644); err != nil {
		common.PrintWarningMessage(fmt.Sprintf("Failed to save state: %v", err))
	}

	common.PrintSuccessMessage("Recording started!")
	common.PrintInfoMessage("To stop recording:")
	if tool == "asciinema" {
		common.PrintInfoMessage("  - Press Ctrl+D or type 'exit'")
		common.PrintInfoMessage("  - Or run: rfswift log stop")
	} else {
		common.PrintInfoMessage("  - Type 'exit' or press Ctrl+D")
		common.PrintInfoMessage("  - Or run: rfswift log stop")
	}

	// Wait for the recording to finish
	if err := cmd.Wait(); err != nil {
		// Clean up state
		loggingPID = 0
		loggingFile = ""
		loggingTool = ""
		os.Remove(stateFile)
		return fmt.Errorf("recording ended: %v", err)
	}

	common.PrintSuccessMessage(fmt.Sprintf("Recording saved to: %s", outputFile))

	// Clean up state
	loggingPID = 0
	loggingFile = ""
	loggingTool = ""
	os.Remove(stateFile)

	return nil
}

// StopLogging stops the current recording session
func StopLogging() error {
	// Try to load state from file
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

	// Send SIGTERM to the process
	process, err := os.FindProcess(loggingPID)
	if err != nil {
		return fmt.Errorf("failed to find process: %v", err)
	}

	if err := process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("failed to stop recording: %v", err)
	}

	common.PrintSuccessMessage(fmt.Sprintf("Recording stopped: %s", loggingFile))

	// Clean up
	loggingPID = 0
	loggingFile = ""
	loggingTool = ""
	os.Remove(stateFile)

	return nil
}

// ReplayLog replays a recorded session
func ReplayLog(inputFile string, speed float64) error {
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", inputFile)
	}

	// Detect file type
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
		// Check if asciinema is available
		if _, err := exec.LookPath("asciinema"); err != nil {
			return fmt.Errorf("asciinema not found. Install with: pip install asciinema")
		}

		speedStr := fmt.Sprintf("%.1f", speed)
		cmd = exec.Command("asciinema", "play", "-s", speedStr, inputFile)

	case "script":
		// For script logs, just cat them (they're plain text)
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

// ListLogs lists all recorded session files
func ListLogs(logDir string) error {
	if logDir == "" {
		logDir = "."
	}

	common.PrintInfoMessage(fmt.Sprintf("Searching for session logs in: %s", logDir))

	// Find all .cast and .log files
	var logs []string

	err := filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			if strings.HasSuffix(path, ".cast") || (strings.HasSuffix(path, ".log") && strings.Contains(path, "rfswift-session")) {
				logs = append(logs, path)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to search directory: %v", err)
	}

	if len(logs) == 0 {
		common.PrintInfoMessage("No session logs found")
		return nil
	}

	// Display logs
	cyan := "\033[36m"
	reset := "\033[0m"
	fmt.Printf("%s📹 Session Logs: %d%s\n", cyan, len(logs), reset)

	for _, log := range logs {
		info, _ := os.Stat(log)
		size := float64(info.Size()) / 1024
		modTime := info.ModTime().Format("2006-01-02 15:04:05")

		var tool string
		if strings.HasSuffix(log, ".cast") {
			tool = "asciinema"
		} else {
			tool = "script"
		}

		fmt.Printf("  • %s\n", log)
		fmt.Printf("    Tool: %s | Size: %.2f KB | Modified: %s\n", tool, size, modTime)
	}

	return nil
}

// DockerRunWithRecording runs a container with session recording
func DockerRunWithRecording(containerName string, recordOutput string, image string, extraArgs map[string]string) error {
	// Detect recording tool
	tool, err := detectLoggingTool(false)
	if err != nil {
		return err
	}

	// Generate output filename if not provided
	if recordOutput == "" {
		timestamp := time.Now().Format("20060102-150405")
		if tool == "asciinema" {
			recordOutput = fmt.Sprintf("rfswift-run-%s-%s.cast", containerName, timestamp)
		} else {
			recordOutput = fmt.Sprintf("rfswift-run-%s-%s.log", containerName, timestamp)
		}
	}
	
	common.PrintInfoMessage(fmt.Sprintf("🔴 Recording session with %s to: %s", tool, recordOutput))

	// Get the current executable path
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	// Build the full command with all necessary flags
	runCmdStr := fmt.Sprintf("%s run -n %s", executable, containerName)
	
	// Add image if specified
	if image != "" {
		runCmdStr += fmt.Sprintf(" -i %s", image)
	}
	
	// Add extra arguments
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

	if err := recordCmd.Run(); err != nil {
		return fmt.Errorf("recording session failed: %v", err)
	}

	fmt.Printf("\033]0;Terminal\007")
	common.PrintSuccessMessage(fmt.Sprintf("🔴 Session recorded to: %s", recordOutput))
	
	return nil
}

// DockerExecWithRecording executes into a container with session recording
func DockerExecWithRecording(containerIdentifier string, workingDir string, recordOutput string, execCommand string) error {
	// Detect recording tool
	tool, err := detectLoggingTool(false)
	if err != nil {
		return err
	}

	// If no container specified, get the latest one
	if containerIdentifier == "" {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		containerIdentifier = latestDockerID(labelKey, labelValue)
		if containerIdentifier == "" {
			return fmt.Errorf("no container specified and no recent rfswift container found")
		}
		common.PrintInfoMessage(fmt.Sprintf("Using latest container: %s", containerIdentifier))
	}

	// Get container name for filename
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

	// Generate output filename if not provided
	if recordOutput == "" {
		timestamp := time.Now().Format("20060102-150405")
		if tool == "asciinema" {
			recordOutput = fmt.Sprintf("rfswift-exec-%s-%s.cast", containerName, timestamp)
		} else {
			recordOutput = fmt.Sprintf("rfswift-exec-%s-%s.log", containerName, timestamp)
		}
	}
	
	common.PrintInfoMessage(fmt.Sprintf("🔴 Recording session with %s to: %s", tool, recordOutput))

	// Get the current executable path
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	// Build command with container ID (now guaranteed to be set)
	execCmdStr := fmt.Sprintf("%s exec -c %s", executable, containerIdentifier)
	
	if workingDir != "" && workingDir != "/root" {
		execCmdStr += fmt.Sprintf(" -w %s", workingDir)
	}
	
	if execCommand != "" && execCommand != "/bin/bash" {
		execCmdStr += fmt.Sprintf(" -e %s", execCommand)
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

	if err := recordCmd.Run(); err != nil {
		return fmt.Errorf("recording session failed: %v", err)
	}

	fmt.Printf("\033]0;Terminal\007")
	common.PrintSuccessMessage(fmt.Sprintf("📹 Session recorded to: %s", recordOutput))
	
	return nil
}

// DockerPullVersion pulls a specific version of an image
func DockerPullVersion(imageref string, version string, imagetag string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	architecture := getArchitecture()
	if architecture == "" {
		common.PrintErrorMessage(fmt.Errorf("unsupported architecture"))
		return
	}

	// If no version specified, use regular pull
	if version == "" {
		DockerPull(imageref, imagetag)
		return
	}

	common.PrintInfoMessage(fmt.Sprintf("Looking for version %s of %s...", version, imageref))

	// Find the version in remote repos
	repo, digest, baseName, err := FindVersionInRemote(imageref, version, architecture)
	if err != nil {
		common.PrintErrorMessage(err)
		common.PrintInfoMessage("Use 'rfswift images versions' to see available versions")
		return
	}

	common.PrintSuccessMessage(fmt.Sprintf("Found version %s in %s", version, repo))
	common.PrintInfoMessage(fmt.Sprintf("Digest: %s", digest[:min(32, len(digest))]))

	// Build the versioned tag name with architecture
	// Format: baseName_version_architecture (e.g., reversing_0.0.7_amd64)
	versionedTag := fmt.Sprintf("%s_%s_%s", baseName, version, architecture)
	pullRef := fmt.Sprintf("%s:%s", repo, versionedTag)

	// Set display tag - use underscore format without 'v' prefix
	// Format: repo:baseName_version (e.g., penthertz/rfswift_noble:reversing_0.0.7)
	if imagetag == "" {
		imagetag = fmt.Sprintf("%s:%s_%s", repo, baseName, version)
	}

	common.PrintInfoMessage(fmt.Sprintf("Pulling %s...", pullRef))

	out, err := cli.ImagePull(ctx, pullRef, image.PullOptions{})
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to pull image: %v", err))
		return
	}
	defer out.Close()

	// Process pull output
	fd, isTerminal := term.GetFdInfo(os.Stdout)
	jsonDecoder := json.NewDecoder(out)
	for {
		var msg jsonmessage.JSONMessage
		if err := jsonDecoder.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		if isTerminal {
			_ = jsonmessage.DisplayJSONMessagesStream(out, os.Stdout, fd, isTerminal, nil)
		} else {
			fmt.Println(msg)
		}
	}

	// Get the pulled image info
	remoteInspect, _, err := cli.ImageInspectWithRaw(ctx, pullRef)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	// Tag with friendly name (without architecture suffix)
	err = cli.ImageTag(ctx, remoteInspect.ID, imagetag)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	common.PrintSuccessMessage(fmt.Sprintf("Image tagged as '%s'", imagetag))

	// Optionally remove the architecture-suffixed tag to keep things clean
	_, err = cli.ImageRemove(ctx, pullRef, image.RemoveOptions{Force: false})
	if err == nil {
		common.PrintInfoMessage(fmt.Sprintf("Removed architecture-suffixed tag: %s", pullRef))
	}

	common.PrintSuccessMessage(fmt.Sprintf("Version %s installed successfully!", version))
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}