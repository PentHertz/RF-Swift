/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*/
package dock

import (
 "fmt"
 "os"
 "strings"
 "io"
 "bufio"

 "context"
 "github.com/docker/docker/api/types/container"
 "github.com/docker/docker/client"
 "github.com/docker/docker/pkg/stdcopy"
 "github.com/docker/docker/api/types/network"
 "github.com/docker/docker/api/types"
 "github.com/docker/docker/api/types/image"
 "golang.org/x/crypto/ssh/terminal"
)

var inout chan []byte

type DockerInst struct {
    net         string
    privileged  bool
    xdisplay     string
    x11forward  string
    usbforward  string
    usbdevice   string
    shell       string
    imagename   string
    extrabinding string
    entrypoint string
}

var dockerObj = DockerInst{ net: "host", 
                            privileged: true, 
                            xdisplay: "DISPLAY=:0",
                            entrypoint: "/bin/bash",
                            x11forward: "/tmp/.X11-unix:/tmp/.X11-unix",
                            usbforward: "/dev/bus/usb:/dev/bus/usb",
                            extrabinding: "/dev/ttyACM0:/dev/ttyACM0", // Some more if needed /run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/dev/snd:/dev/snd,/dev/dri:/dev/dri
                            imagename: "myrfswift:latest",
                            shell: "/bin/bash"} // Instance with default values


func DockerSetx11(x11forward string) {
    /* Sets the shell to use in the Docker container
        in(1): string command shell to use
    */
    if (x11forward != "") {
        dockerObj.x11forward = x11forward
    }
}

func DockerSetShell(shellcmd string) {
    /* Sets the shell to use in the Docker container
        in(1): string command shell to use
    */
    if (shellcmd != "") {
        dockerObj.shell = shellcmd
    }
}

func DockerAddBiding(addbindings string) {
    /* Add extra bindings to the Docker container to run
        in(1): string of bindings separated by commas
    */
    if (addbindings != "") {
        dockerObj.extrabinding = addbindings
    }
}

func DockerSetImage(imagename string) {
    /* Set image name to use if the default one is not used
        in(1): string image name
    */ 
    if (imagename != "") {
        dockerObj.imagename = imagename
    }
}

func DockerSetXDisplay(display string) {
    /* Sets the XDISPLAY env variable value
        in(1): string display
    */
    if (display != "") {
        dockerObj.xdisplay = display
    }
}

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
        if (ifilter != "") {
            if (container.Image == ifilter) {
                fmt.Println("[",container.Created,"][",container.Image,"] Container: ", container.ID, ", Command: ", container.Command)
            }
        } else {
            fmt.Println("[",container.Created,"][",container.Image,"] Container: ", container.ID, ", Command: ", container.Command)
        }   
    }
}

func DockerRun() {
    /* Create a container and run it
    */
    inout = make(chan []byte)
    ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()
 
    x11split := strings.Split(dockerObj.x11forward, ",")
    var bindings = x11split
    //bindings = append(bindings, dockerObj.x11split...)
    bindings = append(bindings, strings.Split(dockerObj.usbforward, ",")...)

    if (dockerObj.extrabinding != "") {
        bindings = append(bindings, strings.Split(dockerObj.extrabinding, ",")...)
    }

    resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:           dockerObj.imagename,
        //Entrypoint:      []string{dockerObj.entrypoint},
		Cmd:             []string{dockerObj.shell},
		NetworkDisabled: false,
        Env: []string{dockerObj.xdisplay},
        OpenStdin: true,
        StdinOnce: true,
        AttachStdin: true,
        AttachStdout: true,
        AttachStderr:true,
        Tty: true,
    }, &container.HostConfig{
            NetworkMode: "host",
            Binds: bindings,
            Privileged: true,
            //ExtraHosts: []string{"pluto.local:192.168.2.1"}, // TODO: look if needed later
	}, &network.NetworkingConfig{}, nil,"")

	if err != nil {
		panic(err)
	}

    waiter, err := cli.ContainerAttach(ctx, resp.ID, container.AttachOptions{
        Stderr:       true,
        Stdout:       true,
        Stdin:        true,
        Stream:       true,
    })

    if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
        panic(err)
    }

    // readapted from https://stackoverflow.com/questions/58732588/accept-user-input-os-stdin-to-container-using-golang-docker-sdk-interactive-co
    go io.Copy(os.Stdout, waiter.Reader)
    go io.Copy(os.Stderr, waiter.Reader)
    go io.Copy(waiter.Conn, os.Stdin)

    if err != nil {
        panic(err)
    }

    fd:=int(os.Stdin.Fd())
    var oldState *terminal.State
    if terminal.IsTerminal(fd) {
        oldState, err = terminal.MakeRaw(fd)
        if err != nil {// TODO handle error?
        }

        go func(){
                for {
                        consoleReader:= bufio.NewReaderSize(os.Stdin,1)
                        input,_:= consoleReader.ReadByte() // Ctrl-C = 3
                        /*if (input ==3) {
                            cli.ContainerRemove( context.Background(), cont.ID, types.ContainerRemoveOptions{Force:true,})
                        }*/
                        waiter.Conn.Write([]byte{input})
                }
        }()
    }
    statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNextExit)
	select {
	    case err := <-errCh:
		    if err != nil {
			    panic(err)
		    }
	    case <-statusCh:
	}

    if terminal.IsTerminal(fd) {
        terminal.Restore(fd, oldState)
    }

	out, err := cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true})
	if err != nil {
		panic(err)
	}

    stdcopy.StdCopy(os.Stdout, os.Stderr, out)
}

