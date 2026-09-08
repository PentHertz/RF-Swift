/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Host setup - the Nix engine's --isolate jail (bubblewrap) on Linux.
*
*  bubblewrap needs to create a user namespace as the user. Ubuntu 24.04+
*  restricts unprivileged user namespaces with AppArmor and lets only a
*  *profiled* bubblewrap create one; the profile (bwrap-userns-restrict) is
*  attached to /usr/bin/bwrap, so a bubblewrap built from nixpkgs, or a Nix
*  profile's ahead on PATH, is blocked with "setting up uid map: Permission
*  denied". Stock Debian does not restrict them; a hardened Debian kernel can
*  have kernel.unprivileged_userns_clone=0.
*
*  This file diagnoses that in one place for the jail itself, the CLI, the
*  doctor and the Workbench, and builds the targeted fix - the distribution's
*  bubblewrap package plus its AppArmor profile - so the user gets one command
*  (`rfswift host isolate`) or one click instead of three sudo lines. Lifting
*  the restriction for every program (a sysctl) stays an explicit last resort.
 */

package hostsetup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Isolation causes (IsolationStatus.Cause).
const (
	IsolationReady           = "ready"
	IsolationUnsupported     = "unsupported"         // not Linux
	IsolationNoBwrap         = "no-bwrap"            // no bubblewrap on the host
	IsolationAppArmorProfile = "apparmor-profile"    // /usr/bin/bwrap present, its AppArmor profile not active
	IsolationAppArmorBinary  = "apparmor-unprofiled" // the bwrap in use is not /usr/bin/bwrap, the only one the profile covers
	IsolationUsernsDisabled  = "userns-disabled"     // kernel.unprivileged_userns_clone=0
	IsolationSetuidFailed    = "setuid-failed"       // a setuid bwrap still fails: local policy
	IsolationUnknown         = "unknown"
)

// BwrapProfile is the name of Ubuntu's AppArmor profile for bubblewrap.
const BwrapProfile = "bwrap-userns-restrict"

// Host paths, variables so tests can point them at a temporary tree.
var (
	// DistroBwrap is where the distribution package installs bubblewrap: the
	// path Ubuntu's AppArmor profile is attached to, so RF Swift prefers it
	// over any other bwrap on PATH.
	DistroBwrap = "/usr/bin/bwrap"
	// NixOSBwrapWrapper is NixOS's setuid wrapper, which needs no unprivileged
	// user namespaces at all.
	NixOSBwrapWrapper = "/run/wrappers/bin/bwrap"
	// ProcSys is the sysctl tree.
	ProcSys = "/proc/sys"
	// AppArmorDir holds the active profiles; AppArmorExtraDir the optional
	// ones the apparmor-profiles package ships (Ubuntu 24.04, Debian).
	AppArmorDir      = "/etc/apparmor.d"
	AppArmorExtraDir = "/usr/share/apparmor/extra-profiles"
	// SysctlDropIn is where a sysctl RF Swift sets is persisted.
	SysctlDropIn = "/etc/sysctl.d/99-rfswift-userns.conf"
)

const (
	apparmorUsernsKnob = "kernel/apparmor_restrict_unprivileged_userns" // Ubuntu 24.04+
	usernsCloneKnob    = "kernel/unprivileged_userns_clone"             // Debian kernels
)

// IsolationStatus is the state of this host for the Nix engine's --isolate
// jail. Never changes anything.
type IsolationStatus struct {
	Supported          bool     `json:"supported"`          // Linux
	Ready              bool     `json:"ready"`              // the jail can be created by this user now
	Bwrap              string   `json:"bwrap"`              // the bubblewrap RF Swift will use; "" = none installed (built from nixpkgs at first use)
	Setuid             bool     `json:"setuid"`             // that bwrap is setuid-root (needs no unprivileged namespaces)
	DistroBwrap        bool     `json:"distroBwrap"`        // DistroBwrap exists
	AppArmorRestricted bool     `json:"apparmorRestricted"` // kernel.apparmor_restrict_unprivileged_userns=1
	UsernsClone        string   `json:"usernsClone"`        // kernel.unprivileged_userns_clone: "0", "1", or "" when the kernel has no such knob
	ProfileInstalled   bool     `json:"profileInstalled"`   // AppArmorDir/BwrapProfile exists
	ProfileAvailable   bool     `json:"profileAvailable"`   // a packaged copy sits in AppArmorExtraDir
	Cause              string   `json:"cause"`
	Error              string   `json:"error,omitempty"` // bubblewrap's own message
	Detail             string   `json:"detail"`
	CanFix             bool     `json:"canFix"`   // PlanIsolationFix has a targeted fix (no sysctl)
	FixSteps           []string `json:"fixSteps"` // what that fix does
}

