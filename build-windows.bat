@echo off
REM This code is part of RF Switch by @Penthertz
REM Author(s): Sébastien Dudek (@FlUxIuS)

setlocal enabledelayedexpansion

set "GREEN="
set "RED="
set "YELLOW="
set "NC="

REM Function to check Docker installation
:check_docker
docker --version >nul 2>&1
if %errorlevel% neq 0 (
    echo Docker is not installed. Do you want to install it now? (yes/no)
    set /p install_docker="Choose an option: "
    if "%install_docker%" == "yes" (
        echo Installing Docker...
        powershell -Command "Invoke-WebRequest -UseBasicParsing https://get.docker.com/ | Invoke-Expression"
        powershell -Command "Start-Service docker"
        powershell -Command "Set-Service -Name docker -StartupType Automatic"
        echo Docker installed successfully.
    ) else (
        echo Docker is required to proceed. Exiting.
        exit /b 1
    )
) else (
    echo Docker is already installed. Moving on.
)
goto :eof

REM Function to install Go
:install_go
where go >nul 2>&1
if %errorlevel% == 0 (
    echo golang is already installed and in PATH. Moving on.
    goto :eof
)

if exist "C:\Go\bin\go.exe" (
    echo golang is already installed in C:\Go\bin. Moving on.
    goto :eof
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
goto :eof

REM Function to build RF Switch Go Project
:building_rfswift
cd go\rfswift
go build .
move rfswift.exe ..\..\
cd ..\..
echo RF Switch Go Project built successfully.
goto :eof

REM Main script execution
echo Checking Docker installation
call :check_docker

echo Installing Go
call :install_go

echo Building RF Switch Go Project
call :building_rfswift

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
    set /p imagename="Enter image tag value (default: %DEFAULT_IMAGE%): "
    set /p dockerfile="Enter value for Dockerfile to use (default: %DEFAULT_DOCKERFILE%): "

    REM Use default values if variables are empty
    if "!imagename!" == "" set "imagename=%DEFAULT_IMAGE%"
    if "!dockerfile!" == "" set "dockerfile=%DEFAULT_DOCKERFILE%"

    echo Building the Docker container
    docker build . -t !imagename! -f !dockerfile!
) else if "%option%" == "2" (
    set "DEFAULT_PULL_IMAGE=penthertz/rfswift:latest"
    set /p pull_image="Enter the image tag to pull (default: %DEFAULT_PULL_IMAGE%): "
    if "!pull_image!" == "" set "pull_image=%DEFAULT_PULL_IMAGE%"

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