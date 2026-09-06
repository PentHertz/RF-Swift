//go:build !windows

package cli

import (
	"os"
	osuser "os/user"
	"reflect"
	"strconv"

	"golang.org/x/sys/unix"
)

// deviceAccessInfo says whether a device path is usable by this process and,
// when it is not, what stands in the way.
type deviceAccessInfo struct {
	Kind        string // device | directory | missing | other
	Accessible  bool
	Group       string // owning group name, or "GID n"
	GroupAccess bool   // the group has read and write bits
	OwnerRoot   bool
}

func deviceAccess(path string) deviceAccessInfo {
	info, err := os.Lstat(path)
	if err != nil {
		return deviceAccessInfo{Kind: "missing"}
	}
	out := deviceAccessInfo{Kind: "other"}
	switch {
	case info.Mode()&os.ModeDevice != 0:
		out.Kind = "device"
	case info.IsDir():
		out.Kind = "directory"
	}
	out.Accessible = unix.Access(path, unix.R_OK|unix.W_OK) == nil
	out.GroupAccess = info.Mode().Perm()&0o060 == 0o060
	value := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if value.IsValid() {
		if uid := value.FieldByName("Uid"); uid.IsValid() && uid.CanUint() {
			out.OwnerRoot = uid.Uint() == 0
		}
		if field := value.FieldByName("Gid"); field.IsValid() && field.CanUint() {
			gid := strconv.FormatUint(field.Uint(), 10)
			out.Group = "GID " + gid
			if resolved, lookupErr := osuser.LookupGroupId(gid); lookupErr == nil {
				out.Group = resolved.Name
			}
		}
	}
	return out
}
