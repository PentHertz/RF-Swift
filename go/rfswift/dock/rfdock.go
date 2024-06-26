/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */
package dock

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"golang.org/x/crypto/ssh/terminal"
)

var inout chan []byte

type DockerInst struct {
	net          string
	privileged   bool
	xdisplay     string
	x11forward   string
	usbforward   string
	usbdevice    string
	shell        string
	imagename    string
	extrabinding string
	entrypoint   string
	extrahosts   string
	extraenv     string
	pulse_server string
	network_mode string
}

var dockerObj = DockerInst{net: "host",
	privileged:   true,
	xdisplay:     "DISPLAY=:0",
	entrypoint:   "/bin/bash",
	x11forward:   "/tmp/.X11-unix:/tmp/.X11-unix",
	usbforward:   "/dev/bus/usb:/dev/bus/usb",
	extrabinding: "/dev/ttyACM0:/dev/ttyACM0", // Some more if needed /run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/dev/snd:/dev/snd,/dev/dri:/dev/dri
	imagename:    "myrfswift:latest",
	extrahosts:   "",
	extraenv:     "",
	network_mode: "host",
	pulse_server: "tcp:localhost:34567",
	shell:        "/bin/bash"} // Instance with default values

func DockerLast(ifilter string) {
	/* Lists 10 last Docker containers
	   in(1):  string optional filter for image name
	*/
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	containers, err := cli.ContainerList(ctx, container.ListOptions{Latest: true, All: true, Limit: 10})
	if err != nil {
		panic(err)
	}

	for _, container := range containers {
		if ifilter != "" {
			if container.Image == ifilter {
				fmt.Println("[", container.Created, "][", container.Image, "] Container: ", container.ID, ", Command: ", container.Command)
			}
		} else {
			fmt.Println("[", container.Created, "][", container.Image, "] Container: ", container.ID, ", Command: ", container.Command)
		}
	}
}

func latestDockerID() string {
	/* Get latest Docker container ID by image name
	   out: string container ID
	*/
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		panic(err)
	}

	var latestContainer types.Container
	for _, container := range containers {
		if latestContainer.ID == "" || container.Created > latestContainer.Created {
			latestContainer = container
		}
	}

	if latestContainer.ID == "" {
		fmt.Println("No container found with the specified image name.")
	}

	return latestContainer.ID
}

func DockerExec(contid string, WorkingDir string) {
	/*
	 *   Start last or specified container ID and execute a program inside
	 *    in(1): string container ID
	 */
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	if contid == "" {
		contid = latestDockerID()
	}

	if err := cli.ContainerStart(ctx, contid, container.StartOptions{}); err != nil {
		panic(err)
	}

	if dockerObj.shell == "/bin/bash" {
		attachAndInteract(ctx, cli, contid)
	} else {
		execCommandInContainer(ctx, cli, contid, WorkingDir)
	}
}

func DockerRun() {
	/*
	 *   Create a container and run it
	 */
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	bindings := combineBindings(dockerObj.x11forward, dockerObj.usbforward, dockerObj.extrabinding)
	extrahosts := splitAndCombine(dockerObj.extrahosts)
	dockerenv := combineEnv(dockerObj.xdisplay, dockerObj.pulse_server, dockerObj.extraenv)

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:        dockerObj.imagename,
		Cmd:          []string{dockerObj.shell},
		Env:          dockerenv,
		OpenStdin:    true,
		StdinOnce:    true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	}, &container.HostConfig{
		NetworkMode: container.NetworkMode(dockerObj.network_mode),
		Binds:       bindings,
		Privileged:  true,
		ExtraHosts:  extrahosts,
	}, &network.NetworkingConfig{}, nil, "")
	if err != nil {
		panic(err)
	}

	waiter, err := cli.ContainerAttach(ctx, resp.ID, container.AttachOptions{
		Stderr: true,
		Stdout: true,
		Stdin:  true,
		Stream: true,
	})
	if err != nil {
		panic(err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		panic(err)
	}

	handleIOStreams(waiter)

	fd := int(os.Stdin.Fd())
	if terminal.IsTerminal(fd) {
		oldState, err := terminal.MakeRaw(fd)
		if err != nil {
			panic(err)
		}
		defer terminal.Restore(fd, oldState)

		go readAndWriteInput(waiter)
	}

	waitForContainer(ctx, cli, resp.ID)

	out, err := cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		panic(err)
	}

	stdcopy.StdCopy(os.Stdout, os.Stderr, out)
}

