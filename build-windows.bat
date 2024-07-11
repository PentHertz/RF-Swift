@echo off

:: This code is part of RF Switch by @Penthertz
:: Author(s): Sébastien Dudek (@FlUxIuS)

setlocal enabledelayedexpansion
set "GOVERSION=1.22.5"

install_go() (
    go version >nul 2>&1 && (
        echo golang is already installed. moving on
        exit /b 0
    )

    if not exist thirdparty mkdir thirdparty
    cd thirdparty
    for /f "tokens=2 delims==" %%a in ('"wmic os get osarchitecture /value"') do set "arch=%%a"
    set "arch=!arch: =!"

    set "prog="
    if "!arch!"=="64-bit" (
        set "prog=go%GOVERSION%.windows-amd64.zip"
    ) else if "!arch!"=="32-bit" (
        set "prog=go%GOVERSION%.windows-386.zip"
    ) else (
        echo Unsupported architecture: "!arch!" -> Download or build Go instead
        exit /b 2
    )

    powershell -command "Invoke-WebRequest -Uri https://go.dev/dl/%prog% -OutFile %prog%"
    rmdir /s /q C:\Go
    powershell -command "Expand-Archive -Path %prog% -DestinationPath C:\Go"
    setx PATH "%PATH%;C:\Go\bin"
    cd ..
    rmdir /s /q thirdparty
)

building_rfswift() (
    cd go\rfswift
    go build .
    move rfswift.exe ..\..
    cd ..\..
)

echo [+] Installing Go
install_go

echo [+] Building RF Switch Go Project
building_rfswift

REM Prompt the user if they want to build a Docker container or pull an image
echo Do you want to build a Docker container or pull an existing image?
echo 1) Build Docker container
echo 2) Pull Docker image
set /p option=Choose an option (1 or 2): 

if "%option%"=="1" (
    REM Set default values
    set "DEFAULT_IMAGE=myrfswift:latest"
    set "DEFAULT_DOCKERFILE=Dockerfile"

    REM Prompt the user for input with default values
    set /p imagename=Enter image tag value (default: %DEFAULT_IMAGE%): 
    set /p dockerfile=Enter value for Dockerfile to use (default: %DEFAULT_DOCKERFILE%): 

    REM Use default values if variables are empty
    if not defined imagename set "imagename=%DEFAULT_IMAGE%"
    if not defined dockerfile set "dockerfile=%DEFAULT_DOCKERFILE%"

    echo [+] Building the Docker container
    docker build . -t %imagename% -f %dockerfile%
) else if "%option%"=="2" (
    set "DEFAULT_IMAGE=penthertz/rfswift:latest"
    set /p pull_image=Enter the image tag to pull (default: %DEFAULT_IMAGE%): 
    if not defined pull_image set "pull_image=%DEFAULT_IMAGE%"

    echo [+] Pulling the Docker image
    docker pull %pull_image%
) else (
    echo Invalid option. Exiting.
    exit /b 1
)