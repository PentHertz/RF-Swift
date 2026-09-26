/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package common

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Under sudo, ConfigFileByPlatform and the profiles directory resolve to the
// invoking user's home (SUDO_USER), so a `sudo rfswift ...` run, including
// RF Swift's own elevation for configuration edits, writes there as root; the
// permission-denied fallbacks that retry through `sudo mkdir`/`sudo tee` do
// too. Whatever they create stays root-owned, and later runs as the user can
// no longer update it (settings, profiles, the package setup offer, which is
// then offered on every launch). These helpers give such paths back to the
// user and warn when the configuration directory is already in that state.

// ConfigDirByPlatform returns the directory holding config.ini and profiles/.
func ConfigDirByPlatform() string {
	return filepath.Dir(ConfigFileByPlatform())
}

// invokingIDs returns the uid and gid of the user who ran sudo, when this
// process is root under sudo on a Unix system.
func invokingIDs() (int, int, bool) {
	if runtime.GOOS == "windows" || os.Geteuid() != 0 {
		return 0, 0, false
	}
	uid, uerr := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, gerr := strconv.Atoi(os.Getenv("SUDO_GID"))
	if uerr != nil || gerr != nil || uid == 0 {
		return 0, 0, false
	}
	return uid, gid, true
}

// ChownToInvokingUser gives paths written by a sudo-elevated run back to the
// user who ran sudo. It does nothing outside sudo. Best effort.
func ChownToInvokingUser(paths ...string) {
	uid, gid, ok := invokingIDs()
	if !ok {
		return
	}
	for _, p := range paths {
		_ = os.Lchown(p, uid, gid)
	}
}

// MkdirAllForInvokingUser is os.MkdirAll that, under sudo, gives the
// directories it creates to the user who ran sudo. Existing directories keep
// their owner.
func MkdirAllForInvokingUser(dir string, perm os.FileMode) error {
	var created []string
	if _, _, ok := invokingIDs(); ok {
		for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
			if _, err := os.Lstat(d); err == nil || filepath.Dir(d) == d {
				break
			}
			created = append(created, d)
		}
	}
	if err := os.MkdirAll(dir, perm); err != nil {
		return err
	}
	ChownToInvokingUser(created...)
	return nil
}

// GiveConfigDirBackAfterSudo returns the configuration directory to the
// current user after a permission-denied fallback wrote into it through
// `sudo mkdir` or `sudo tee`. It reuses the sudo credentials just entered.
func GiveConfigDirBackAfterSudo() {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return
	}
	cmd := exec.Command("sudo", "chown", "-R", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), ConfigDirByPlatform())
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	if err := cmd.Run(); err != nil {
		PrintWarningMessage(fmt.Sprintf("Could not give %s back to your user: %v", ConfigDirByPlatform(), err))
	}
}

// WarnForeignOwnedConfig warns when the configuration directory, or anything
// in it, belongs to another user (typically root after a sudo run): RF Swift
// cannot update those files without privileges. It prints the command that
// fixes it and returns whether it warned.
func WarnForeignOwnedConfig() bool {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return false
	}
	dir := ConfigDirByPlatform()
	me := os.Getuid()
	var foreign []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are reported by the owner check of their parent
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if uid, ok := fileOwner(info); ok && uid != me {
			foreign = append(foreign, path)
		}
		return nil
	})
	if len(foreign) == 0 {
		return false
	}
	shown := foreign
	if len(shown) > 3 {
		shown = append(shown[:3:3], fmt.Sprintf("and %d more", len(foreign)-3))
	}
	PrintWarningMessage(fmt.Sprintf("RF Swift's configuration in %s is not owned by you, most likely created by an earlier sudo run: %s. "+
		"Settings, profiles and the package setup offer cannot be saved until you run: sudo chown -R \"$(id -u):$(id -g)\" %s",
		dir, strings.Join(shown, ", "), dir))
	return true
}
