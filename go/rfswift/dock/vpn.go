/* This code is part of RF Switch by @Penthertz
 * Author(s): Sébastien Dudek (@FlUxIuS)
 */

package dock

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	common "penthertz/rfswift/common"
)

// VPN type constants
const (
	VPNWireGuard = "wireguard"
	VPNOpenVPN   = "openvpn"
	VPNTailscale = "tailscale"
	VPNNetbird   = "netbird"
)

// parseVPN splits the --vpn flag into type and argument.
//
//	in(1): string vpnFlag  format "type:argument"
//	out: (vpnType, vpnArg string, err error)
func parseVPN(vpnFlag string) (string, string, error) {
	parts := strings.SplitN(vpnFlag, ":", 2)
	vpnType := strings.ToLower(strings.TrimSpace(parts[0]))
	vpnArg := ""
	if len(parts) >= 2 {
		vpnArg = strings.TrimSpace(parts[1])
	}

	switch vpnType {
	case VPNWireGuard, VPNOpenVPN:
		if vpnArg == "" {
			return "", "", fmt.Errorf("%s requires a config file path (e.g., %s:./wg0.conf)", vpnType, vpnType)
		}
	case VPNTailscale, VPNNetbird:
		// auth key is optional — interactive login if omitted
	default:
		return "", "", fmt.Errorf("unsupported VPN type '%s': use wireguard, openvpn, tailscale, or netbird", vpnType)
	}

	return vpnType, vpnArg, nil
}

// applyVPNConfig parses the VPN flag and adjusts containerCfg before container creation:
// adds capabilities, /dev/net/tun device, bind mounts for config files, and env vars.
//
//	out: error
func applyVPNConfig() error {
	vpnType, vpnArg, err := parseVPN(containerCfg.vpn)
	if err != nil {
		return err
	}

	// Add NET_RAW capability (NET_ADMIN is already in defaults)
	if !strings.Contains(containerCfg.caps, "NET_RAW") {
		ContainerAddCaps("NET_RAW")
	}

	// Add /dev/net/tun device
	if !strings.Contains(containerCfg.devices, "/dev/net/tun") {
		ContainerAddDevices("/dev/net/tun:/dev/net/tun")
	}

	switch vpnType {
	case VPNWireGuard:
		absPath, err := filepath.Abs(vpnArg)
		if err != nil {
			return fmt.Errorf("invalid WireGuard config path: %v", err)
		}
		ContainerAddBinding(absPath + ":/etc/wireguard/wg0.conf:ro")
		common.PrintInfoMessage(fmt.Sprintf("VPN: WireGuard config mounted from %s", vpnArg))

	case VPNOpenVPN:
		absPath, err := filepath.Abs(vpnArg)
		if err != nil {
			return fmt.Errorf("invalid OpenVPN config path: %v", err)
		}
		ContainerAddBinding(absPath + ":/etc/openvpn/client.ovpn:ro")
		common.PrintInfoMessage(fmt.Sprintf("VPN: OpenVPN config mounted from %s", vpnArg))

	case VPNTailscale:
		if vpnArg != "" {
			authKey := vpnArg
			authKey = strings.TrimPrefix(authKey, "--auth-key=")
			authKey = strings.TrimPrefix(authKey, "--auth-key ")
			if containerCfg.extraenv != "" {
				containerCfg.extraenv += ","
			}
			containerCfg.extraenv += "TS_AUTHKEY=" + authKey
			common.PrintInfoMessage("VPN: Tailscale configured (auth key)")
		} else {
			common.PrintInfoMessage("VPN: Tailscale configured (interactive login)")
		}

	case VPNNetbird:
		if vpnArg != "" {
			setupKey := vpnArg
			setupKey = strings.TrimPrefix(setupKey, "--setup-key=")
			setupKey = strings.TrimPrefix(setupKey, "--setup-key ")
			if containerCfg.extraenv != "" {
				containerCfg.extraenv += ","
			}
			containerCfg.extraenv += "NB_SETUP_KEY=" + setupKey
			common.PrintInfoMessage("VPN: Netbird configured (setup key)")
		} else {
			common.PrintInfoMessage("VPN: Netbird configured (interactive login)")
		}
	}

	return nil
}

