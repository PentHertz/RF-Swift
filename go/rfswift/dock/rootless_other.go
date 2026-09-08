//go:build !linux

package dock

// hostHardLimit has no meaning outside Linux: rootless Podman there runs
// inside a VM whose limits are not visible from the client, so every ulimit
// is kept and left for the engine to validate.
func hostHardLimit(string) (int64, bool) {
	return 0, false
}
