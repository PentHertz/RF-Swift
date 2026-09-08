//go:build !linux

package dock

func deviceNumbers(string) (uint32, uint32, bool) { return 0, 0, false }

func deviceNumbersSupported() bool { return false }
