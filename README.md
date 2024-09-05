# RF Swift

Welcome to the RF Swift project! 🎉 Our mission is to provide all the essential tools for both HAM radio enthusiasts and RF professionals. 📡🔧

<div align="center">
  <img alt="RF Swift logo" width="600" src="https://github.com/PentHertz/RF-Swift-docs/blob/main/.assets/logo.png?raw=true">
  <br><br>
  <img alt="linux supported" src="https://img.shields.io/badge/linux-supported-success">
  <img alt="windows supported" src="https://img.shields.io/badge/windows-supported-success">
  <img alt="windows supported" src="https://img.shields.io/badge/macos-pending-orange">
	
  <br>
  <img alt="amd64" src="https://img.shields.io/badge/amd64%20(x86__64)-supported-success">
  <img alt="arm64" src="https://img.shields.io/badge/arm64%20(aarch64)-supported-success">
  <img alt="risc64" src="https://img.shields.io/badge/risc64-supported-success">
  <br><br>
  <a target="_blank" rel="noopener noreferrer" href="https://x.com/intent/follow?screen_name=FlUxIuS" title="Follow"><img src="https://img.shields.io/twitter/follow/_nwodtuhs?label=FlUxIuS&style=social" alt="Twitter FlUxIuS"></a>
  <a target="_blank" rel="noopener noreferrer" href="https://x.com/intent/follow?screen_name=Penthertz" title="Follow"><img src="https://img.shields.io/twitter/follow/_nwodtuhs?label=Penthertz&style=social" alt="Twitter Dramelac"></a>
  <br><br>
  <a target="_blank" rel="noopener noreferrer" href="https://discord.gg/NS3HayKrpA" title="Join us on Discord"><img src="https://github.com/PentHertz/RF-Swift-docs/blob/main/.assets/discord_join_us.png?raw=true" width="150" alt="Join us on Discord"></a>
  <br><br>
</div><div class="toctree-wrapper compound">
</div>

Introducing our Go and shell script-based toolbox, designed to streamline the deployment of Docker containers for your preferred RF tools. This evolving toolkit promises even more features in the near future, making it an essential asset for RF enthusiasts.

Currently, the scripts are still under development. However, we invite you to contribute by adding any tools you find necessary for large-scale deployment.

Inspired by the remarkable [Exegol project](https://github.com/ThePorgs/Exegol), our toolbox aims to integrate all essential tools for radio analysis without requiring you to uninstall your preferred operating system. It also offers special Docker file recipes to help you conserve space based on your specific needs.

For those who prefer a single OS with all RF software, consider using [DragonOS](https://cemaxecuter.com/). But if your goal is to deploy tools within a container without affecting your host system, or saving space deploying specific recipes, this toolbox is your ideal solution.

Our philosophy is straightforward: maintain the integrity of your Linux or Windows systems while enjoying unrestricted RF experimentation. Start exploring RF without boundaries today!

## Documentation

We have a new [documentation that will guide you through the different steps](https://rfswift.io/).

A list of included tools can be also seen [here](https://rfswift.io/docs/guide/list-of-tools/).

## Quick overview

## On Linux

https://github.com/PentHertz/RF-Swift/assets/715195/bb2ccd96-b688-4106-8fba-d82f84ff1ea4

## On Windows

With GQRX ;)

https://github.com/PentHertz/RF-Swift/assets/715195/25a4a857-aa5a-4daa-9a08-28fa53d2f799

## Using OpenCL with Intel or Nvidia GPU

![OpenCL recipe in action](https://github.com/PentHertz/RF-Swift/assets/715195/a29eedd5-b1df-40fc-97c0-4dc5323f36a8)

## On Mac-OS - Apple Silicon M1/M2/M3

Even if the Go program can build on any platform and Dockerfiles are compilable on macOS, the USB may be difficult to reach on this platform.

For the moment, consider using the tool inside a VM that handles USB, such as VMware Fusion.

You can directly pull a working container from our registry using the `penthertz/rfswift:sdr_full_aarch64` reference: [Docker Hub Link](https://hub.docker.com/layers/penthertz/rfswift/sdr_full_aarch64/images/sha256-3385e49c1369bad2465e85c75b74ae241a0e285f0666321620c73fc9ff260996?context=repo).

## How to Contribute

You are warmly welcomed to contribute and fill scripts with your desired tools.

In the future, we will create a dedicated page for developers.
