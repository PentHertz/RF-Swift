# 🚀 RF Swift 📡 🛠️

<div align="center">
  <img alt="RF Swift logo" width="600" src="https://github.com/PentHertz/RF-Swift-docs/blob/main/.assets/logo.png?raw=true">
  <br><br>
  <img alt="linux supported" src="https://img.shields.io/badge/linux-supported-success">
  <img alt="windows supported" src="https://img.shields.io/badge/windows-supported-success">
  <img alt="macOS supported" src="https://img.shields.io/badge/macos-supported%20without%20USB%20forward-success">
  
  <br>
  <img alt="amd64" src="https://img.shields.io/badge/amd64%20(x86__64)-supported-success">
  <img alt="arm64" src="https://img.shields.io/badge/arm64%20(aarch64)-supported-success">
  <img alt="riscv64" src="https://img.shields.io/badge/riscv64-supported-success">
  <br><br>
   <a target="_blank" rel="noopener noreferrer" href="https://www.blackhat.com/eu-24/arsenal/schedule/index.html#rf-swift-a-swifty-toolbox-for-all-wireless-assessments-41157" title="Schedule">
   <img alt="Black Hat Europe 2024" src="https://img.shields.io/badge/Black%20Hat%20Arsenal-Europe%202024-blueviolet">
  </a>
  <a target="_blank" rel="noopener noreferrer" href="https://spectrum-conference.org/24/schedule" title="Schedule">
   <img alt="Spectrum 24" src="https://img.shields.io/badge/Spectrum-2024-yellow">
  </a>
  <a target="_blank" rel="noopener noreferrer" href="https://fosdem.org/2025/schedule/event/fosdem-2025-4301-rf-swift-a-swifty-toolbox-for-all-wireless-assessments/" title="Schedule">
   <img alt="FOSDEM 2025" src="https://img.shields.io/badge/FOSDEM-2025-pink">
  </a>
  <a target="_blank" rel="noopener noreferrer" href="https://www.cyberonboard.org/en/content/sujetsscientifiques" title="Schedule">
   <img alt="CyberOnBoard" src="https://img.shields.io/badge/CyberOnBoard-2025-green">
  </a>
  <a target="_blank" rel="noopener noreferrer" href="https://www.prasec.cz/index.html#topics" title="Schedule">
   <img alt="PraSec" src="https://img.shields.io/badge/PraSec-2025-green">
  </a>
  <br><br>
  <a target="_blank" rel="noopener noreferrer" href="https://x.com/intent/follow?screen_name=FlUxIuS" title="Follow"><img src="https://img.shields.io/twitter/follow/_nwodtuhs?label=FlUxIuS&style=social" alt="Twitter FlUxIuS"></a>
  <a target="_blank" rel="noopener noreferrer" href="https://x.com/intent/follow?screen_name=Penthertz" title="Follow"><img src="https://img.shields.io/twitter/follow/_nwodtuhs?label=Penthertz&style=social" alt="Twitter Penthertz"></a>
  <br><br>
  <a target="_blank" rel="noopener noreferrer" href="https://discord.gg/NS3HayKrpA" title="Join us on Discord"><img src="https://github.com/PentHertz/RF-Swift-docs/blob/main/.assets/discord_join_us.png?raw=true" width="150" alt="Join us on Discord"></a>
  <br><br>
</div>

## 🔍 What is RF Swift?

RF Swift is a revolutionary toolbox that transforms any computer into a powerful RF testing laboratory without requiring a dedicated operating system. 🔄 Unlike traditional approaches that force you to sacrifice your primary OS, RF Swift brings containerized RF tools to your existing environment. 🏠

### ⚡ Why RF Swift Outperforms Dedicated OS Solutions

| Feature | RF Swift | Dedicated OS (Kali/DragonOS) |
|---------|---------|------------------------------|
| **🏠 Host OS Preservation** | ✅ Keep your existing OS | ❌ Requires dedicated partition or VM |
| **🛡️ Tool Isolation** | ✅ Tools contained without system impact | ❌ Tools can destabilize system |
| **⚡ Deployment Speed** | ✅ Seconds to deploy | ❌ Hours for full installation |
| **💾 Disk Space** | ✅ Only install tools you need | ❌ Requires 20-50GB minimum |
| **🔄 Updates** | ✅ Update individual tools without risk | ❌ System-wide updates can break functionality |
| **🌐 Multi-architecture** | ✅ x86_64, ARM64, RISCV64 and more! | ❌ Limited architecture support |
| **🔁 Reproducibility** | ✅ Identical environments everywhere | ❌ System drift between installations |
| **💼 Work Environment** | ✅ Use alongside productivity tools | ❌ Switch contexts between systems |

## ✨ Key Features

- **🏠 Non-disruptive Integration**: Run specialized RF tools while continuing to use your preferred OS for daily work
- **🧩 Modular Tool Selection**: Deploy only the tools you need, when you need them
- **🛡️ Containerized Isolation**: Prevent RF tools from affecting system stability or security
- **🌍 Cross-platform Compatibility**: Works seamlessly on Linux, Windows, and macOS
- **🔌 Dynamic Hardware Integration**: Connect and disconnect USB devices without restarting
- **🛠️ Custom Environment Creation**: Build specialized images for specific assessment needs
- **⚡ GPU Acceleration**: Dedicated images with OpenCL support for Intel and NVIDIA GPUs, and more
- **💾 Space Efficiency**: Use a fraction of the disk space required by dedicated OS solutions
- **🔄 Version Control**: Maintain multiple tool versions simultaneously without conflicts

