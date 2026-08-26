//go:build !windows

package cli

import (
	"fmt"
	"os"
	osuser "os/user"
	"reflect"
	"strconv"

	"golang.org/x/sys/unix"
)

func serialDeviceAccess(path string) (bool, string) {
	if unix.Access(path, unix.R_OK|unix.W_OK) == nil {
		return true, ""
	}
	group := "the device group"
	info, err := os.Stat(path)
	if err != nil {
		return false, group
	}
	value := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !value.IsValid() {
		return false, group
	}
	field := value.FieldByName("Gid")
	if !field.IsValid() || !field.CanUint() {
		return false, group
	}
	gid := strconv.FormatUint(field.Uint(), 10)
	group = fmt.Sprintf("GID %s", gid)
	if resolved, lookupErr := osuser.LookupGroupId(gid); lookupErr == nil {
		group = resolved.Name
	}
	return false, group
}
