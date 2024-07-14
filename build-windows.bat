@echo off
REM This code is part of RF Switch by @Penthertz
REM Author(s): Sébastien Dudek (@FlUxIuS)

setlocal enabledelayedexpansion

REM Function to check and activate Docker if installed
:check_docker
docker --version >nul 2>&1
if %errorlevel% neq 0 (
    echo Docker is not installed. Please install Docker to proceed. Exiting.
    exit /b 1
) 

REM Function to install Go
:install_go
where go >nul 2>&1
if %errorlevel% == 0 (
    echo golang is already installed and in PATH. Moving on.
    goto building_rfswift
)

if not exist thirdparty mkdir thirdparty
cd thirdparty
set "arch=%PROCESSOR_ARCHITECTURE%"
set "prog="
set "version=1.22.5"

if "%arch%" == "AMD64" (
    set "prog=go%version%.windows-amd64.zip"
) else if "%arch%" == "x86" (
    set "prog=go%version%.windows-386.zip"
) else (
    echo Unsupported architecture: "%arch%" -> Download or build Go instead
    exit /b 2
)

powershell -Command "Invoke-WebRequest -OutFile %prog% https://go.dev/dl/%prog%"
powershell -Command "Expand-Archive -Path %prog% -DestinationPath C:\"
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

    REM Prompt the user for input with default values
    set /p imagename="Enter image tag value (default: !DEFAULT_IMAGE!): "
    set /p dockerfile="Enter value for Dockerfile to use (default: !DEFAULT_DOCKERFILE!): "

    REM Use default values if variables are empty
    if "!imagename!" == "" set "imagename=!DEFAULT_IMAGE!"
    if "!dockerfile!" == "" set "dockerfile=!DEFAULT_DOCKERFILE!"

    echo Building the Docker container
    docker build . -t !imagename! -f !dockerfile!
) else if "%option%" == "2" (
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