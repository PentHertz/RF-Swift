/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Container (runtime) security audit.
*
*  Where `images audit` scans an image for CVEs, this audits a *container* for
*  its attack surface, so users are aware of how exposed a running/created
*  container is - to the network AND to the host:
*
*    - host exposure: --privileged, host network/PID/IPC namespaces, sensitive
*      host bind mounts (the docker socket, /, /etc, ...), added capabilities,
*      disabled seccomp/apparmor, running as root, device passthrough;
*    - network exposure: published/exposed ports, flagging those bound to all
*      interfaces (reachable from the host and beyond);
*    - known CVEs: the container's image is scanned with trivy (reusing the
*      image-audit engine);
*    - attack-enabling binaries present inside a running container (nc, socat,
*      curl, compilers, ...).
*
*  It works the same on Linux, macOS and Windows: it drives the engine CLI
*  (`docker`/`podman inspect`, `exec`) rather than any host-specific socket.
 */

package dock

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	common "penthertz/rfswift/common"
)

// ContainerAuditOptions controls a container audit.
type ContainerAuditOptions struct {
	FailOn  string
	OutDir  string
	Formats string
}

type ctrFinding struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

// inspectDoc is the subset of `docker/podman inspect` we evaluate.
type inspectDoc struct {
	Name  string `json:"Name"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	Config struct {
		Image        string              `json:"Image"`
		User         string              `json:"User"`
		ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	} `json:"Config"`
	HostConfig struct {
		Privileged   bool                        `json:"Privileged"`
		NetworkMode  string                      `json:"NetworkMode"`
		PidMode      string                      `json:"PidMode"`
		IpcMode      string                      `json:"IpcMode"`
		CapAdd       []string                    `json:"CapAdd"`
		SecurityOpt  []string                    `json:"SecurityOpt"`
		Binds        []string                    `json:"Binds"`
		PortBindings map[string][]portBinding    `json:"PortBindings"`
		Devices      []struct{ PathOnHost string } `json:"Devices"`
	} `json:"HostConfig"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Ports map[string][]portBinding `json:"Ports"`
	} `json:"NetworkSettings"`
}