// startVPNInContainer launches the VPN client inside an already-running container
// via detached exec. The VPN tools must be pre-installed in the container image.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string containerID
//	out: error
func startVPNInContainer(ctx context.Context, cli *client.Client, containerID string) error {
	vpnType, vpnArg, err := parseVPN(containerCfg.vpn)
	if err != nil {
		return err
	}

	// Detect if container is privileged (enables kernel TUN mode for Tailscale)
	privileged := isContainerPrivileged(ctx, cli, containerID)

	// Pre-flight: check /dev/net/tun is available (needed by all VPN types)
	if err := checkDeviceAvailable(ctx, cli, containerID, "/dev/net/tun"); err != nil {
		common.PrintWarningMessage("Container lacks /dev/net/tun — VPN may not work. Recreate with: rfswift run --vpn ...")
	}

	// Ensure /dev/net/tun node exists inside container (some images don't have it pre-created)
	_ = execVPNCmd(ctx, cli, containerID,
		[]string{"sh", "-c", "mkdir -p /dev/net && [ -e /dev/net/tun ] || mknod /dev/net/tun c 10 200"},
		nil,
		"TUN device setup",
	)

	// WireGuard and OpenVPN require privileged mode (no userspace fallback)
	if !privileged && (vpnType == VPNWireGuard || vpnType == VPNOpenVPN) {
		common.PrintWarningMessage(fmt.Sprintf("%s requires privileged mode for kernel TUN/iptables access. Use: rfswift run --privileged 1 --vpn %s", vpnType, containerCfg.vpn))
	}

	switch vpnType {
	case VPNWireGuard:
		return execVPNCmd(ctx, cli, containerID,
			[]string{"wg-quick", "up", "/etc/wireguard/wg0.conf"},
			nil,
			"WireGuard",
		)

	case VPNOpenVPN:
		return execVPNCmd(ctx, cli, containerID,
			[]string{"openvpn", "--config", "/etc/openvpn/client.ovpn", "--daemon"},
			nil,
			"OpenVPN",
		)

	case VPNTailscale:
		// Ensure state/socket directories exist
		_ = execVPNCmd(ctx, cli, containerID,
			[]string{"mkdir", "-p", "/var/run/tailscale", "/var/lib/tailscale"},
			nil,
			"Tailscale dirs",
		)

		// Step 1: start daemon
		// Privileged containers can use kernel TUN (full networking: ping, incoming connections)
		// Non-privileged use userspace networking (TCP/UDP only via SOCKS5/HTTP proxy)
		var tailscaledCmd string
		if privileged {
			tailscaledCmd = "tailscaled --state=/var/lib/tailscale/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock > /tmp/tailscaled.log 2>&1 &"
			common.PrintInfoMessage("Privileged mode: using kernel TUN (full networking)")
		} else {
			tailscaledCmd = "tailscaled --tun=userspace-networking --socks5-server=localhost:1055 --outbound-http-proxy-listen=localhost:1080 --state=/var/lib/tailscale/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock > /tmp/tailscaled.log 2>&1 &"
			common.PrintInfoMessage("Non-privileged mode: using userspace networking (SOCKS5: localhost:1055, HTTP: localhost:1080)")
		}
		if err := execVPNCmd(ctx, cli, containerID,
			[]string{"sh", "-c", tailscaledCmd},
			nil,
			"Tailscale daemon",
		); err != nil {
			return err
		}

		// Wait for tailscaled socket to be ready (up to 15 seconds)
		if err := waitForDaemon(ctx, cli, containerID,
			[]string{"test", "-S", "/var/run/tailscale/tailscaled.sock"},
			15, "tailscaled",
		); err != nil {
			return err
		}

		// Step 2: bring up
		if vpnArg != "" {
			// Headless with auth key
			authKey := vpnArg
			authKey = strings.TrimPrefix(authKey, "--auth-key=")
			authKey = strings.TrimPrefix(authKey, "--auth-key ")
			return execVPNCmd(ctx, cli, containerID,
				[]string{"tailscale", "up", "--authkey=" + authKey, "--accept-routes"},
				nil,
				"Tailscale",
			)
		}
		// Interactive: run non-detached so user sees the login URL
		common.PrintInfoMessage("Tailscale will print a login URL — open it in your browser")
		return execVPNInteractive(ctx, cli, containerID,
			[]string{"tailscale", "up", "--accept-routes"},
			"Tailscale",
		)

	case VPNNetbird:
		// Ensure state directory exists
		_ = execVPNCmd(ctx, cli, containerID,
			[]string{"mkdir", "-p", "/var/lib/netbird", "/var/run/netbird"},
			nil,
			"Netbird dirs",
		)

		// Non-privileged: use netstack (userspace) mode with SOCKS5 proxy
		var netbirdEnv []string
		if !privileged {
			netbirdEnv = []string{"NB_USE_NETSTACK_MODE=true", "NB_SOCKS5_LISTENER_PORT=1080"}
			common.PrintInfoMessage("Non-privileged mode: using netstack userspace networking (SOCKS5: localhost:1080)")
		} else {
			common.PrintInfoMessage("Privileged mode: using kernel WireGuard (full networking)")
		}

		if vpnArg != "" {
			// Headless with setup key
			setupKey := vpnArg
			setupKey = strings.TrimPrefix(setupKey, "--setup-key=")
			setupKey = strings.TrimPrefix(setupKey, "--setup-key ")
			return execVPNCmd(ctx, cli, containerID,
				[]string{"netbird", "up", "--setup-key", setupKey},
				netbirdEnv,
				"Netbird",
			)
		}
		// Interactive: run non-detached so user sees the login URL
		common.PrintInfoMessage("Netbird will print a login URL — open it in your browser")
		return execVPNInteractive(ctx, cli, containerID,
			[]string{"netbird", "up"},
			"Netbird",
			netbirdEnv,
		)
	}

	return nil
}

