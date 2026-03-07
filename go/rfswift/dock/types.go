/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package dock

import (
	"log"
	"strings"

	rfutils "penthertz/rfswift/rfutils"
	common "penthertz/rfswift/common"
)

// HostConfigFull mirrors Docker's host config JSON for direct file manipulation.
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

// ContainerConfig holds the runtime configuration for container creation.
type ContainerConfig struct {
	net          string
	privileged   bool
	xdisplay     string
	x11forward   string
	usbforward   string
	usbdevice    string
	shell        string
	imagename    string
	repotag      string
	extrabinding string
	entrypoint   string
	extrahosts   string
	extraenv     string
	pulseServer  string
	networkMode  string
	exposedPorts string
	bindedPorts  string
	devices      string
	caps         string
	seccomp      string
	cgroups      string
	ulimits      string
	realtime     bool
	desktopProto string
	desktopHost  string
	desktopPort  string
	desktopPass  string
}

var containerCfg = ContainerConfig{
	net:          "host",
	privileged:   false,
	xdisplay:     "DISPLAY=:0",
	entrypoint:   "/bin/bash",
	x11forward:   "/tmp/.X11-unix:/tmp/.X11-unix",
	usbforward:   "",
	extrabinding: "/run/dbus/system_bus_socket:/run/dbus/system_bus_socket",
	imagename:    "myrfswift:latest",
	repotag:      "penthertz/rfswift_noble",
	extrahosts:   "",
	extraenv:     "",
	networkMode:  "host",
	exposedPorts: "",
	bindedPorts:  "",
	pulseServer:  "tcp:localhost:34567",
	devices:      "/dev/snd:/dev/snd,/dev/dri:/dev/dri,/dev/input:/dev/input",
	caps:         "SYS_RAWIO,NET_ADMIN,SYS_TTY_CONFIG,SYS_ADMIN",
	seccomp:      "unconfined",
	cgroups:      "c *:* rwm",
	ulimits:      "",
	realtime:     false,
	shell:        "/bin/bash",
	desktopProto: "",
	desktopHost:  "127.0.0.1",
	desktopPort:  "6080",
	desktopPass:  "",
}

// BuildRecipe defines a YAML recipe for building container images.
type BuildRecipe struct {
	Name      string            `yaml:"name"`
	BaseImage string            `yaml:"base_image"`
	Tag       string            `yaml:"tag"`
	Context   string            `yaml:"context"`
	Labels    map[string]string `yaml:"labels"`
	Steps     []BuildStep       `yaml:"steps"`
}

// BuildStep defines a single step in a build recipe.
type BuildStep struct {
	Type      string     `yaml:"type"`
	Commands  []string   `yaml:"commands"`
	Items     []CopyItem `yaml:"items"`
	Path      string     `yaml:"path"`
	Name      string     `yaml:"name"`
	Script    string     `yaml:"script"`
	Functions []string   `yaml:"functions"`
	Paths     []string   `yaml:"paths"`
	AptClean  bool       `yaml:"apt_clean"`
}

// CopyItem defines a source/destination pair for COPY steps.
type CopyItem struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
}

var loggingPID int
var loggingFile string
var loggingTool string

// init loads the persisted configuration from the platform-specific config file
// into the global containerCfg at package startup.
//
//	out: none
func init() {
	updateContainerCfgFromConfig()
}

// updateContainerCfgFromConfig reads the platform-specific configuration file and
// applies every recognised field to the global containerCfg instance, logging a
// warning and leaving defaults intact if the file cannot be read.
//
//	out: none
func updateContainerCfgFromConfig() {
	config, err := rfutils.ReadOrCreateConfig(common.ConfigFileByPlatform())
	if err != nil {
		log.Printf("Error reading config: %v. Using default values.", err)
		return
	}

	containerCfg.imagename = config.General.ImageName
	containerCfg.repotag = config.General.RepoTag
	containerCfg.shell = config.Container.Shell
	containerCfg.networkMode = config.Container.Network
	containerCfg.exposedPorts = config.Container.ExposedPorts
	containerCfg.bindedPorts = config.Container.PortBindings
	containerCfg.xdisplay = config.Container.XDisplay
	containerCfg.extrahosts = config.Container.ExtraHost
	containerCfg.extraenv = config.Container.ExtraEnv
	containerCfg.devices = config.Container.Devices
	containerCfg.pulseServer = config.Audio.PulseServer
	containerCfg.privileged = strings.ToLower(config.Container.Privileged) == "true"
	containerCfg.caps = config.Container.Caps
	containerCfg.seccomp = config.Container.Seccomp
	if config.Container.Cgroups != "" {
		containerCfg.cgroups = config.Container.Cgroups
	}

	var bindings []string
	var x11Bindings []string

	for _, binding := range config.Container.Bindings {
		if strings.Contains(binding, ".X11-unix") {
			x11Bindings = append(x11Bindings, binding)
		} else if strings.Contains(binding, "/dev/bus/usb") {
			containerCfg.usbforward = binding
			bindings = append(bindings, binding)
		} else {
			bindings = append(bindings, binding)
		}
	}

	if len(x11Bindings) > 0 {
		containerCfg.x11forward = strings.Join(x11Bindings, ",")
	}

	containerCfg.extrabinding = strings.Join(bindings, ",")

	if config.Desktop.Proto != "" {
		containerCfg.desktopProto = config.Desktop.Proto
	}
	if config.Desktop.Host != "" {
		containerCfg.desktopHost = config.Desktop.Host
	}
	if config.Desktop.Port != "" {
		containerCfg.desktopPort = config.Desktop.Port
	}
	if config.Desktop.Password != "" {
		containerCfg.desktopPass = config.Desktop.Password
	}
}
