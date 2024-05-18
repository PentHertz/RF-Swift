# RF Swift

Welcome to the RF Swift project which aims to provide all needed tools for RF HAMs and professionals.

![RF Swift logo](images/logo.png "RF Swift logo")

This toolbox written in Go and shell script allows to quickly deploy Docker containers with desired RF tools, and will all provide more feautures in the near future.

For the moment, the scripts are not yet complete, but your are welcomed to fullfil them if you see there is a needed tool you want to deploy at scale.

This toolbox was inspirated from the nice [Exegol project](https://github.com/ThePorgs/Exegol) in order to integrate all needed tools for radio analysis without uninstalling your favorite OS, and save space depending on your needs via special Docker file recipes.

Still, if you want to have all RF softwares in one OS, you can alternatively use [DragonOS](https://cemaxecuter.com/). But if you want to deploy your tools, messing inside a container without affecting your host, this toolbox is for you ;)

The philosophy of this toolbox is simple -> keep your Linux or Windows systems as they are, and start to play on RF without borders!

## Requirements

### Linux

The tool requires only one direct dependency to install on you own:

- Docker engine (e.g: `apt install docker.io` in Ubuntu)

### Windows

You need to install 3 tools:

- Docker desktop following this link (making sure it is run in WSL 2): https://docs.docker.com/desktop/wsl/#enabling-docker-support-in-wsl-2-distros
- GoLang installed with MSI package: https://go.dev/dl/
- `usbipd` as described here: https://learn.microsoft.com/en-us/windows/wsl/connect-usb 

To attach USB device, you'll need first to detect the USB id you are connecting with this command line:

	usbipd wsl list

Then bind & attach this device:

	usbipd bind --busid <bus id>
	usbipd attach --wsl --busid <busid>

After that, the device should appear on the container without issues ;)


## Quick overview

## On Linux

https://github.com/PentHertz/RF-Swift/assets/715195/bb2ccd96-b688-4106-8fba-d82f84ff1ea4

## On Windows

With GQRX ;)

https://github.com/PentHertz/RF-Swift/assets/715195/25a4a857-aa5a-4daa-9a08-28fa53d2f799

## Building

### On Linux

For the momemt the building script is rather simple and give you the choice of using a image tag name and a specific Docker file:

	./build.sh 
	[+] Installing Go
	...
	[+] Building RF Switch Go Project
	Enter image tag value (default: myrfswift:latest): 
	Enter value for Dockerfile to use (default: Dockerfile):

Note: uncomment some lines in Docker files, particularly if you are using the GPU with OpenCL

### On Windows

Use the `build-windows.bat` instead after installing all the requirements.


## Creating and running a container

To run a container, use the command `./rfswift run -h` to see needed arguments:

	[...]
	Usage:
	  rfswift run [flags]

	Flags:
	  -b, --bind string      extra bindings (separe them with commas)
	  -e, --command string   command to exec (by default: '/bin/bash')
	  -d, --display string   set X Display (by default: 'DISPLAY=:0')
	  -h, --help             help for run
	  -i, --image string     image (by default: 'myrfswift:latest')


By default, you can the command without arguments if you want to start the `/bin/bash` interpreter and use the default image tag name, and with default environement diplay variable.

## Executing a command inside an existing container

Running a command inside a previous container is fairly easy, if you run a cointainer and exit it.

All you need to do, is to execute the desire command as follows:

	sudo ./rfswift exec -e urh

if you want to run it into another existing container, you can precise the container ID as follows:

	sudo ./rfswift last # to get the list
	[...]
	[ 1716024976 ][ myrfswift:latest ] Container:  c9e223a987a36441fb631f4a11def746aabb1a1bc862b5f2589d5b3ac8429cb1 , Command:  /bin/bash
	[ 1716024209 ][ sha256:ed26c47b0d1dba0473a4729885374e04e24b4189125a245c77280fd472bf934b ] Container:  2354c99f6699b4f3abc97d55cdb825fcfafba2af1b27e02a612fc2586061eb6e , Command:  /bin/sh -c './entrypoint.sh eaphammer_soft_install'
	[ 1716021780 ][ myrfswift:rfid ] Container:  a3e91704571d92f9a48e355b1db0ca5a97769087aebf573a6295392fb3f4d394 , Command:  /bin/bash
	[ 1716021385 ][ sha256:95fd8938e078792fc3e09c1311c7bdbed3e8e112887b7f0f36bf5a57616cf414 ] Container:  0b922d0ee58c1235bdba13fe2793ee7544f16fc5a5a710df4ebc68b05b928cc8 , Command:  /bin/sh -c './entrypoint.sh mfoc_soft_install'
	[...]
	sudo ./rfswift exec -e /bin/bash -c c9e223a987a36441fb631f4a11def746aabb1a1bc862b5f2589d5b3ac8429cb1 # we are executing on the 'c9e223a987a36441fb631f4a11def746aabb1a1bc862b5f2589d5b3ac8429cb1' container


## Getting the latests containers

To get the 10 last containers you have create, you can use the following command:

	sudo ./rfswift last -h
	  rfswift last [flags]

	Flags:
	  -f, --filter string   filter by image name
	[...]
	sudo ./rfswift last -f myrfswift:latest # we are using a filter for images
	[ 1716024976 ][ myrfswift:latest ] Container:  c9e223a987a36441fb631f4a11def746aabb1a1bc862b5f2589d5b3ac8429cb1 , Command:  /bin/bash

## Options

### OpenCL

You can enable OpenCL with the driver associated to your graphic card:

	# Installing OpenCL
	## NVidia drivers
	#RUN apt-fast install -y nvidia-opencl-dev nvidia-modprobe
	## Installing Intel's OpenCL
	#RUN apt-fast install -y intel-opencl-icd ocl-icd-dev ocl-icd-opencl-dev

 ### RTL-SDR

 The RTL-SDR v4 uses a different driver that replaces the others.

Until we find a proper way to support both drivers, comment the basic function and uncomment the v4 one in the recipe as follow:

	#RUN ./entrypoint.sh rtlsdr_devices_install
	RUN ./entrypoint.sh rtlsdrv4_devices_install # optionnal, remove rtlsdr_devices_install if you are using the v4 version

	# Installing gr-fosphor with OpenCL
	#RUN ./entrypoint.sh grfosphor_grmod_install

## How to contribute

You are warmly welcomed to contribute to fill scripts we your desired tools.

In the future, we will create a dedicated page for developpers.

## Troubleshooting

### Sound

The sound is sometimes notes restarting when stoping to play with tools like SDR++. 

To solve it for the moment, you can restart the tool and try playing it.

Some tools like GQRX are not yet working with the sound, we will try to fix it when possible, but you can also capture the signal and demodulate it to `wav` and play it with Audacity as a quick fix.

If some contributor have the time to solve this issue, that would be awesome <3

### Wiki??!

We will find out some time to build one ;)
