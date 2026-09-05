/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package hostsetup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolationHost points every host path the isolation code reads at a
// temporary tree and returns it. knobs are written under ProcSys.
func isolationHost(t *testing.T, knobs map[string]string, osRelease string) string {
	t.Helper()
	dir := t.TempDir()
	origs := []*string{&DistroBwrap, &NixOSBwrapWrapper, &ProcSys, &AppArmorDir, &AppArmorExtraDir, &SysctlDropIn, &osReleasePath}
	saved := make([]string, len(origs))
	for i, p := range origs {
		saved[i] = *p
	}
	t.Cleanup(func() {
		for i, p := range origs {
			*p = saved[i]
		}
	})
	DistroBwrap = filepath.Join(dir, "usr/bin/bwrap")
	NixOSBwrapWrapper = filepath.Join(dir, "run/wrappers/bin/bwrap")
	ProcSys = filepath.Join(dir, "proc/sys")
	AppArmorDir = filepath.Join(dir, "etc/apparmor.d")
	AppArmorExtraDir = filepath.Join(dir, "usr/share/apparmor/extra-profiles")
	SysctlDropIn = filepath.Join(dir, "etc/sysctl.d/99-rfswift-userns.conf")
	osReleasePath = filepath.Join(dir, "os-release")
	for name, value := range knobs {
		p := filepath.Join(ProcSys, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(osReleasePath, []byte(osRelease), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

const ubuntu = "ID=ubuntu\nID_LIKE=debian\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n"

var uidMapDenied = errors.New("bwrap: setting up uid map: Permission denied")

func TestDiagnoseIsolationUbuntuProfileMissing(t *testing.T) {
	// Ubuntu 24.04: restriction on, /usr/bin/bwrap installed, its profile only
	// available as an extra profile. The fix copies and loads it, no sysctl.
	isolationHost(t, map[string]string{apparmorUsernsKnob: "1", usernsCloneKnob: "1"}, ubuntu)
	touch(t, DistroBwrap)
	touch(t, filepath.Join(AppArmorExtraDir, BwrapProfile))

	st := DiagnoseIsolation(DistroBwrap, false, uidMapDenied)
	if st.Cause != IsolationAppArmorProfile || st.Ready {
		t.Fatalf("cause %q ready %v, want %s", st.Cause, st.Ready, IsolationAppArmorProfile)
	}
	if !st.CanFix || len(st.FixSteps) != 2 {
		t.Fatalf("CanFix %v steps %v", st.CanFix, st.FixSteps)
	}
	plan, err := PlanIsolationFix(st, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"set -e", "cp '" + filepath.Join(AppArmorExtraDir, BwrapProfile) + "' '" + filepath.Join(AppArmorDir, BwrapProfile) + "'", "apparmor_parser -r '" + filepath.Join(AppArmorDir, BwrapProfile) + "'"} {
		if !strings.Contains(plan.Script, want) {
			t.Errorf("script lacks %q:\n%s", want, plan.Script)
		}
	}
	if strings.Contains(plan.Script, "sysctl") || plan.Sysctl || len(plan.Packages) != 0 {
		t.Errorf("targeted fix must not touch sysctls or install packages: %+v", plan)
	}
}

func TestDiagnoseIsolationUbuntuNoExtraProfile(t *testing.T) {
	// Profile neither installed nor available: apt installs apparmor-profiles
	// first. On a non-apt host there is no targeted fix, only --sysctl.
	isolationHost(t, map[string]string{apparmorUsernsKnob: "1"}, ubuntu)
	touch(t, DistroBwrap)
	st := DiagnoseIsolation(DistroBwrap, false, uidMapDenied)
	plan, err := PlanIsolationFix(st, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Script, "apt-get install -y apparmor-profiles") || plan.Packages[0] != "apparmor-profiles" {
		t.Errorf("apt host should install apparmor-profiles: %+v", plan)
	}

	isolationHost(t, map[string]string{apparmorUsernsKnob: "1"}, "ID=fedora\nPRETTY_NAME=Fedora\n")
	touch(t, DistroBwrap)
	st = DiagnoseIsolation(DistroBwrap, false, uidMapDenied)
	if st.CanFix {
		t.Errorf("no packaged profile and no apt: CanFix should be false, steps %v", st.FixSteps)
	}
	if _, err := PlanIsolationFix(st, false); err == nil || !strings.Contains(err.Error(), "--sysctl") {
		t.Errorf("expected a --sysctl hint, got %v", err)
	}
	plan, err = PlanIsolationFix(st, true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Sysctl || !strings.Contains(plan.Script, "sysctl -w 'kernel.apparmor_restrict_unprivileged_userns=0'") || !strings.Contains(plan.Script, SysctlDropIn) {
		t.Errorf("--sysctl plan should lift the AppArmor restriction and persist it: %+v", plan)
	}
}

func TestDiagnoseIsolationUnprofiledBwrap(t *testing.T) {
	// The bwrap in use comes from a Nix profile; the distribution package is
	// missing. Fix = install it (then the profile is shipped or enabled).
	isolationHost(t, map[string]string{apparmorUsernsKnob: "1"}, ubuntu)
	nixBwrap := filepath.Join(t.TempDir(), "bwrap")
	touch(t, nixBwrap)
	st := DiagnoseIsolation(nixBwrap, false, uidMapDenied)
	if st.Cause != IsolationAppArmorBinary {
		t.Fatalf("cause %q, want %s", st.Cause, IsolationAppArmorBinary)
	}
	if !strings.Contains(st.Detail, nixBwrap) || !strings.Contains(st.Detail, DistroBwrap) {
		t.Errorf("detail should name both binaries: %s", st.Detail)
	}
	plan, err := PlanIsolationFix(st, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Packages) == 0 || plan.Packages[0] != "bubblewrap" || !strings.Contains(plan.Script, "apt-get install -y bubblewrap") {
		t.Errorf("should install the distribution's bubblewrap: %+v", plan)
	}
	if !strings.Contains(plan.Script, "apparmor_parser -r") {
		t.Errorf("should also enable the profile on a restricted host: %s", plan.Script)
	}
}

func TestDiagnoseIsolationDebianKnobOff(t *testing.T) {
	// Debian kernel with user namespaces switched off: no AppArmor talk, the
	// fix is the distribution-default sysctl.
	isolationHost(t, map[string]string{usernsCloneKnob: "0"}, "ID=debian\nPRETTY_NAME=\"Debian GNU/Linux 13\"\n")
	touch(t, DistroBwrap)
	st := DiagnoseIsolation(DistroBwrap, false, errors.New("bwrap: No permissions to create a new namespace"))
	if st.Cause != IsolationUsernsDisabled {
		t.Fatalf("cause %q, want %s", st.Cause, IsolationUsernsDisabled)
	}
	plan, err := PlanIsolationFix(st, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Script, "sysctl -w 'kernel.unprivileged_userns_clone=1'") || strings.Contains(plan.Script, "apparmor") {
		t.Errorf("unexpected plan: %s", plan.Script)
	}
	// The drop-in is rewritten without the key first, so re-runs do not
	// duplicate it.
	if !strings.Contains(plan.Script, "grep -v '^kernel.unprivileged_userns_clone='") {
		t.Errorf("drop-in rewrite missing: %s", plan.Script)
	}
}

func TestDiagnoseIsolationReadyUnknownAndNoBwrap(t *testing.T) {
	isolationHost(t, map[string]string{usernsCloneKnob: "1"}, ubuntu)
	touch(t, DistroBwrap)
	if st := DiagnoseIsolation(DistroBwrap, false, nil); !st.Ready || st.Cause != IsolationReady || st.CanFix {
		t.Errorf("working probe: %+v", st)
	}
	if _, err := PlanIsolationFix(DiagnoseIsolation(DistroBwrap, false, nil), false); err == nil {
		t.Error("a working sandbox has nothing to plan")
	}
	// Neither knob explains the failure: no automatic fix.
	if st := DiagnoseIsolation(DistroBwrap, false, uidMapDenied); st.Cause != IsolationUnknown || st.CanFix {
		t.Errorf("unknown cause: %+v", st)
	}
	// A setuid bwrap that fails is a policy problem RF Swift does not touch.
	if st := DiagnoseIsolation(DistroBwrap, true, uidMapDenied); st.Cause != IsolationSetuidFailed || st.CanFix {
		t.Errorf("setuid failure: %+v", st)
	}
	// No bubblewrap at all, unrestricted host: the package is the fix and the
	// jail would otherwise build one from nixpkgs.
	if err := os.Remove(DistroBwrap); err != nil {
		t.Fatal(err)
	}
	st := DiagnoseIsolation("", false, nil)
	if st.Cause != IsolationNoBwrap || !st.CanFix {
		t.Errorf("no bwrap: %+v", st)
	}
}

func TestFindBwrapPrefersDistroOverPath(t *testing.T) {
	isolationHost(t, nil, ubuntu)
	touch(t, DistroBwrap)
	onPath := filepath.Join(t.TempDir(), "bwrap")
	touch(t, onPath)
	t.Setenv("PATH", filepath.Dir(onPath))
	if got, setuid := FindBwrap(); got != DistroBwrap || setuid {
		t.Errorf("FindBwrap() = %s (setuid %v), want the distribution's %s", got, setuid, DistroBwrap)
	}
	// Without the package, the PATH one is used; a setuid wrapper wins over both.
	if err := os.Remove(DistroBwrap); err != nil {
		t.Fatal(err)
	}
	if got, _ := FindBwrap(); got != onPath {
		t.Errorf("FindBwrap() = %s, want the PATH bwrap %s", got, onPath)
	}
	touch(t, NixOSBwrapWrapper)
	if err := os.Chmod(NixOSBwrapWrapper, 0o755|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	if got, setuid := FindBwrap(); got != NixOSBwrapWrapper || !setuid {
		t.Errorf("FindBwrap() = %s (setuid %v), want the setuid wrapper", got, setuid)
	}
}