// checkDeviceAvailable checks if a device exists inside the container.
func checkDeviceAvailable(ctx context.Context, cli *client.Client, containerID string, device string) error {
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"test", "-e", device},
	}
	execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return err
	}
	resp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Reader)
	resp.Close()
	inspect, err := cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return err
	}
	if inspect.ExitCode != 0 {
		return fmt.Errorf("device %s not found", device)
	}
	return nil
}

// isContainerPrivileged checks if a container is running in privileged mode.
func isContainerPrivileged(ctx context.Context, cli *client.Client, containerID string) bool {
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return false
	}
	return containerJSON.HostConfig != nil && containerJSON.HostConfig.Privileged
}

// execVPNCmd runs a detached exec command inside the container for VPN setup.
func execVPNCmd(ctx context.Context, cli *client.Client, containerID string, cmd []string, env []string, vpnName string) error {
	execConfig := container.ExecOptions{
		Detach: true,
		Cmd:    cmd,
		Env:    env,
	}

	execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to start %s: %v (is %s installed in the container image?)", vpnName, err, strings.ToLower(vpnName))
	}

	if err := cli.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{Detach: true}); err != nil {
		return fmt.Errorf("failed to start %s: %v", vpnName, err)
	}

	common.PrintSuccessMessage(fmt.Sprintf("%s started inside container", vpnName))
	return nil
}

// waitForDaemon polls a check command inside the container until it succeeds or times out.
func waitForDaemon(ctx context.Context, cli *client.Client, containerID string, checkCmd []string, maxRetries int, daemonName string) error {
	for i := 0; i < maxRetries; i++ {
		time.Sleep(1 * time.Second)
		execConfig := container.ExecOptions{
			AttachStdout: true,
			AttachStderr: true,
			Cmd:          checkCmd,
		}
		execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
		if err != nil {
			continue
		}
		// Attach and wait for the command to finish
		resp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
		if err != nil {
			continue
		}
		// Drain output to let the command complete
		_, _ = io.Copy(io.Discard, resp.Reader)
		resp.Close()

		// Now the exec has finished, check exit code
		inspect, err := cli.ContainerExecInspect(ctx, execID.ID)
		if err != nil {
			continue
		}
		if inspect.ExitCode == 0 {
			return nil
		}
	}
	return fmt.Errorf("%s did not become ready after %d seconds", daemonName, maxRetries)
}

// execVPNInteractive runs a VPN command attached to the terminal so the user can see
// login URLs and interactive output (used for Tailscale/Netbird without auth keys).
func execVPNInteractive(ctx context.Context, cli *client.Client, containerID string, cmd []string, vpnName string, env ...[]string) error {
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}
	if len(env) > 0 && env[0] != nil {
		execConfig.Env = env[0]
	}

	execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to start %s: %v (is %s installed in the container image?)", vpnName, err, strings.ToLower(vpnName))
	}

	resp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("failed to attach to %s: %v", vpnName, err)
	}
	defer resp.Close()

	// Stream output to terminal so user sees the login URL
	_, _ = io.Copy(os.Stdout, resp.Reader)

	// Check the exit code
	inspect, err := cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return fmt.Errorf("failed to check %s status: %v", vpnName, err)
	}
	if inspect.ExitCode != 0 {
		return fmt.Errorf("%s exited with code %d", vpnName, inspect.ExitCode)
	}

	common.PrintSuccessMessage(fmt.Sprintf("%s connected", vpnName))
	return nil
}

// printVPNInfo prints VPN status after container start.
func printVPNInfo() {
	if containerCfg.vpn == "" {
		return
	}

	vpnType, _, err := parseVPN(containerCfg.vpn)
	if err != nil {
		return
	}

	var info string
	switch vpnType {
	case VPNWireGuard:
		info = "WireGuard tunnel active (check with: wg show)"
	case VPNOpenVPN:
		info = "OpenVPN tunnel active (check with: ip a show tun0)"
	case VPNTailscale:
		info = "Tailscale mesh active (check with: tailscale status)"
	case VPNNetbird:
		info = "Netbird mesh active (check with: netbird status)"
	}

	common.PrintInfoMessage(fmt.Sprintf("VPN: %s", info))
}
