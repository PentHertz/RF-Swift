@echo off
REM This code is part of RF Swift by @Penthertz
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

echo [i] Checking if RF Swift configuration file exists and is valid
call :check_config_file
if %errorlevel% neq 0 (
    echo [!] Configuration file has missing keys. Please address the issues before continuing.
    set /p continue_anyway="Do you want to continue anyway? (yes/no): "
    if /i not "!continue_anyway!" == "yes" (
        exit /b 1
    )
)

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
set "version=1.24.2"
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

REM Function to build RF Swift Go Project
:building_rfswift
cd go\rfswift
go build .
move rfswift.exe ..\..\
cd ..\..
echo RF Swift Go Project built successfully.

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
    set "DEFAULT_PULL_IMAGE=penthertz/rfswift:sdr_light"
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

goto :eof

REM Function to check if config.ini has all required fields
:check_config_file
setlocal
set "CONFIG_PATH=%APPDATA%\rfswift\config.ini"
echo [i] Checking configuration file at: %CONFIG_PATH%

REM Check if config file exists
if not exist "%CONFIG_PATH%" (
    echo [!] Config file not found at %CONFIG_PATH%
    echo [+] A new config file will be created on first run.
    exit /b 0
)

REM Define required sections and keys
set "general_required=imagename repotag"
set "container_required=shell bindings network exposedports portbindings x11forward xdisplay extrahost extraenv devices privileged caps seccomp cgroups"
set "audio_required=pulse_server"

set missing_fields=0
set current_section=

REM Temporary files for processing
set "temp_file=%TEMP%\rfswift_config_check.tmp"
set "debug_file=%TEMP%\rfswift_config_debug.tmp"

echo [i] Scanning config file for keys...
echo. > "%debug_file%"

REM Process the config file line by line
for /f "usebackq tokens=* delims=" %%a in ("%CONFIG_PATH%") do (
    set "line=%%a"
    set "line=!line: =!"
    
    REM Skip empty lines and comments
    if not "!line!" == "" if not "!line:~0,1!" == "#" (
        REM Check if line is a section header
        if "!line:~0,1!!line:~-1!" == "[]" (
            set "current_section=!line:~1,-1!"
            echo [i] Found section: [!current_section!] >> "%debug_file%"
        ) else (
            REM Check if line is a key-value pair
            for /f "tokens=1,* delims==" %%b in ("!line!") do (
                set "key=%%b"
                REM Trim spaces from key
                set "key=!key: =!"
                
                if not "!key!" == "" (
                    echo [i] Found key: !key! in section [!current_section!] >> "%debug_file%"
                    
                    REM Check which section we're in and remove key from the required list
                    if "!current_section!" == "general" (
                        set "general_required=!general_required: !key! = !"
                        set "general_required=!general_required:!key! = !"
                        set "general_required=!general_required: !key!= !"
                        set "general_required=!general_required:!key!= !"
                        set "general_required=!general_required: !key! =!"
                        set "general_required=!general_required:!key! =!"
                    ) else if "!current_section!" == "container" (
                        set "container_required=!container_required: !key! = !"
                        set "container_required=!container_required:!key! = !"
                        set "container_required=!container_required: !key!= !"
                        set "container_required=!container_required:!key!= !"
                        set "container_required=!container_required: !key! =!"
                        set "container_required=!container_required:!key! =!"
                    ) else if "!current_section!" == "audio" (
                        set "audio_required=!audio_required: !key! = !"
                        set "audio_required=!audio_required:!key! = !"
                        set "audio_required=!audio_required: !key!= !"
                        set "audio_required=!audio_required:!key!= !"
                        set "audio_required=!audio_required: !key! =!"
                        set "audio_required=!audio_required:!key! =!"
                    )
                )
            )
        )
    )
)

REM Debug: Display remaining required keys
echo [i] Remaining required keys in [general]: !general_required! >> "%debug_file%"
echo [i] Remaining required keys in [container]: !container_required! >> "%debug_file%"
echo [i] Remaining required keys in [audio]: !audio_required! >> "%debug_file%"

REM Check for missing fields in each section
REM General section
for %%k in (!general_required!) do (
    if not "%%k" == "" (
        echo [!] Missing key in [general] section: %%k
        set /a missing_fields+=1
    )
)

REM Container section
for %%k in (!container_required!) do (
    if not "%%k" == "" (
        echo [!] Missing key in [container] section: %%k
        set /a missing_fields+=1
    )
)

REM Audio section
for %%k in (!audio_required!) do (
    if not "%%k" == "" (
        echo [!] Missing key in [audio] section: %%k
        set /a missing_fields+=1
    )
)

if !missing_fields! gtr 0 (
    echo [!] WARNING: !missing_fields! required keys are missing from your config file.
    echo [!] You should either:
    echo [!]   1. Add the missing keys to %CONFIG_PATH% (values can be empty)
    echo [!]   2. Rename or delete %CONFIG_PATH% to generate a fresh config with defaults
    exit /b 1
) else (
    echo [+] Config file validation successful! All required keys present.
    exit /b 0
)

endlocal
exit /b

endlocal
exit /b 0