// FindBwrap returns the bubblewrap RF Swift will use and whether it is
// setuid-root: NixOS's setuid wrapper, then the distribution's binary (the
// one an AppArmor profile covers where user namespaces are restricted), then
// any other bwrap on PATH. A setuid one wins outright. "" when none exists.
func FindBwrap() (string, bool) {
	candidates := []string{NixOSBwrapWrapper, DistroBwrap}
	if p, err := exec.LookPath("bwrap"); err == nil {
		candidates = append(candidates, p)
	}
	first := ""
	for _, c := range candidates {
		fi, err := os.Stat(c)
		if err != nil || fi.IsDir() {
			continue
		}
		if fi.Mode()&os.ModeSetuid != 0 {
			return c, true
		}
		if first == "" {
			first = c
		}
	}
	return first, false
}

// BwrapSandboxes runs a trivial sandbox with the given bubblewrap. The error
// carries bubblewrap's own message.
func BwrapSandboxes(bwrap string) error {
	out, err := exec.Command(bwrap, "--ro-bind", "/", "/", "--proc", "/proc", "--", "true").CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return errors.New(msg)
}

// readKnob returns a sysctl's value, "" when the kernel has no such knob.
func readKnob(name string) string {
	b, err := os.ReadFile(filepath.Join(ProcSys, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// GetIsolationStatus probes the host: which bubblewrap would be used, whether
// it can sandbox as this user and, when it cannot, why.
func GetIsolationStatus() IsolationStatus {
	if runtime.GOOS != "linux" {
		return IsolationStatus{Cause: IsolationUnsupported, Detail: "bubblewrap jails are a Linux mechanism (macOS uses sandbox-exec, Windows the WSL 2 distribution)"}
	}
	bwrap, setuid := FindBwrap()
	var probe error
	if bwrap != "" {
		probe = BwrapSandboxes(bwrap)
	}
	return DiagnoseIsolation(bwrap, setuid, probe)
}

// DiagnoseIsolation explains a probe of bwrap (probe nil = the sandbox works)
// from the host's sysctls and AppArmor files, and says whether
// PlanIsolationFix can repair it. Separate from GetIsolationStatus so the
// jail can explain the failure it just hit without a second probe.
func DiagnoseIsolation(bwrap string, setuid bool, probe error) IsolationStatus {
	st := IsolationStatus{
		Supported:          true,
		Bwrap:              bwrap,
		Setuid:             setuid,
		DistroBwrap:        fileExists(DistroBwrap),
		AppArmorRestricted: readKnob(apparmorUsernsKnob) == "1",
		UsernsClone:        readKnob(usernsCloneKnob),
		ProfileInstalled:   fileExists(filepath.Join(AppArmorDir, BwrapProfile)),
		ProfileAvailable:   fileExists(filepath.Join(AppArmorExtraDir, BwrapProfile)),
	}
	if probe != nil {
		st.Error = probe.Error()
	}
	switch {
	case bwrap == "":
		st.Cause = IsolationNoBwrap
		if st.AppArmorRestricted {
			st.Detail = "bubblewrap is not installed, and this host restricts unprivileged user namespaces with AppArmor (Ubuntu 24.04+ default), so the copy RF Swift would build from nixpkgs could not sandbox: the distribution package (" + DistroBwrap + ") is needed"
		} else {
			st.Detail = "bubblewrap is not installed; --isolate builds it from nixpkgs at first use, the distribution package makes it ready now"
		}
	case probe == nil:
		st.Cause, st.Ready = IsolationReady, true
		st.Detail = "bubblewrap sandbox works (" + bwrap + ")"
	case setuid:
		st.Cause = IsolationSetuidFailed
		st.Detail = bwrap + " is setuid-root yet cannot sandbox: check this host's AppArmor/seccomp policy"
	case st.AppArmorRestricted && bwrap != DistroBwrap:
		st.Cause = IsolationAppArmorBinary
		st.Detail = "AppArmor restricts unprivileged user namespaces on this host (Ubuntu 24.04+ default) and lets only a profiled bubblewrap create one; the profile is attached to " + DistroBwrap + " and the bwrap in use is " + bwrap
	case st.AppArmorRestricted:
		st.Cause = IsolationAppArmorProfile
		st.Detail = "AppArmor restricts unprivileged user namespaces on this host (Ubuntu 24.04+ default) and the profile that lets " + DistroBwrap + " create one (" + BwrapProfile + ") is not enabled"
	case st.UsernsClone == "0":
		st.Cause = IsolationUsernsDisabled
		st.Detail = "unprivileged user namespaces are disabled on this kernel (kernel.unprivileged_userns_clone=0)"
	default:
		st.Cause = IsolationUnknown
		st.Detail = "bubblewrap cannot create a user namespace and neither sysctl explains it: a hardened kernel, seccomp or a container runtime may be blocking it"
	}
	if plan, err := PlanIsolationFix(st, false); err == nil {
		st.CanFix, st.FixSteps = true, plan.Steps
	}
	return st
}

// IsolationPlan is everything EnableIsolation will do, so the user can see it
// before agreeing.
type IsolationPlan struct {
	Distro   Distro   `json:"distro"`
	Packages []string `json:"packages"` // distribution packages to install
	Steps    []string `json:"steps"`    // human-readable summary of the privileged script
	Script   string   `json:"script"`   // the privileged script itself
	Sysctl   bool     `json:"sysctl"`   // lifts the AppArmor restriction for every program (last resort)
}

// PlanIsolationFix builds the targeted fix for st: the distribution's
// bubblewrap package when the binary in use is not it (or there is none),
// then Ubuntu's bwrap-userns-restrict AppArmor profile where the host
// restricts user namespaces (loaded from /etc/apparmor.d, else copied from
// the packaged extra profile, installing apparmor-profiles first), and
// kernel.unprivileged_userns_clone=1 where that knob is off. With allowSysctl
// the AppArmor restriction itself is lifted for every program instead - the
// last resort, since it weakens the host - persisted under /etc/sysctl.d.
func PlanIsolationFix(st IsolationStatus, allowSysctl bool) (IsolationPlan, error) {
	plan := IsolationPlan{Distro: DetectDistro()}
	if !st.Supported {
		return plan, errors.New("bubblewrap jails are a Linux mechanism")
	}
	if st.Ready {
		return plan, errors.New("nothing to fix: the sandbox works")
	}
	if st.Cause == IsolationSetuidFailed {
		return plan, errors.New("a setuid bubblewrap that cannot sandbox needs a policy change on this host; nothing RF Swift can apply safely")
	}
	pm := plan.Distro.PackageManager
	var b strings.Builder
	b.WriteString("set -e\n")
	if (st.Cause == IsolationNoBwrap || st.Cause == IsolationAppArmorBinary) && !st.DistroBwrap {
		if pm == "" || pm == "nix" {
			return plan, fmt.Errorf("install bubblewrap with your package manager first (no known one for %s)", plan.Distro.Name)
		}
		plan.Packages = append(plan.Packages, "bubblewrap")
		b.WriteString(installCommand(pm, []string{"bubblewrap"}) + "\n")
		plan.Steps = append(plan.Steps, fmt.Sprintf("install bubblewrap with %s (%s, the binary the AppArmor profile covers)", pm, DistroBwrap))
	}
	if st.AppArmorRestricted {
		if allowSysctl {
			b.WriteString(sysctlScript(apparmorUsernsKnob, "0"))
			plan.Steps = append(plan.Steps, "set kernel.apparmor_restrict_unprivileged_userns=0 and persist it in "+SysctlDropIn+" - lifts the restriction for EVERY program")
			plan.Sysctl = true
		} else {
			profile := filepath.Join(AppArmorDir, BwrapProfile)
			extra := filepath.Join(AppArmorExtraDir, BwrapProfile)
			switch {
			case st.ProfileInstalled:
				plan.Steps = append(plan.Steps, "load the AppArmor profile "+profile)
			case st.ProfileAvailable:
				b.WriteString("cp " + ShellQuote(extra) + " " + ShellQuote(profile) + "\n")
				plan.Steps = append(plan.Steps, "enable the packaged AppArmor profile "+BwrapProfile+" (copy into "+AppArmorDir+")")
			case pm == "apt":
				plan.Packages = append(plan.Packages, "apparmor-profiles")
				b.WriteString(installCommand(pm, []string{"apparmor-profiles"}) + "\n")
				b.WriteString("cp " + ShellQuote(extra) + " " + ShellQuote(profile) + "\n")
				plan.Steps = append(plan.Steps, "install apparmor-profiles and enable its "+BwrapProfile+" profile (copy into "+AppArmorDir+")")
			default:
				return plan, fmt.Errorf("no packaged %s AppArmor profile on %s; --sysctl lifts the restriction for every program instead", BwrapProfile, plan.Distro.Name)
			}
			b.WriteString("apparmor_parser -r " + ShellQuote(profile) + "\n")
			plan.Steps = append(plan.Steps, "load it with apparmor_parser (the restriction stays in force for every other program)")
		}
	}
	if st.UsernsClone == "0" {
		b.WriteString(sysctlScript(usernsCloneKnob, "1"))
		plan.Steps = append(plan.Steps, "set kernel.unprivileged_userns_clone=1 (the distribution default) and persist it in "+SysctlDropIn)
	}
	if len(plan.Steps) == 0 {
		return plan, errors.New("no automatic fix for this host: " + st.Detail)
	}
	plan.Script = b.String()
	return plan, nil
}

// sysctlScript sets a knob now and persists it in RF Swift's sysctl.d
// drop-in, rewriting the drop-in without that key first so a re-run does not
// duplicate it.
func sysctlScript(knob, value string) string {
	key := strings.ReplaceAll(knob, "/", ".")
	line := ShellQuote(key + "=" + value)
	conf := ShellQuote(SysctlDropIn)
	return "sysctl -w " + line + " >/dev/null\n" +
		"mkdir -p " + ShellQuote(filepath.Dir(SysctlDropIn)) + "\n" +
		"{ if [ -f " + conf + " ]; then grep -v " + ShellQuote("^"+key+"=") + " " + conf + " || true; fi; echo " + line + "; } > " + conf + ".tmp\n" +
		"mv " + conf + ".tmp " + conf + "\n"
}

// IsolationReport is what EnableIsolation did and the state afterwards.
type IsolationReport struct {
	Plan   IsolationPlan   `json:"plan"`
	Status IsolationStatus `json:"status"`
	Detail string          `json:"detail"`
}

// EnableIsolation runs the plan in one privileged call (sudo on a terminal,
// a polkit prompt from the Workbench) and probes the sandbox again.
func EnableIsolation(plan IsolationPlan) (IsolationReport, error) {
	report := IsolationReport{Plan: plan}
	if plan.Script == "" {
		return report, errors.New("empty plan")
	}
	if err := RunPrivileged(plan.Script); err != nil {
		return report, fmt.Errorf("enabling the bubblewrap sandbox failed: %w", err)
	}
	report.Status = GetIsolationStatus()
	if report.Status.Ready {
		report.Detail = "bubblewrap can sandbox now (" + report.Status.Bwrap + "): 'rfswift run --engine nix --isolate' is ready"
	} else {
		report.Detail = "applied, but the sandbox still fails: " + report.Status.Detail
	}
	return report, nil
}