func latestDockerID() string {
    /* Get latest Docker container ID by image name
        out: string container ID
    */
    latestID := ""
    ctx := context.Background()
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        panic(err)
    }
    defer cli.Close()

    containers, err := cli.ContainerList(ctx, container.ListOptions{Latest: true, All: true})
    if err != nil {
        panic(err)
    }

    for _, container := range containers {
    	if (container.Image == dockerObj.imagename) {
            latestID = container.ID
            break
        }
    }
    return latestID
}

func DockerExec(contid string, WorkingDir string) {
    /* Start last or specified container ID and execute a program inside
        in(1): string container ID
    */
    ctx := context.Background()
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    execShell := []string{}

    if err != nil {
        panic(err)
    }
    defer cli.Close()


    if (contid == "") {
        contid = latestDockerID()
    }

    if err := cli.ContainerStart(ctx, contid, container.StartOptions{}); err != nil {
        panic(err)
    }

    var oldState *terminal.State

    if (dockerObj.shell != "") {
        execShell = append(execShell, strings.Split(dockerObj.shell, " ")...)
    }

    if (dockerObj.shell != "/bin/bash") { // Attach and Exec the binarr
        optionsCreate := types.ExecConfig{
            WorkingDir: WorkingDir,
            AttachStdin: true,
            AttachStdout: true,
            AttachStderr:true,
            Detach: false,
            Privileged: true,
            Tty: true,
            Cmd:        execShell,
        }

        fmt.Println(WorkingDir)

        rst, err := cli.ContainerExecCreate(ctx, contid, optionsCreate)
        if err != nil {
            panic(err)
        }

        optionsStartCheck := types.ExecStartCheck{
            Detach: false,
            Tty: true,
        }

        response, err := cli.ContainerExecAttach(ctx, rst.ID, optionsStartCheck)
        if err != nil {
            panic(err)
        }

        go io.Copy(os.Stdout, response.Reader)
        go io.Copy(os.Stderr, response.Reader)
        go io.Copy(response.Conn, os.Stdin)
        defer response.Close()
        
        statusCh, errCh := cli.ContainerWait(ctx, contid, container.WaitConditionNextExit)
        select {
            case err := <-errCh:
                if err != nil {
                    panic(err)
                }
            case <-statusCh:
        }
    } else { // Interactive mode
        response, err := cli.ContainerAttach(ctx, contid, container.AttachOptions{
            Stderr:       true,
            Stdout:       true,
            Stdin:        true,
            Stream:       true,
        })

        go io.Copy(os.Stdout, response.Reader)
        go io.Copy(os.Stderr, response.Reader)
        go io.Copy(response.Conn, os.Stdin)

        if err != nil {
            panic(err)
        }

        fd:=int(os.Stdin.Fd())
        if terminal.IsTerminal(fd) {
            oldState, err = terminal.MakeRaw(fd)
            if err != nil {// TODO handle error?
            }

            go func(){
                    for {
                            consoleReader:= bufio.NewReaderSize(os.Stdin,1)
                            input,_:= consoleReader.ReadByte() // Ctrl-C = 3
                            /*if (input ==3) {
                                cli.ContainerRemove( context.Background(), cont.ID, types.ContainerRemoveOptions{Force:true,})
                            }*/
                            response.Conn.Write([]byte{input})
                    }
            }()
        }
        statusCh, errCh := cli.ContainerWait(ctx, contid, container.WaitConditionNextExit)
        select {
            case err := <-errCh:
                if err != nil {
                    panic(err)
                }
            case <-statusCh:
        }

        if terminal.IsTerminal(fd) {
            terminal.Restore(fd, oldState)
        }
    }
}

// TODO: Optimize it and handle errors
func DockerInstallFromScript(contid string) {
    /* Hot install inside a created Docker container
        in(1): string function script to use
    */
    s := fmt.Sprintf("./entrypoint.sh %s", dockerObj.shell)
    fmt.Println(s)
    dockerObj.shell = s
    DockerExec(contid, "/root/scripts")
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

    if (imagetag == "") { // if tag is empty, keep same tag
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