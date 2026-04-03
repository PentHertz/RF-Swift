/* This code is part of RF Swift by @Penthertz
 * Author(s): Sébastien Dudek (@FlUxIuS)
 *
 * Integrated reporting: combine container metadata, session recordings,
 * shell history, and workspace artifacts into structured reports.
 */

package dock

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/client"
	common "penthertz/rfswift/common"
)

// ReportFormat defines the output format for reports.
type ReportFormat string

const (
	ReportFormatMarkdown ReportFormat = "markdown"
	ReportFormatHTML     ReportFormat = "html"
	ReportFormatPDF      ReportFormat = "pdf"
)

// ReportArtifact represents a file found in the workspace.
type ReportArtifact struct {
	Path     string
	Name     string
	Size     string
	Modified string
	Category string // "recording", "capture", "log", "config", "other"
}

// ReportData holds all data collected for a report.
type ReportData struct {
	// Metadata
	Title         string
	ContainerName string
	ContainerID   string
	ImageName     string
	ImageHash     string
	CreatedAt     string
	GeneratedAt   string
	Duration      string
	State         string

	// Environment
	NetworkMode  string
	Privileged   string
	Devices      string
	Capabilities string
	Cgroups      string
	GPUs         string
	Bindings     string
	Ulimits      string
	WorkspacePath string

	// Content
	Recordings []ReportArtifact
	History    []string
	Artifacts  []ReportArtifact
	Notes      string
}

// GenerateReport collects data from a container and its workspace, then writes
// a report in the requested format.
//
//	in(1): string containerName - name or ID of the container
//	in(2): ReportFormat format - output format (markdown, html, pdf)
//	in(3): string outputPath - output file path (auto-generated if empty)
//	in(4): string title - report title (auto-generated if empty)
//	out: (string, error) - path to the generated report, or error
func GenerateReport(containerName string, format ReportFormat, outputPath string, title string) (string, error) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return "", fmt.Errorf("failed to connect to container engine: %w", err)
	}
	defer cli.Close()

	// Resolve container by name
	containerID := resolveContainerIDForReport(ctx, cli, containerName)
	if containerID == "" {
		return "", fmt.Errorf("container '%s' not found", containerName)
	}

	common.PrintInfoMessage(fmt.Sprintf("Collecting data for container '%s'...", containerName))

	// Collect container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	// Get container inspect for creation time and state
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	// Parse creation time
	createdAt := containerJSON.Created
	created, _ := time.Parse(time.RFC3339Nano, createdAt)
	duration := time.Since(created).Truncate(time.Minute).String()

	// Determine workspace path
	workspacePath := resolveWorkspaceFromBindings(containerJSON.HostConfig.Binds)

	// Build report data
	if title == "" {
		title = fmt.Sprintf("RF Swift Assessment Report — %s", containerName)
	}

	data := ReportData{
		Title:         title,
		ContainerName: containerName,
		ContainerID:   containerJSON.ID[:12],
		ImageName:     props["ImageName"],
		ImageHash:     props["ImageHash"],
		CreatedAt:     created.Format("2006-01-02 15:04:05"),
		GeneratedAt:   time.Now().Format("2006-01-02 15:04:05"),
		Duration:      duration,
		State:         containerJSON.State.Status,

		NetworkMode:  props["NetworkMode"],
		Privileged:   props["Privileged"],
		Devices:      props["Devices"],
		Capabilities: props["Caps"],
		Cgroups:      props["Cgroups"],
		GPUs:         props["GPUs"],
		Bindings:     strings.ReplaceAll(props["Bindings"], ";;", "\n"),
		Ulimits:      props["Ulimits"],
		WorkspacePath: workspacePath,
	}

	// Collect session recordings
	common.PrintInfoMessage("Scanning for session recordings...")
	data.Recordings = collectRecordings(workspacePath, containerName)

	// Extract shell history from the container
	common.PrintInfoMessage("Extracting shell history...")
	data.History = extractShellHistory(ctx, containerName)

	// Inventory workspace artifacts
	if workspacePath != "" {
		common.PrintInfoMessage(fmt.Sprintf("Inventorying workspace: %s", workspacePath))
		data.Artifacts = collectArtifacts(workspacePath)
	}

	// Generate the report
	if outputPath == "" {
		ext := ".md"
		switch format {
		case ReportFormatHTML:
			ext = ".html"
		case ReportFormatPDF:
			ext = ".pdf"
		}
		outputPath = fmt.Sprintf("rfswift-report-%s-%s%s",
			containerName,
			time.Now().Format("20060102-150405"),
			ext)
	}

	switch format {
	case ReportFormatMarkdown:
		return outputPath, writeMarkdownReport(data, outputPath)
	case ReportFormatHTML:
		return outputPath, writeHTMLReport(data, outputPath)
	case ReportFormatPDF:
		return outputPath, writePDFReport(data, outputPath)
	default:
		return outputPath, writeMarkdownReport(data, outputPath)
	}
}