func execCommandInContainer(ctx context.Context, cli *client.Client, contid, WorkingDir string) {
	execShell := []string{}
	if dockerObj.shell != "" {
		execShell = append(execShell, strings.Split(dockerObj.shell, " ")...)
	}

	optionsCreate := types.ExecConfig{
		WorkingDir:   WorkingDir,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Detach:       false,
		Privileged:   true,
		Tty:          true,
		Cmd:          execShell,
	}

	rst, err := cli.ContainerExecCreate(ctx, contid, optionsCreate)
	if err != nil {
		panic(err)
	}

	optionsStartCheck := types.ExecStartCheck{
		Detach: false,
		Tty:    true,
	}

	response, err := cli.ContainerExecAttach(ctx, rst.ID, optionsStartCheck)
	if err != nil {
		panic(err)
	}
	defer response.Close()

	handleIOStreams(response)
	waitForContainer(ctx, cli, contid)
}

func attachAndInteract(ctx context.Context, cli *client.Client, contid string) {
	response, err := cli.ContainerAttach(ctx, contid, container.AttachOptions{
		Stderr: true,
		Stdout: true,
		Stdin:  true,
		Stream: true,
	})
	if err != nil {
		panic(err)
	}
	defer response.Close()

	handleIOStreams(response)

	fd := int(os.Stdin.Fd())
	if terminal.IsTerminal(fd) {
		oldState, err := terminal.MakeRaw(fd)
		if err != nil {
			panic(err)
		}
		defer terminal.Restore(fd, oldState)

		go readAndWriteInput(response)
	}

	waitForContainer(ctx, cli, contid)
}

func handleIOStreams(response types.HijackedResponse) {
	go io.Copy(os.Stdout, response.Reader)
	go io.Copy(os.Stderr, response.Reader)
	go io.Copy(response.Conn, os.Stdin)
}

func readAndWriteInput(response types.HijackedResponse) {
	reader := bufio.NewReaderSize(os.Stdin, 1)
	for {
		input, err := reader.ReadByte()
		if err != nil {
			return
		}
		response.Conn.Write([]byte{input})
	}
}

func waitForContainer(ctx context.Context, cli *client.Client, contid string) {
	statusCh, errCh := cli.ContainerWait(ctx, contid, container.WaitConditionNextExit)
	select {
	case err := <-errCh:
		if err != nil {
			panic(err)
		}
	case <-statusCh:
	}
}

func combineBindings(x11forward, usbforward, extrabinding string) []string {
	bindings := append(strings.Split(x11forward, ","), strings.Split(usbforward, ",")...)
	if extrabinding != "" {
		bindings = append(bindings, strings.Split(extrabinding, ",")...)
	}
	return bindings
}

func splitAndCombine(commaSeparated string) []string {
	if commaSeparated == "" {
		return []string{}
	}
	return strings.Split(commaSeparated, ",")
}

func combineEnv(xdisplay, pulse_server, extraenv string) []string {
	dockerenv := append(strings.Split(xdisplay, ","), "PULSE_SERVER="+pulse_server)
	if extraenv != "" {
		dockerenv = append(dockerenv, strings.Split(extraenv, ",")...)
	}
	return dockerenv
}

func DockerCommit(contid string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	if err := cli.ContainerStart(ctx, contid, container.StartOptions{}); err != nil {
		panic(err)
	}

	commitResp, err := cli.ContainerCommit(ctx, contid, container.CommitOptions{Reference: dockerObj.imagename})
	if err != nil {
		panic(err)
	}
	fmt.Println(commitResp.ID)
}

func DockerPull(imageref string, imagetag string) {
	/* Pulls an image from a registry
	   in(1): string Image reference
	   in(2): string Image tag target
	*/

	if imagetag == "" { // if tag is empty, keep same tag
		imagetag = imageref
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	out, err := cli.ImagePull(ctx, imageref, image.PullOptions{})
	if err != nil {
		panic(err)
	}

	defer out.Close()

	io.Copy(os.Stdout, out)

	err = cli.ImageTag(ctx, imageref, imagetag)
	if err != nil {
		panic(err)
	}
}

func DockerRename(imageref string, imagetag string) {
	/* Rename an image to another name
	   in(1): string Image reference
	   in(2): string Image tag target
	*/
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	err = cli.ImageTag(ctx, imageref, imagetag)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("[+] Image renamed!")
	}
}

func DockerRemove(contid string) {
	/* Remove a container
	   in(1): string container ID
	*/
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()
	err = cli.ContainerRemove(ctx, contid, container.RemoveOptions{Force: true})
	if err != nil {
		panic(err)
	} else {
		fmt.Println("[+] Container removed!")
	}
}
