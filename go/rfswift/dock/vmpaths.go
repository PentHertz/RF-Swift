/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  /dev paths that live inside the engine's virtual machine (Lima on macOS).
*
*  The containers run in a Linux VM whose /dev tree (the USB bus once a
*  device is attached with `rfswift macusb attach`, serial ports, /dev/snd)
*  is not visible from the host, so a host stat of such a path says nothing.
*  This file asks the VM instead and gives the callers that classify device
*  paths (the bindings dialog, the creation normaliser) the same view they
*  have on a Linux host.
 */

package dock

import (
	"os/exec"
	"strconv"
	"strings"

	rfutils "penthertz/rfswift/rfutils"
)

// VMPathInfo is what the VM knows about one path.
type VMPathInfo struct {
	Exists bool
	IsDir  bool
	Device bool   // character or block device node
	Kind   string // "c" or "b" for a device node
	Major  int64  // device major number, -1 when unknown
}

// vmPathInfoFn is the seam tests use; production asks the Lima VM.
var vmPathInfoFn = limaPathInfo

// vmPathInfo describes the paths inside the engine VM. ok is false when the
// VM cannot be asked (not running, limactl missing); callers then fall back
// to what the path names say.
func vmPathInfo(paths []string) (map[string]VMPathInfo, bool) {
	if len(paths) == 0 {
		return map[string]VMPathInfo{}, true
	}
	return vmPathInfoFn(paths)
}

// limaPathInfo runs a small shell inside the Lima VM that prints one line
// per existing path: "<c|b|d|f> <major-hex|-> <path>".
func limaPathInfo(paths []string) (map[string]VMPathInfo, bool) {
	lima, ok := GetEngine().(*LimaEngine)
	if !ok {
		return nil, false
	}
	return limaPathInfoIn(lima.getInstance(), paths)
}

func limaPathInfoIn(instance string, paths []string) (map[string]VMPathInfo, bool) {
	script := `for p in "$@"; do
if [ -c "$p" ]; then printf 'c %s %s\n' "$(stat -c %t "$p" 2>/dev/null || echo -)" "$p";
elif [ -b "$p" ]; then printf 'b %s %s\n' "$(stat -c %t "$p" 2>/dev/null || echo -)" "$p";
elif [ -d "$p" ]; then printf 'd - %s\n' "$p";
elif [ -e "$p" ]; then printf 'f - %s\n' "$p"; fi
done; exit 0`
	args := append([]string{"shell", instance, "sh", "-c", script, "sh"}, paths...)
	out, err := exec.Command(rfutils.LimaCtl(), args...).Output()
	if err != nil {
		return nil, false
	}
	return parseVMPathInfo(string(out)), true
}

func parseVMPathInfo(out string) map[string]VMPathInfo {
	found := map[string]VMPathInfo{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(parts) != 3 {
			continue
		}
		info := VMPathInfo{Exists: true, Major: -1}
		switch parts[0] {
		case "c", "b":
			info.Device, info.Kind = true, parts[0]
			if major, err := strconv.ParseInt(parts[1], 16, 64); err == nil {
				info.Major = major
			}
		case "d":
			info.IsDir = true
		}
		found[parts[2]] = info
	}
	return found
}

// Rule is the device cgroup rule that lets a container open this node
// ("c 189:* rwm"), "" when the major is unknown or the path is not a node.
func (i VMPathInfo) Rule() string {
	if !i.Device || i.Major < 0 {
		return ""
	}
	return i.Kind + " " + strconv.FormatInt(i.Major, 10) + ":* rwm"
}
