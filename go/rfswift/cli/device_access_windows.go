//go:build windows

package cli

// Windows does not expose Unix tty ownership or supplementary groups. The Nix
// engine is not native there, and Unix serial paths are never enumerated.
func serialDeviceAccess(string) (bool, string) { return true, "" }