type portBinding struct {
	HostIp   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// dangerous capabilities and attack-enabling binaries to look for.
var riskyCaps = map[string]string{
	"SYS_ADMIN": "high", "NET_ADMIN": "high", "SYS_PTRACE": "high",
	"SYS_MODULE": "critical", "ALL": "critical", "DAC_READ_SEARCH": "high",
	"SYS_BOOT": "high", "NET_RAW": "medium",
}
var riskyBins = []string{"nc", "ncat", "netcat", "socat", "nmap", "curl", "wget",
	"python3", "python", "perl", "gcc", "cc", "ssh", "tcpdump", "msfconsole"}

// AuditContainer audits a running/created container and writes the report.
func AuditContainer(name string, opts ContainerAuditOptions) error {
	outDir := opts.OutDir
	if outDir == "" {
		outDir = "security-report"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("cannot create report dir %s: %w", outDir, err)
	}
	formats := opts.Formats
	if formats == "" {
		formats = "stdout"
	}
	want := func(f string) bool { return strings.Contains(","+formats+",", ","+f+",") }

	eng := GetEngine()
	if eng == nil || !eng.IsAvailable() {
		return fmt.Errorf("no container engine available (start Docker/Podman)")
	}
	cli := engineCLI(eng.Type())

	raw, err := exec.Command(cli, "inspect", name).Output()
	if err != nil {
		return fmt.Errorf("%s inspect %s failed (does the container exist?): %w", cli, name, err)
	}
	var docs []inspectDoc
	if err := json.Unmarshal(raw, &docs); err != nil || len(docs) == 0 {
		return fmt.Errorf("could not parse inspect output for %s", name)
	}
	c := docs[0]

	fmt.Println()
	common.PrintInfoMessage(fmt.Sprintf("=== container: %s (%s) ===", strings.TrimPrefix(c.Name, "/"), c.Config.Image))

	findings := analyzeContainer(c, cli)

	// CVE scan of the container's image (reuse the image-audit trivy engine).
	imgWorst := 0
	imgRes := imgResult{Ref: c.Config.Image, Worst: "none"}
	hostTrivy, tcli := trivyContext()
	if hostTrivy || tcli != "" {
		jsonPath := filepath.Join(outDir, "trivy-"+sanitizeRef(c.Config.Image)+".json")
		if e := runTrivy(c.Config.Image, jsonPath, hostTrivy, tcli, outDir); e == nil {
			imgRes = summarizeTrivy(jsonPath, c.Config.Image)
			imgWorst = sevRank(imgRes.Worst)
		} else {
			common.PrintWarningMessage("image CVE scan skipped: " + e.Error())
		}
	} else {
		common.PrintWarningMessage("image CVE scan skipped: no trivy and no engine to run it")
	}

	// Print findings, tracking worst.
	worst := imgWorst
	if len(findings) == 0 {
		fmt.Printf("  ✅ config: no risky runtime settings detected\n")
	}
	for _, f := range findings {
		if sevRank(f.Severity) > worst {
			worst = sevRank(f.Severity)
		}
		icon := "⚠️"
		if sevRank(f.Severity) >= 3 {
			icon = "❌"
		} else if sevRank(f.Severity) == 0 {
			icon = "ℹ️"
		}
		fmt.Printf("  %s %-9s [%s] %s\n", icon, f.Severity, f.Category, f.Message)
	}

	generated := time.Now().UTC().Format(time.RFC3339)
	ok := worst == 0
	if want("json") {
		writeContainerJSON(filepath.Join(outDir, "report.json"), generated, strings.TrimPrefix(c.Name, "/"), c.Config.Image, rankName(worst), ok, findings, imgRes)
		common.PrintInfoMessage("Wrote " + filepath.Join(outDir, "report.json"))
	}
	if want("html") || want("pdf") {
		htmlPath := filepath.Join(outDir, "report.html")
		writeContainerHTML(htmlPath, generated, strings.TrimPrefix(c.Name, "/"), c.Config.Image, rankName(worst), ok, findings, imgRes)
		if want("html") {
			common.PrintInfoMessage("Wrote " + htmlPath)
		}
		if want("pdf") {
			if e := renderPDF(htmlPath, filepath.Join(outDir, "report.pdf")); e != nil {
				common.PrintWarningMessage("PDF not generated: " + e.Error() + " (HTML report is available)")
			} else {
				common.PrintInfoMessage("Wrote " + filepath.Join(outDir, "report.pdf"))
			}
			if !want("html") {
				_ = os.Remove(htmlPath)
			}
		}
	}

	fmt.Println()
	common.PrintInfoMessage(fmt.Sprintf("Container audit complete. Worst severity: %s. Reports in %s.", rankName(worst), outDir))

	if opts.FailOn != "none" && opts.FailOn != "" && sevRank(opts.FailOn) > 0 && worst >= sevRank(opts.FailOn) {
		return fmt.Errorf("container audit found findings at or above '%s' (worst: %s)", opts.FailOn, rankName(worst))
	}
	return nil
}

// analyzeContainer evaluates the inspect data into attack-surface findings.
func analyzeContainer(c inspectDoc, cli string) []ctrFinding {
	var f []ctrFinding
	add := func(sev, cat, msg string) { f = append(f, ctrFinding{sev, cat, msg}) }

	// --- Host exposure ---
	if c.HostConfig.Privileged {
		add("critical", "host", "runs --privileged: full access to host devices and kernel (container escape is trivial)")
	}
	if nm := c.HostConfig.NetworkMode; nm == "host" {
		add("high", "host", "host network mode: shares the host's network stack; container ports ARE host ports")
	}
	if c.HostConfig.PidMode == "host" {
		add("high", "host", "host PID namespace: can see and signal host processes")
	}
	if c.HostConfig.IpcMode == "host" {
		add("high", "host", "host IPC namespace: shares host shared-memory/semaphores")
	}
	for _, cap := range c.HostConfig.CapAdd {
		if sev, ok := riskyCaps[strings.ToUpper(strings.TrimPrefix(cap, "CAP_"))]; ok {
			add(sev, "capability", "added capability "+cap+": elevates in-container privilege / host reach")
		}
	}
	for _, so := range c.HostConfig.SecurityOpt {
		l := strings.ToLower(so)
		if strings.Contains(l, "seccomp") && strings.Contains(l, "unconfined") {
			add("high", "hardening", "seccomp disabled (unconfined): all syscalls allowed")
		}
		if strings.Contains(l, "apparmor") && strings.Contains(l, "unconfined") {
			add("high", "hardening", "AppArmor disabled (unconfined)")
		}
	}
	if u := strings.TrimSpace(c.Config.User); u == "" || u == "root" || u == "0" || strings.HasPrefix(u, "0:") {
		add("medium", "user", "runs as root inside the container")
	}

	// --- Sensitive mounts (host filesystem exposure) ---
	type m struct{ src, dst string; rw bool }
	var mounts []m
	for _, b := range c.HostConfig.Binds {
		p := strings.SplitN(b, ":", 3)
		src := p[0]
		dst := src
		rw := true
		if len(p) >= 2 {
			dst = p[1]
		}
		if len(p) == 3 && strings.Contains(p[2], "ro") {
			rw = false
		}
		mounts = append(mounts, m{src, dst, rw})
	}
	for _, mt := range c.Mounts {
		if mt.Type == "bind" {
			mounts = append(mounts, m{mt.Source, mt.Destination, mt.RW})
		}
	}
	seenMount := map[string]bool{}
	for _, mm := range mounts {
		s := mm.src
		if seenMount[s] {
			continue
		}
		seenMount[s] = true
		switch {
		case strings.HasSuffix(s, "docker.sock") || strings.Contains(s, "podman.sock"):
			add("critical", "mount", "mounts the container engine socket ("+s+"): equals root on the host")
		case s == "/" || s == "/etc" || s == "/root" || s == "/boot" || strings.HasPrefix(s, "/proc") || strings.HasPrefix(s, "/sys"):
			add("high", "mount", "mounts sensitive host path "+s+" into the container")
		case mm.rw && strings.HasPrefix(s, "/"):
			add("low", "mount", "writable host bind mount "+s+" -> "+mm.dst)
		}
	}

	// --- Network exposure (ports) ---
	ports := c.HostConfig.PortBindings
	if len(ports) == 0 {
		ports = c.NetworkSettings.Ports
	}
	for port, binds := range ports {
		for _, b := range binds {
			hip := b.HostIp
			if hip == "" || hip == "0.0.0.0" || hip == "::" {
				add("medium", "network", fmt.Sprintf("port %s published on all interfaces (%s:%s): reachable from the host and its network", port, "0.0.0.0", b.HostPort))
			} else if hip == "127.0.0.1" || hip == "::1" {
				add("low", "network", fmt.Sprintf("port %s published to localhost (%s:%s)", port, hip, b.HostPort))
			} else {
				add("medium", "network", fmt.Sprintf("port %s published on %s:%s", port, hip, b.HostPort))
			}
		}
	}
	if len(ports) == 0 && len(c.Config.ExposedPorts) > 0 {
		var ps []string
		for p := range c.Config.ExposedPorts {
			ps = append(ps, p)
		}
		add("low", "network", "image EXPOSEs ports not published to the host: "+strings.Join(ps, ", "))
	}

	// --- Device passthrough (expected for SDR/hardware, but note it) ---
	if len(c.HostConfig.Devices) > 0 {
		var ds []string
		for _, d := range c.HostConfig.Devices {
			ds = append(ds, d.PathOnHost)
		}
		add("low", "device", "host devices passed through (expected for radio/USB work): "+strings.Join(ds, ", "))
	}

	// --- Attack-enabling binaries inside a running container ---
	if c.State.Running {
		if present := probeBinaries(cli, c.Name); len(present) > 0 {
			add("low", "binaries", "attack-enabling tools present in the container: "+strings.Join(present, ", "))
		}
	}
	return f
}

// probeBinaries execs into a running container and reports which risky binaries
// are on PATH. Best-effort; requires a shell in the (Linux) container.
func probeBinaries(cli, name string) []string {
	script := "for b in " + strings.Join(riskyBins, " ") + "; do command -v $b >/dev/null 2>&1 && echo $b; done"
	out, err := exec.Command(cli, "exec", name, "sh", "-c", script).Output()
	if err != nil {
		return nil
	}
	var found []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			found = append(found, l)
		}
	}
	return found
}

