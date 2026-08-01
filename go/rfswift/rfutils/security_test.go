/* This code is part of RF Swift by @Penthertz
*  Security regression tests: input validation that guards the QEMU monitor
*  (QMP/HMP) command builders and the Lima template writer against injection.
 */

package rfutils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsValidUSBID(t *testing.T) {
	valid := []string{"0x1d50", "1d50", "0x604b", "604b", "0", "ffff", "0xFFFF", "0xa", "a"}
	// Injection / malformed payloads that must be rejected before reaching the
	// QEMU human-monitor "device_add usb-host,vendorid=...,..." command.
	invalid := []string{
		"",
		"0x1d50,bus=usb-bus.0",     // extra device_add property
		"0x1d50,id=evil",           // override id
		"0x1d50 -device something", // extra HMP token
		"1d50;rm -rf ~",            // command chaining
		"0x1d50\ndevice_add x",     // newline / second command
		"0x12345",                  // > 16 bits
		"12345",                    // > 4 hex digits
		"0xzz",                     // non-hex
		"0x1d 50",                  // embedded space
		"'0x1d50'",                 // quotes
	}
	for _, v := range valid {
		if !IsValidUSBID(v) {
			t.Errorf("IsValidUSBID(%q) = false, want true", v)
		}
	}
	for _, v := range invalid {
		if IsValidUSBID(v) {
			t.Errorf("IsValidUSBID(%q) = true, want false (injection guard)", v)
		}
	}
}

func TestIsSafeQMPDeviceID(t *testing.T) {
	valid := []string{"usb-1d50-604b", "usb-host.0", "dev_1", "A-b.c_9"}
	invalid := []string{
		"",
		"usb;device_del other", // command chaining
		"usb host",             // space
		"usb,id=x",             // comma
		"usb\ndevice_del y",    // newline
		"usb`whoami`",          // backtick
	}
	for _, v := range valid {
		if !isSafeQMPDeviceID(v) {
			t.Errorf("isSafeQMPDeviceID(%q) = false, want true", v)
		}
	}
	for _, v := range invalid {
		if isSafeQMPDeviceID(v) {
			t.Errorf("isSafeQMPDeviceID(%q) = true, want false (injection guard)", v)
		}
	}
	// Over-length device IDs are rejected.
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	if isSafeQMPDeviceID(string(long)) {
		t.Error("isSafeQMPDeviceID accepted a 65-char id, want reject")
	}
}

// TestLimaSizeRejectsInjection ensures memory/disk values that could break out
// of the quoted YAML scalar are rejected by the validator.
func TestLimaSizeRejectsInjection(t *testing.T) {
	for _, v := range []string{
		"8GiB\ndisk: /etc/passwd", // newline -> new YAML key
		`8GiB" evil: true`,        // quote break-out
		"8GiB; rm -rf ~",          // shell metachars
		"8GiB ",                   // trailing space (written verbatim)
		"$(whoami)GiB",            // command substitution chars
	} {
		if IsValidLimaSize(v) {
			t.Errorf("IsValidLimaSize(%q) = true, want false (injection guard)", v)
		}
	}
}

// TestFindLimaQMPSocket_NoShellExecution is a regression test for the removal of
// the old `bash -c "...grep <instance>..."` code path: a shell-injection payload
// in the instance name must never be executed.
func TestFindLimaQMPSocket_NoShellExecution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	marker := filepath.Join(t.TempDir(), "pwned")
	evil := "; touch " + marker + " ;" // classic shell-injection payload

	// We don't care about the return value (no matching VM exists); we care that
	// the payload is treated as data, not executed.
	_, _ = FindLimaQMPSocket(evil)

	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("SECURITY: instance name was executed as a shell command (marker %q created)", marker)
	}
}