## 🏁 Quick Start

### 💿 Installation

#### 🐧 Linux (Recommended)

```bash
# Clone the repository
git clone https://github.com/PentHertz/RF-Swift.git
cd RF-Swift

# Run the installation script
./install.sh
```

The script will:
- 📦 Install Docker, BuildX, and Go (if needed)
- 🔨 Build the RF Swift binary
- 🔊 Configure audio and X11 forwarding
- 🔗 Create an alias for easy access

#### 🪟 Windows

```powershell
# Clone the repository
git clone https://github.com/PentHertz/RF-Swift.git
cd RF-Swift

# Run the Windows build script
.\build-windows.bat
```

Additionally, install:
- 🐳 [Docker Desktop](https://docs.docker.com/desktop/install/windows-install/) for Windows
- 🔌 [usbipd](https://learn.microsoft.com/en-us/windows/wsl/connect-usb) for USB device forwarding

### 🏃‍♂️ Running Your First Container

```bash
# Pull a pre-built image
rfswift images pull -i sdr_full

# Create and run a container
rfswift run -i penthertz/rfswift:sdr_full -n my_sdr_container
```

## 🎬 Demo Videos

### 🐧 On Linux
https://github.com/PentHertz/RF-Swift/assets/715195/bb2ccd96-b688-4106-8fba-d82f84ff1ea4

### 🪟 On Windows (With GQRX)
https://github.com/PentHertz/RF-Swift/assets/715195/25a4a857-aa5a-4daa-9a08-28fa53d2f799

### 🖥️ Using OpenCL with Intel or NVIDIA GPU
![OpenCL recipe in action](https://github.com/PentHertz/RF-Swift/assets/715195/a29eedd5-b1df-40fc-97c0-4dc5323f36a8)

## 📦 Available Specialized Images

RF Swift's container approach allows for specialized environments optimized for specific tasks:

| Category | Images | Description |
|----------|--------|-------------|
| 📻 SDR | `sdr_light`, `sdr_full` | Software-defined radio tools |
| 📱 Telecom | `telecom_utils`, `telecom_2Gto3G`, `telecom_4G_5GNSA`, `telecom_5G` | Mobile network analysis |
| 📶 Short-range | `bluetooth`, `wifi`, `rfid` | Bluetooth, Wi-Fi, and RFID tools |
| 🔧 Hardware | `hardware`, `reversing` | Hardware security tools |
| 🚗 Automotive | `automotive` | Vehicle communications |

## 🌟 Real-World Advantages

### 👔 For Professionals

- **🧰 Assessment Readiness**: Deploy an RF and hardware lab in minutes at a client site
- **🔄 Tool Consistency**: Eliminate "works on my machine" issues with consistent environments
- **⚙️ Parallel Workflows**: Run multiple isolated assessments simultaneously
- **📋 Document Storage**: Keep reports and evidence separate from tools
- **🛠️ Custom Toolsets**: Create specialized containers for specific engagements

### 🔬 For Researchers

- **📊 Reproducible Research**: Share exact tool environments with colleagues
- **🧪 Experiment Isolation**: Prevent experimental configurations from affecting other work
- **🌐 Multi-platform Collaboration**: Collaborate across Linux, Windows, and macOS
- **🔢 Version Control**: Test with specific tool versions without compatibility issues
- **⚡ Resource Efficiency**: Optimize container resources for specific research tasks

### 👨‍🏫 For Educators

- **🏫 Classroom Deployment**: Identical environments for all students
- **💻 No Reformatting**: Students keep their existing OS
- **🖥️ Low Hardware Requirements**: Works on standard lab computers
- **📚 Focused Learning**: Custom containers with only the tools needed for specific lessons
- **🔄 Quick Reset**: Easily reset environments between classes

## 📖 Documentation

Comprehensive documentation is available at [rfswift.io](https://rfswift.io/), including:

- 🚀 [Getting Started Guide](https://rfswift.io/docs/getting-started/)
- 🏁 [Quick Start Tutorial](https://rfswift.io/docs/quick-start/)
- 📘 [User Guide](https://rfswift.io/docs/guide/)
- 👨‍💻 [Development Documentation](https://rfswift.io/docs/development/)
- 🧰 [List of Included Tools](https://rfswift.io/docs/guide/list-of-tools/)

## 👥 Community & Support

- 💬 [Join our Discord](https://discord.gg/NS3HayKrpA) for community support
- 🐛 [Report issues](https://github.com/PentHertz/RF-Swift/issues) on GitHub
- 🐦 Follow [FlUxIuS](https://x.com/intent/follow?screen_name=FlUxIuS) and [Penthertz](https://x.com/intent/follow?screen_name=Penthertz) on X (Twitter)

## 🤝 Contributing

Contributions are welcome! Here's how you can help:

- 🧰 **Tool Integration**: Add new tools or improve existing ones
- 📝 **Documentation**: Improve guides and examples
- 🐞 **Bug Reports**: Report issues you encounter
- 💡 **Feature Requests**: Suggest new features or improvements
- 👨‍💻 **Code Contributions**: Submit PRs to enhance functionality

## ⚖️ License

RF Swift is released under GNU GPLv3 license. See LICENSE file for details.