func writeContainerJSON(path, generated, name, image, worst string, ok bool, findings []ctrFinding, img imgResult) {
	obj := map[string]interface{}{
		"tool":           "rfswift audit (container)",
		"generated":      generated,
		"container":      name,
		"image":          image,
		"worst_severity": worst,
		"ok":             ok,
		"findings":       findings,
		"image_cve":      img,
	}
	b, _ := json.MarshalIndent(obj, "", "  ")
	_ = os.WriteFile(path, b, 0o644)
}

func writeContainerHTML(path, generated, name, image, worst string, ok bool, findings []ctrFinding, img imgResult) {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>RF Swift container audit</title>
<style>body{font-family:system-ui,Arial,sans-serif;margin:2rem;color:#1b1f24}h1{font-size:1.5rem}
.meta{color:#57606a;font-size:.9rem}.banner{padding:.7rem 1rem;border-radius:8px;font-weight:600;margin:1rem 0}
.green{background:#d3f9d8;color:#0b6b2e}.red{background:#ffe3e3;color:#a4133c}.yellow{background:#fff3bf;color:#8a6d00}
table{border-collapse:collapse;width:100%;font-size:.92rem}th,td{border:1px solid #d0d7de;padding:.35rem .55rem;text-align:left}
th{background:#f6f8fa}.critical{color:#a4133c;font-weight:600}.high{color:#c9184a;font-weight:600}.medium{color:#8a6d00}.low{color:#57606a}code{font-size:.85em}</style></head><body>`)
	b.WriteString("<h1>RF Swift container audit</h1>")
	b.WriteString(fmt.Sprintf(`<p class="meta">container: <code>%s</code> &nbsp;|&nbsp; image: <code>%s</code> &nbsp;|&nbsp; generated: %s</p>`,
		html.EscapeString(name), html.EscapeString(image), html.EscapeString(generated)))
	if ok {
		b.WriteString(`<div class="banner green">&#9989; No risky settings and no known CVEs</div>`)
	} else if sevRank(worst) >= 3 {
		b.WriteString(fmt.Sprintf(`<div class="banner red">&#10060; Attack surface found &mdash; worst severity: %s</div>`, worst))
	} else {
		b.WriteString(fmt.Sprintf(`<div class="banner yellow">&#9888;&#65039; Attack surface found &mdash; worst severity: %s</div>`, worst))
	}
	b.WriteString(fmt.Sprintf(`<p>Image CVEs (trivy): critical=%d high=%d medium=%d low=%d (worst: %s)</p>`,
		img.Critical, img.High, img.Medium, img.Low, img.Worst))
	b.WriteString(`<h2>Runtime findings</h2><table><tr><th>Severity</th><th>Category</th><th>Finding</th></tr>`)
	for _, f := range findings {
		b.WriteString(fmt.Sprintf(`<tr><td class="%s">%s</td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(f.Severity), html.EscapeString(f.Severity), html.EscapeString(f.Category), html.EscapeString(f.Message)))
	}
	if len(findings) == 0 {
		b.WriteString(`<tr><td class="low">-</td><td>config</td><td>no risky runtime settings detected</td></tr>`)
	}
	b.WriteString("</table></body></html>")
	_ = os.WriteFile(path, []byte(b.String()), 0o644)
}
