@echo off

:: This code is part of RF Switch by @Penthertz
:: Author(s): Sébastien Dudek (@FlUxIuS)

setlocal enabledelayedexpansion

:: Stop the script if any command fails
set "errorlevel="
if not defined errorlevel goto :eof

:install_go
go version >nul 2>&1
if %errorlevel% equ 0 (
    echo golang is already installed. moving on
    goto :install_usbipd
)

if not exist thirdparty mkdir thirdparty
cd thirdparty
for /f "tokens=2 delims==" %%i in ('wmic os get osarchitecture /value') do set "arch=%%i"
set "prog="
set "version=1.22.4"

if "%arch%"=="64-bit" (
    set "prog=go%version%.windows-amd64.zip"
) else if "%arch%"=="32-bit" (
    set "prog=go%version%.windows-386.zip"
) else (
    echo Unsupported architecture: %arch% -> Download or build Go instead
    exit /b 2
)

powershell -command "Invoke-WebRequest -Uri 'https://go.dev/dl/%prog%' -OutFile '%prog%'"
powershell -command "Expand-Archive -Path '%prog%' -DestinationPath 'C:\Go'"
setx PATH "%PATH%;C:\Go\bin"
cd ..

:install_usbipd
if not exist thirdparty mkdir thirdparty
cd thirdparty

:: Download and install usbipd-win_4.2.0.msi
powershell -command "Invoke-WebRequest -Uri 'https://github.com/dorssel/usbipd-win/releases/download/v4.2.0/usbipd-win_4.2.0.msi' -OutFile 'usbipd-win_4.2.0.msi'"
msiexec /i usbipd-win_4.2.0.msi /quiet /norestart

cd ..

:: Build rfswift
cd go\rfswift
go build .
move rfswift.exe ..\..
cd ..\..

:: Set default values
set "DEFAULT_IMAGE=myrfswift:latest"
set "DEFAULT_DOCKERFILE=Dockerfile"

:: Prompt the user for input with default values
set /p "imagename=Enter image tag value (default: %DEFAULT_IMAGE%): "
set /p "dockerfile=Enter value for Dockerfile to use (default: %DEFAULT_DOCKERFILE%): "

:: Use default values if variables are empty
if "%imagename%"=="" set "imagename=%DEFAULT_IMAGE%"
if "%dockerfile%"=="" set "dockerfile=%DEFAULT_DOCKERFILE%"

echo [+] Building the Docker container
docker build . -t %imagename% -f %dockerfile%

endlocal