// ---------------------------------------------------------------------------
// Data collection
// ---------------------------------------------------------------------------

// resolveWorkspaceFromBindings finds the /workspace mount source from container bindings.
func resolveWorkspaceFromBindings(binds []string) string {
	for _, b := range binds {
		parts := strings.SplitN(b, ":", 3)
		if len(parts) >= 2 && parts[1] == workspaceContainerPath {
			return parts[0]
		}
	}
	return ""
}

// collectRecordings finds .cast and rfswift-*.log files in the workspace and
// common recording locations.
func collectRecordings(workspacePath, containerName string) []ReportArtifact {
	var recordings []ReportArtifact

	// Search workspace
	if workspacePath != "" {
		recordings = append(recordings, findRecordingsInDir(workspacePath)...)
	}

	// Search current directory for container-specific recordings
	cwd, _ := os.Getwd()
	for _, entry := range findRecordingsInDir(cwd) {
		if strings.Contains(entry.Name, containerName) {
			recordings = append(recordings, entry)
		}
	}

	// Deduplicate by absolute path
	seen := map[string]bool{}
	var unique []ReportArtifact
	for _, r := range recordings {
		abs, _ := filepath.Abs(r.Path)
		if !seen[abs] {
			seen[abs] = true
			unique = append(unique, r)
		}
	}

	return unique
}

func findRecordingsInDir(dir string) []ReportArtifact {
	var results []ReportArtifact
	if dir == "" {
		return results
	}

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		isRecording := strings.HasSuffix(path, ".cast") ||
			(strings.HasSuffix(path, ".log") && strings.Contains(info.Name(), "rfswift"))
		if isRecording {
			results = append(results, ReportArtifact{
				Path:     path,
				Name:     info.Name(),
				Size:     formatSize(info.Size()),
				Modified: info.ModTime().Format("2006-01-02 15:04"),
				Category: "recording",
			})
		}
		return nil
	})
	return results
}

// extractShellHistory reads shell history from the container's filesystem.
func extractShellHistory(ctx context.Context, containerName string) []string {
	var history []string

	// Try to read history files from the container using docker/podman cp
	historyFiles := []string{
		"/root/.bash_history",
		"/root/.zsh_history",
		"/home/*/.bash_history",
		"/home/*/.zsh_history",
	}

	for _, histFile := range historyFiles {
		// Use docker exec to cat the history file
		engine := GetEngine()
		var cmd *exec.Cmd
		switch engine.Type() {
		case EnginePodman:
			cmd = exec.Command("podman", "cp", containerName+":"+histFile, "/dev/stdout")
		default:
			cmd = exec.Command("docker", "cp", containerName+":"+histFile, "/dev/stdout")
		}
		output, err := cmd.Output()
		if err != nil {
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Skip zsh history metadata lines (: timestamp:0;)
			if line == "" || strings.HasPrefix(line, ": ") {
				continue
			}
			history = append(history, line)
		}
		if len(history) > 0 {
			break // Got history from one file, done
		}
	}

	return history
}

// collectArtifacts inventories files in the workspace directory.
func collectArtifacts(workspacePath string) []ReportArtifact {
	var artifacts []ReportArtifact

	filepath.Walk(workspacePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Skip recordings (already collected separately)
		if strings.HasSuffix(path, ".cast") ||
			(strings.HasSuffix(path, ".log") && strings.Contains(info.Name(), "rfswift")) {
			return nil
		}

		rel, _ := filepath.Rel(workspacePath, path)
		artifacts = append(artifacts, ReportArtifact{
			Path:     rel,
			Name:     info.Name(),
			Size:     formatSize(info.Size()),
			Modified: info.ModTime().Format("2006-01-02 15:04"),
			Category: categorizeFile(info.Name()),
		})
		return nil
	})

	// Sort by modification time descending
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Modified > artifacts[j].Modified
	})

	return artifacts
}

