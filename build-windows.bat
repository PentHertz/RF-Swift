@echo off
REM This code is part of RF Switch by @Penthertz
REM Author(s): Sébastien Dudek (@FlUxIuS)

setlocal enabledelayedexpansion

echo [i] Checking if Docker is installed
REM Function to check and activate Docker if installed
:check_docker
docker --version >nul 2>&1
if %errorlevel% neq 0 (
    echo Docker is not installed. Please install Docker to proceed. Exiting.
    exit /b 1
) 
echo [+] Docker is installed!

echo [i] Checking if Go is installed
REM Function to install Go
:install_go
where go >nul 2>&1
if %errorlevel% == 0 (
    echo golang is already installed and in PATH. Moving on.
    goto building_rfswift
)

REM Provide link to download the MSI installer based on architecture
set "arch=%PROCESSOR_ARCHITECTURE%"
set "version=1.23.4"

if "%arch%" == "AMD64" (
    echo [i] Unsupported architecture detected or Go not installed.
    echo [!] Please download the Go MSI installer manually from the following link:
    echo [!] https://go.dev/dl/go%version%.windows-amd64.msi
    echo Press any key to open the download page in your browser...
    pause >nul
    start https://go.dev/dl/go%version%.windows-amd64.msi
    exit /b 1
) else if "%arch%" == "x86" (
    echo [i] Unsupported architecture detected or Go not installed.
    echo [!] Please download the Go MSI installer manually from the following link:
    echo [!] https://go.dev/dl/go%version%.windows-386.msi
    echo Press any key to open the download page in your browser...
    pause >nul
    start https://go.dev/dl/go%version%.windows-386.msi
    exit /b 1
) else (
    echo [!] Unsupported architecture: "%arch%"
    echo [!] Please visit https://go.dev/dl/ to download an appropriate installer for your system.
    exit /b 2
)
setx PATH "%PATH%;C:\Go\bin"
cd ..
rmdir /s /q thirdparty
echo Go installed successfully.
goto building_rfswift

REM Function to build RF Switch Go Project
:building_rfswift
cd go\rfswift
go build .
move rfswift.exe ..\..\
cd ..\..
echo RF Switch Go Project built successfully.

REM Prompt the user if they want to build a Docker container, pull an image, or exit
echo Do you want to build a Docker container, pull an existing image, or exit?
echo 1) Build Docker container
echo 2) Pull Docker image
echo 3) Exit
set /p option="Choose an option (1, 2, or 3): "

if "%option%" == "1" (
    REM Set default values
    set "DEFAULT_IMAGE=myrfswift:latest"
    set "DEFAULT_DOCKERFILE=Dockerfile"
    set "DEFAULT_REDIR=."

    REM Prompt the user for input with default values

    set /p ressourcesdir="Enter ressources directory where configuration and scripts are placed (default: !DEFAULT_REDIR!): "
    set /p imagename="Enter image tag value (default: !DEFAULT_IMAGE!): "
    set /p dockerfile="Enter value for Dockerfile to use (default: !DEFAULT_DOCKERFILE!): "

    REM Use default values if variables are empty
    if "!ressourcesdir!" == "" set "imagename=!DEFAULT_REDIR!"
    if "!imagename!" == "" set "imagename=!DEFAULT_IMAGE!"
    if "!ressourcesdir!" == "" set "dockerfile=!DEFAULT_DOCKERFILE!"

    echo Building the Docker container
    docker build !imagename! -t !imagename! -f !dockerfile!
) else if "%option%" == "2" (
    rfswift.exe images remote
    set "DEFAULT_PULL_IMAGE=penthertz/rfswift:latest"
    set /p pull_image="Enter the image tag to pull (default: !DEFAULT_PULL_IMAGE!): "
    if "!pull_image!" == "" set "pull_image=!DEFAULT_PULL_IMAGE!"

    echo Pulling the Docker image
    docker pull !pull_image!
) else if "%option%" == "3" (
    echo Exiting without building or pulling Docker images.
    exit /b 0
) else (
    echo Invalid option. Exiting.
    exit /b 1
)

endlocal
exit /b 0