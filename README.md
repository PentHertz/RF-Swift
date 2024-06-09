# RF Swift

Welcome to the RF Swift project! 🎉 Our mission is to provide all the essential tools for both HAM radio enthusiasts and RF professionals. 📡🔧

![RF Swift logo](images/logo.png "RF Swift logo")

Introducing our Go and shell script-based toolbox, designed to streamline the deployment of Docker containers for your preferred RF tools. This evolving toolkit promises even more features in the near future, making it an essential asset for RF enthusiasts.

Currently, the scripts are still under development. However, we invite you to contribute by adding any tools you find necessary for large-scale deployment.

Inspired by the remarkable [Exegol project](https://github.com/ThePorgs/Exegol), our toolbox aims to integrate all essential tools for radio analysis without requiring you to uninstall your preferred operating system. It also offers special Docker file recipes to help you conserve space based on your specific needs.

For those who prefer a single OS with all RF software, consider using [DragonOS](https://cemaxecuter.com/). But if your goal is to deploy tools within a container without affecting your host system, or saving space deploying specific recipes, this toolbox is your ideal solution.

Our philosophy is straightforward: maintain the integrity of your Linux or Windows systems while enjoying unrestricted RF experimentation. Start exploring RF without boundaries today!

## Requirements

### Linux

The tool requires only one direct dependency to install on you own:

- Docker engine (e.g: `apt install docker.io` in Ubuntu)

### Windows

You need to install 3 tools:

1. Docker Desktop by following this link (make sure it is run in WSL 2): [Docker Desktop WSL 2](https://docs.docker.com/desktop/wsl/#enabling-docker-support-in-wsl-2-distros)
2. GoLang using the MSI package: [GoLang Downloads](https://go.dev/dl/)
3. `usbipd` as described here: [Connect USB in WSL](https://learn.microsoft.com/en-us/windows/wsl/connect-usb)

To attach a USB device, you'll need to first detect the USB ID with this command line:

```
usbipd wsl list
```

Then bind and attach the device:

```
usbipd bind --busid <busid>
usbipd attach --wsl --busid <busid>
```

After that, the device should appear in the container without issues. 😊


## Quick overview

## On Linux

https://github.com/PentHertz/RF-Swift/assets/715195/bb2ccd96-b688-4106-8fba-d82f84ff1ea4

## On Windows

With GQRX ;)

https://github.com/PentHertz/RF-Swift/assets/715195/25a4a857-aa5a-4daa-9a08-28fa53d2f799

## On Mac-OS - Apple Silicon M1/M2/M3

Even if the Go program can build on any platform and Dockerfiles are compilable on macOS, the USB may be difficult to reach on this platform.

For the moment, consider using the tool inside a VM that handles USB, such as VMware Fusion.

You can directly pull a working container from our registry using the `penthertz/rfswift:sdr_full_aarch64` reference: [Docker Hub Link](https://hub.docker.com/layers/penthertz/rfswift/sdr_full_aarch64/images/sha256-3385e49c1369bad2465e85c75b74ae241a0e285f0666321620c73fc9ff260996?context=repo).

## Quick run

If you want to skip building the Go program, you can pick one of the built archives here: [RF-Swift Releases](https://github.com/PentHertz/RF-Swift/releases/tag/v0.1-dev).

If you want to use one of our built containers directly, please find the tags you can pull from our official registry: [RF-Swift Docker Tags](https://hub.docker.com/repository/docker/penthertz/rfswift/tags).

## Building

This section is about building your own images using our provided recipes, as well as your own recipes. If you want to use an already built Docker container, you can skip this section and go to the "Pulling Containers" one.

### On Linux

For the moment, the build script is rather simple and gives you the choice of using an image tag name and a specific Dockerfile:

```
./build.sh 
[+] Installing Go
...
[+] Building RF Switch Go Project
Enter image tag value (default: myrfswift:latest): 
Enter value for Dockerfile to use (default: Dockerfile):
```

Note: Uncomment some lines in the Dockerfile, particularly if you are using the GPU with OpenCL.

Note 2: The default tag used by the tool is `myrfswift:latest`. We will manage configuration files in the future to avoid specifying it for each `run` and `exec` command when using a non-default one.

### On Windows

Use the `build-windows.bat` instead after installing all the requirements.

## Pulling a container

If you want to use an already built container and save more time, you can pull one docker container from our official repository: [https://hub.docker.com/repository/docker/penthertz/rfswift/tags](https://hub.docker.com/r/penthertz/rfswift/tags)

As an example, we'd like to pull `rfswift:bluetooth` that includes all tools from the `bluetooth.docker` file, we can use the following command:

	sudo ./rfswift pull -i "penthertz/rfswift:bluetooth" -t myrfswift:bluetooth
	[...]
	{"status":"Pulling from penthertz/rfswift","id":"bluetooth"}
	{"status":"Digest: sha256:44b12f2be4f596d4d788f70c401ee757f80b1c85a1f91b5d4c69cb1260d49b88"}
	{"status":"Status: Image is up to date for penthertz/rfswift:bluetooth"}


Existing images from our container registry:

| Tag                 | Supported OS      | architecture | Description                                                                                                                                               |
|---------------------|-------------------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| corebuild           | - Linux - Windows | amd64        | Base image including prerequisites for compiling tools + common SDR devices without tweaks                                                                |
| latest              | - Linux - Windows | amd64        | The full images including all tools for SDR, Wi-Fi, Bluetooth and RFID used in the Dockerfile file                                                        |
| sdr_light           | - Linux - Windows | amd64        | Light image built for SDR uses with limited number of tools used in sdr_light.docker file                                                                 |
| sdr_full            | - Linux - Windows | amd64        | Full image including all SDR tools used in sdr_full.docker file                                                                                           |
| wifi                | - Linux - Windows | amd64        | Wi-Fi image for security tests using tools included in wifi.docker                                                                                        |
| rfid                | - Linux - Windows | amd64        | RFID image for security tests using tools included in rfid.docker                                                                                         |
| bluetooth           | - Linux - Windows | amd64        | Bluetooth classic and LE image for security tests using tools in bluetooth.docker                                                                         |
| rfid_aarch64        | - Linux - Windows | arm64/v8     | RFID image for security tests using tools included in rfid.docker                                                                                         |
| sdr_light_aarch64   | - Linux - Windows | arm64/v8     | Light image built for SDR uses with limited number of tools used in sdr_light.docker file                                                                 |
| latest_aarch64_rpi5 | - Linux - Windows | arm64/v8     | The full images including all tools for SDR, Wi-Fi, Bluetooth and RFID used in the Dockerfile file but two tools are missing. Can be used in Apple M1-M3. |
| sdr_full_rpi5       | - Linux - Windows | arm64/v8     | Full image including all SDR tools used in sdr_full.docker file but with two missing tools. Also works in Apple M1-M3.                                    |
| bluetooth_aarch64   | - Linux - Windows | arm64/v8     | Bluetooth classic and LE image for security tests using tools in bluetooth.docker                                                                         |

## Creating and running a container

To run a container, use the command `./rfswift run -h` to see the needed arguments:

```
[...]
Usage:
  rfswift run [flags]

Flags:
  -b, --bind string      extra bindings (separate them with commas)
  -e, --command string   command to exec (default: '/bin/bash')
  -d, --display string   set X Display (default: 'DISPLAY=:0')
  -h, --help             help for run
  -i, --image string     image (default: 'myrfswift:latest')
```

By default, you can run the command without arguments if you want to start the `/bin/bash` interpreter and use the default image tag name, and the default environment display variable.

### Not Able to See Some Devices

You can add extra bindings with the following command line to help bring up the PlutoSDR:

```
sudo ./rfswift run -b "/run/dbus/system_bus_socket:/run/dbus/system_bus_socket,/dev/snd:/dev/snd,/dev/dri:/dev/dri"
```

## Executing a Command Inside an Existing Container

Running a command inside a previous container is fairly easy if you run a container and exit it.

All you need to do is execute the desired command as follows:

```
sudo ./rfswift exec -e urh
```

If you want to run it in another existing container, you can specify the container ID as follows:

```
sudo ./rfswift last # to get the list
[...]
[ 1716024976 ][ myrfswift:latest ] Container:  c9e223a987a36441fb631f4a11def746aabb1a1bc862b5f2589d5b3ac8429cb1 , Command:  /bin/bash
[ 1716024209 ][ sha256:ed26c47b0d1dba0473a4729885374e04e24b4189125a245c77280fd472bf934b ] Container:  2354c99f6699b4f3abc97d55cdb825fcfafba2af1b27e02a612fc2586061eb6e , Command:  /bin/sh -c './entrypoint.sh eaphammer_soft_install'
[ 1716021780 ][ myrfswift:rfid ] Container:  a3e91704571d92f9a48e355b1db0ca5a97769087aebf573a6295392fb3f4d394 , Command:  /bin/bash
[ 1716021385 ][ sha256:95fd8938e078792fc3e09c1311c7bdbed3e8e112887b7f0f36bf5a57616cf414 ] Container:  0b922d0ee58c1235bdba13fe2793ee7544f16fc5a5a710df4ebc68b05b928cc8 , Command:  /bin/sh -c './entrypoint.sh mfoc_soft_install'
[...]
sudo ./rfswift exec -e /bin/bash -c c9e223a987a36441fb631f4a11def746aabb1a1bc862b5f2589d5b3ac8429cb1 # executing on the 'c9e223a987a36441fb631f4a11def746aabb1a1bc862b5f2589d5b3ac8429cb1' container
```

## Getting the Latest Containers

To get the last 10 containers you have created, you can use the following command:

```
sudo ./rfswift last -h
  rfswift last [flags]

Flags:
  -f, --filter string   filter by image name
[...]
sudo ./rfswift last -f myrfswift:latest # using a filter for images
[ 1716024976 ][ myrfswift:latest ] Container:  c9e223a987a36441fb631f4a11def746aabb1a1bc862b5f2589d5b3ac8429cb1 , Command:  /bin/bash
```

## Hot Install 

If you forgot to enable an installation function, you can always install it:

```
sudo ./rfswift install -i <install function (called by entrypoint.sh)> -c <container id> [-c <container id>]
```

## Commit Changes

If you want to commit changes you've made to your container and start a new one from this image later, you can use the `commit` command as follows:

```
sudo ./rfswift commit -c <container id> -i myrfswift:newtag
```

## Renaming Images

You can rename images using the `rename` command as follows:

```
sudo ./rfswift rename -i myrfswift:supertag -t myrfswift:newsupertag
```

## Removing Containers

You can remove a container using the `remove` command as follows:

```
sudo ./rfswift remove -c <container id>
```

## Options

### OpenCL

You can enable OpenCL with the driver associated with your graphics card:

```
# Installing OpenCL
## NVIDIA drivers
#RUN apt-fast install -y nvidia-opencl-dev nvidia-modprobe
## Installing Intel's OpenCL
#RUN apt-fast install -y intel-opencl-icd ocl-icd-dev ocl-icd-opencl-dev
```

![OpenCL recipe in action](https://github.com/PentHertz/RF-Swift/assets/715195/a29eedd5-b1df-40fc-97c0-4dc5323f36a8)

### RTL-SDR

The RTL-SDR v4 uses a different driver that replaces the others.

Until we find a proper way to support both drivers, comment the basic function and uncomment the v4 one in the recipe as follows:

```
#RUN ./entrypoint.sh rtlsdr_devices_install
RUN ./entrypoint.sh rtlsdrv4_devices_install # optional, remove rtlsdr_devices_install if you are using the v4 version

# Installing gr-fosphor with OpenCL
#RUN ./entrypoint.sh grfosphor_grmod_install
```

## How to Contribute

You are warmly welcomed to contribute and fill scripts with your desired tools.

In the future, we will create a dedicated page for developers.

## Troubleshooting

### Sound

The sound sometimes does not restart when stopping playback with tools like SDR++. Try using different `hw` identifiers.

Some tools, like GQRX, use **pulseaudio** so the best way would be to load `pulseaudio` on TCP giving access to container's IP address:

	pactl load-module module-native-protocol-tcp  port=34567 auth-ip-acl=<container IP address>

 Then use an environment variable while running programs like GQRX:

 	PULSE_SERVER=tcp:<host IP address>:34567 gqrx

Note: If network type mode is set to host on the container, then host is equal to container ip address. 
If any contributor has the time to smooth all of this, that would be awesome. ❤️

### Wiki??!

We will find some time to build one. 😉