func categorizeFile(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".iq") || strings.HasSuffix(lower, ".raw") ||
		strings.HasSuffix(lower, ".cf32") || strings.HasSuffix(lower, ".cs8") ||
		strings.HasSuffix(lower, ".cs16") || strings.HasSuffix(lower, ".cu8") ||
		strings.HasSuffix(lower, ".cfile") || strings.HasSuffix(lower, ".sigmf-data"):
		return "capture"
	case strings.HasSuffix(lower, ".sigmf-meta") || strings.HasSuffix(lower, ".json") ||
		strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml") ||
		strings.HasSuffix(lower, ".xml") || strings.HasSuffix(lower, ".conf") ||
		strings.HasSuffix(lower, ".ini") || strings.HasSuffix(lower, ".cfg"):
		return "config"
	case strings.HasSuffix(lower, ".pcap") || strings.HasSuffix(lower, ".pcapng") ||
		strings.HasSuffix(lower, ".cap"):
		return "capture"
	case strings.HasSuffix(lower, ".log") || strings.HasSuffix(lower, ".txt"):
		return "log"
	case strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".svg") || strings.HasSuffix(lower, ".pdf"):
		return "image"
	case strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".sh") ||
		strings.HasSuffix(lower, ".grc"):
		return "script"
	default:
		return "other"
	}
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// resolveContainerIDForReport finds a container by name and returns its full ID.
func resolveContainerIDForReport(ctx context.Context, cli *client.Client, name string) string {
	containerJSON, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		return ""
	}
	return containerJSON.ID
}

// ---------------------------------------------------------------------------
// Report writers
// ---------------------------------------------------------------------------

const markdownTemplate = `# {{.Title}}

**Generated:** {{.GeneratedAt}}

---

## Container Summary

| Property | Value |
|----------|-------|
| **Name** | {{.ContainerName}} |
| **ID** | ` + "`{{.ContainerID}}`" + ` |
| **Image** | {{.ImageName}} |
| **State** | {{.State}} |
| **Created** | {{.CreatedAt}} |
| **Age** | {{.Duration}} |
| **Workspace** | {{if .WorkspacePath}}` + "`{{.WorkspacePath}}`" + `{{else}}not mounted{{end}} |

## Environment Configuration

| Setting | Value |
|---------|-------|
| **Network** | {{.NetworkMode}} |
| **Privileged** | {{.Privileged}} |
| **Devices** | {{if .Devices}}{{.Devices}}{{else}}none{{end}} |
| **Capabilities** | {{if .Capabilities}}{{.Capabilities}}{{else}}default{{end}} |
| **Cgroups** | {{if .Cgroups}}{{.Cgroups}}{{else}}default{{end}} |
| **GPUs** | {{if .GPUs}}{{.GPUs}}{{else}}none{{end}} |
| **Ulimits** | {{if .Ulimits}}{{.Ulimits}}{{else}}default{{end}} |

{{if .Bindings}}
### Volume Bindings

` + "```" + `
{{.Bindings}}
` + "```" + `
{{end}}

## Session Recordings

{{if .Recordings}}
| # | File | Size | Date |
|---|------|------|------|
{{range $i, $r := .Recordings}}| {{inc $i}} | {{$r.Name}} | {{$r.Size}} | {{$r.Modified}} |
{{end}}

> Replay with: ` + "`rfswift log replay -i <file>`" + `
{{else}}
_No session recordings found._
{{end}}

## Shell History

{{if .History}}
` + "```bash" + `
{{range .History}}{{.}}
{{end}}` + "```" + `
{{else}}
_No shell history available. Start the container and use --record to capture sessions._
{{end}}

## Workspace Artifacts

{{if .Artifacts}}
| File | Category | Size | Modified |
|------|----------|------|----------|
{{range .Artifacts}}| {{.Path}} | {{.Category}} | {{.Size}} | {{.Modified}} |
{{end}}
{{else}}
_No files found in workspace.{{if not .WorkspacePath}} Workspace was not mounted for this container.{{end}}_
{{end}}

## Notes

_Add your assessment notes, findings, and observations below._

---

---

*Report generated by [RF Swift](https://rfswift.io) by @Penthertz*
`

