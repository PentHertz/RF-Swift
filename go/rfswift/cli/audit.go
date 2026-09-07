/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  `rfswift audit <target>`: one entry point that audits a Nix environment, a
*  container image, or a running container, auto-detecting which. The dedicated
*  paths (`rfswift nix audit`, `rfswift images audit`) still exist.
 */

package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
)

var auditCmd = &cobra.Command{
	Use:   "audit <target>",
	Short: "Audit a Nix environment, a container image, or a container for security",
	Long: `Audit the security posture of a target, auto-detecting its type:
  - a Nix environment name from the catalog  -> tools + supply chain + integrity
  - an image reference (has a tag or a '/')   -> image CVEs (trivy)
  - otherwise a container name                -> attack surface + image CVEs

Use --type to force env|image|container. The dedicated commands still exist:
'rfswift nix audit', 'rfswift images audit'. All emit stdout/json/html/pdf via
--format and gate with --fail-on.

Examples:
  rfswift audit wifi                         # a Nix environment
  rfswift audit penthertz/rfswift:sdr_full   # an image
  rfswift audit mysdr                         # a running container
  rfswift audit mysdr --type container --format json,html`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		target := args[0]
		typ, _ := cmd.Flags().GetString("type")
		format, _ := cmd.Flags().GetString("format")
		failOn, _ := cmd.Flags().GetString("fail-on")
		out, _ := cmd.Flags().GetString("out")
		if format == "all" {
			format = "stdout,json,html,pdf"
		}
		if failOn == "" {
			failOn = "none"
		}

		if typ == "" || typ == "auto" {
			typ = detectAuditTarget(target)
		}

		switch typ {
		case "env", "nix":
			auditNixEnv(target, format, failOn, out)
		case "image":
			if err := rfdock.AuditImage([]string{target}, rfdock.ImageAuditOptions{FailOn: failOn, OutDir: out, Formats: format}); err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
		default: // container
			if err := rfdock.AuditContainer(target, rfdock.ContainerAuditOptions{FailOn: failOn, OutDir: out, Formats: format}); err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
		}
	},
}

// detectAuditTarget guesses whether target is a Nix env, an image, or a
// container: a catalog environment name wins; an image-looking ref (tag or
// registry path) is an image; anything else is treated as a container.
func detectAuditTarget(target string) string {
	if cat, err := rfnix.LoadCatalog(); err == nil && cat.Find(target) != nil {
		return "env"
	}
	if strings.Contains(target, ":") || strings.Contains(target, "/") {
		return "image"
	}
	return "container"
}

// auditNixEnv drives the Nix audit for one environment, storing its report in
// the environment's own state dir so `nix info` can surface the posture.
func auditNixEnv(name, format, failOn, out string) {
	flakeRef, image, reportDir := nixAuditTarget(name)
	var sargs []string
	sargs = append(sargs, "--env", image)
	if format == "" {
		format = "stdout,txt,json"
	}
	sargs = append(sargs, "--format", format)
	if failOn != "" {
		sargs = append(sargs, "--fail-on", failOn)
	}
	if out == "" {
		out = reportDir
	}
	sargs = append(sargs, "--out", out)
	if err := rfnix.RunAudit(flakeRef, sargs); err != nil {
		common.PrintErrorMessage(err)
		os.Exit(1)
	}
}

// nixAuditTarget resolves what the audit scans for a name given on the
// command line. A registered environment (rfswift env run -n NAME -i IMAGE)
// is audited as the flake environment it was built from, on the flake it
// was pinned to, with the report kept in the environment's directory: the
// same thing the Workbench does. The flake knows nothing of the local name,
// so passing it as the environment ("--env superlab") only yields "could not
// realise closure". A name that is not an environment is taken as a flake
// environment (an image name such as "sdr_light") on the default flake.
func nixAuditTarget(name string) (flakeRef, image, reportDir string) {
	if env, err := rfnix.GetEnvironment(name); err == nil && env.Image != "" {
		flakeRef = env.FlakeRef
		if flakeRef == "" {
			flakeRef = rfnix.ResolveFlakeRef("")
		}
		return flakeRef, env.Image, rfnix.EnvReportDir(name)
	}
	return rfnix.ResolveFlakeRef(""), name, rfnix.EnvReportDir(name)
}

func registerAuditCommand() {
	rootCmd.AddCommand(auditCmd)
	auditCmd.Flags().String("type", "auto", "target type: auto|env|image|container")
	auditCmd.Flags().String("format", "stdout", "report formats: stdout,json,html,pdf (or 'all')")
	auditCmd.Flags().String("fail-on", "none", "exit non-zero if a finding is >= none|low|medium|high|critical")
	auditCmd.Flags().String("out", "", "report output directory (default: ./security-report)")
}