func writeMarkdownReport(data ReportData, outputPath string) error {
	funcMap := template.FuncMap{
		"inc": func(i int) int { return i + 1 },
	}
	tmpl, err := template.New("report").Funcs(funcMap).Parse(markdownTemplate)
	if err != nil {
		return fmt.Errorf("template parse error: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

const htmlTemplateStr = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Title}}</title>
<style>
  :root { --primary: #0891b2; --bg: #f8fafc; --card: #ffffff; --text: #1e293b; --muted: #64748b; --border: #e2e8f0; }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--bg); color: var(--text); line-height: 1.6; padding: 2rem; max-width: 1000px; margin: 0 auto; }
  h1 { color: var(--primary); border-bottom: 3px solid var(--primary); padding-bottom: 0.5rem; margin-bottom: 0.5rem; }
  h2 { color: var(--primary); margin-top: 2rem; margin-bottom: 0.75rem; border-bottom: 1px solid var(--border); padding-bottom: 0.25rem; }
  table { width: 100%; border-collapse: collapse; margin: 0.5rem 0 1rem; background: var(--card); border-radius: 4px; overflow: hidden; }
  th, td { padding: 0.5rem 0.75rem; text-align: left; border-bottom: 1px solid var(--border); }
  th { background: var(--primary); color: white; font-weight: 600; font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.5px; }
  tr:nth-child(even) { background: #f1f5f9; }
  code, pre { font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace; background: #1e293b; color: #e2e8f0; border-radius: 4px; }
  code { padding: 0.15rem 0.4rem; font-size: 0.9em; }
  pre { padding: 1rem; overflow-x: auto; margin: 0.5rem 0 1rem; font-size: 0.85rem; line-height: 1.5; }
  .meta { color: var(--muted); font-size: 0.9rem; margin-bottom: 1.5rem; }
  .badge { display: inline-block; padding: 0.15rem 0.5rem; border-radius: 999px; font-size: 0.75rem; font-weight: 600; }
  .badge-running { background: #dcfce7; color: #166534; }
  .badge-stopped { background: #fee2e2; color: #991b1b; }
  .badge-capture { background: #dbeafe; color: #1e40af; }
  .badge-config { background: #fef3c7; color: #92400e; }
  .badge-log { background: #e0e7ff; color: #3730a3; }
  .badge-script { background: #ede9fe; color: #5b21b6; }
  .badge-image { background: #fce7f3; color: #9d174d; }
  .badge-other { background: #f1f5f9; color: #475569; }
  .badge-recording { background: #ccfbf1; color: #065f46; }
  .notes { background: var(--card); border: 2px dashed var(--border); border-radius: 8px; padding: 1.5rem; margin: 1rem 0; min-height: 100px; }
  .footer { text-align: center; color: var(--muted); font-size: 0.8rem; margin-top: 3rem; padding-top: 1rem; border-top: 1px solid var(--border); }
  @media print { body { padding: 0; } .footer { page-break-before: avoid; } }
</style>
</head>
<body>

<h1>{{.Title}}</h1>
<p class="meta">Generated: {{.GeneratedAt}}</p>

<h2>Container Summary</h2>
<table>
<tr><th style="width:30%">Property</th><th>Value</th></tr>
<tr><td>Name</td><td><strong>{{.ContainerName}}</strong></td></tr>
<tr><td>ID</td><td><code>{{.ContainerID}}</code></td></tr>
<tr><td>Image</td><td>{{.ImageName}}</td></tr>
<tr><td>State</td><td><span class="badge {{if eq .State "running"}}badge-running{{else}}badge-stopped{{end}}">{{.State}}</span></td></tr>
<tr><td>Created</td><td>{{.CreatedAt}}</td></tr>
<tr><td>Age</td><td>{{.Duration}}</td></tr>
<tr><td>Workspace</td><td>{{if .WorkspacePath}}<code>{{.WorkspacePath}}</code>{{else}}<em>not mounted</em>{{end}}</td></tr>
</table>

<h2>Environment Configuration</h2>
<table>
<tr><th style="width:30%">Setting</th><th>Value</th></tr>
<tr><td>Network</td><td>{{.NetworkMode}}</td></tr>
<tr><td>Privileged</td><td>{{.Privileged}}</td></tr>
<tr><td>Devices</td><td>{{if .Devices}}{{.Devices}}{{else}}<em>none</em>{{end}}</td></tr>
<tr><td>Capabilities</td><td>{{if .Capabilities}}{{.Capabilities}}{{else}}<em>default</em>{{end}}</td></tr>
<tr><td>Cgroups</td><td>{{if .Cgroups}}{{.Cgroups}}{{else}}<em>default</em>{{end}}</td></tr>
<tr><td>GPUs</td><td>{{if .GPUs}}{{.GPUs}}{{else}}<em>none</em>{{end}}</td></tr>
<tr><td>Ulimits</td><td>{{if .Ulimits}}{{.Ulimits}}{{else}}<em>default</em>{{end}}</td></tr>
</table>

{{if .Bindings}}<h3>Volume Bindings</h3><pre>{{.Bindings}}</pre>{{end}}

<h2>Session Recordings</h2>
{{if .Recordings}}
<table>
<tr><th>#</th><th>File</th><th>Size</th><th>Date</th></tr>
{{range $i, $r := .Recordings}}<tr><td>{{inc $i}}</td><td>{{$r.Name}}</td><td>{{$r.Size}}</td><td>{{$r.Modified}}</td></tr>
{{end}}</table>
<p><em>Replay with: <code>rfswift log replay -i &lt;file&gt;</code></em></p>
{{else}}<p><em>No session recordings found.</em></p>{{end}}

<h2>Shell History</h2>
{{if .History}}<pre>{{range .History}}{{.}}
{{end}}</pre>
{{else}}<p><em>No shell history available.</em></p>{{end}}

<h2>Workspace Artifacts</h2>
{{if .Artifacts}}
<table>
<tr><th>File</th><th>Category</th><th>Size</th><th>Modified</th></tr>
{{range .Artifacts}}<tr><td>{{.Path}}</td><td><span class="badge badge-{{.Category}}">{{.Category}}</span></td><td>{{.Size}}</td><td>{{.Modified}}</td></tr>
{{end}}</table>
{{else}}<p><em>No files found in workspace.</em></p>{{end}}

<h2>Notes</h2>
<div class="notes">
<p><em>Add your assessment notes, findings, and observations here.</em></p>
</div>

<div class="footer">
Report generated by <a href="https://rfswift.io">RF Swift</a> by @Penthertz
</div>

</body>
</html>`

func writeHTMLReport(data ReportData, outputPath string) error {
	funcMap := template.FuncMap{
		"inc": func(i int) int { return i + 1 },
	}
	tmpl, err := template.New("report").Funcs(funcMap).Parse(htmlTemplateStr)
	if err != nil {
		return fmt.Errorf("template parse error: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

func writePDFReport(data ReportData, outputPath string) error {
	// First generate HTML, then convert to PDF
	htmlPath := strings.TrimSuffix(outputPath, ".pdf") + ".tmp.html"

	if err := writeHTMLReport(data, htmlPath); err != nil {
		return err
	}
	defer os.Remove(htmlPath)

	// Try pandoc first, then wkhtmltopdf
	if _, err := exec.LookPath("pandoc"); err == nil {
		cmd := exec.Command("pandoc", htmlPath, "-o", outputPath,
			"--pdf-engine=weasyprint",
			"--metadata", fmt.Sprintf("title=%s", data.Title))
		if out, err := cmd.CombinedOutput(); err != nil {
			// Fallback: try pandoc with default engine
			cmd2 := exec.Command("pandoc", htmlPath, "-o", outputPath)
			if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
				return fmt.Errorf("pandoc failed: %s\n%s", string(out), string(out2))
			}
		} else {
			_ = out
		}
		return nil
	}

	if _, err := exec.LookPath("wkhtmltopdf"); err == nil {
		cmd := exec.Command("wkhtmltopdf", "--quiet", htmlPath, outputPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("wkhtmltopdf failed: %s", string(out))
		}
		return nil
	}

	// No PDF tool available — keep the HTML and inform the user
	finalHTML := strings.TrimSuffix(outputPath, ".pdf") + ".html"
	os.Rename(htmlPath, finalHTML)
	return fmt.Errorf("PDF generation requires pandoc or wkhtmltopdf.\nHTML report saved to: %s\nInstall pandoc: https://pandoc.org/installing.html", finalHTML)
}
