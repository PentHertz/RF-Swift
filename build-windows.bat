@ECHO OFF 

:: This code is part of RF Switch by @Penthertz
::  Author(s): Sébastien Dudek (@FlUxIuS)

set oldpath=%cd%

TITLE Installing RF Switch for Windows

echo "[+] Compiling RF Switch Go project"
start "" "C:\Program Files\Go\bin\go.exe build ."
move "rfswift.exe" %oldpath%


# Set default values
DEFAULT_IMAGE="myrfswift:latest"
DEFAULT_DOCKERFILE="Dockerfile"

# Prompt the user for input with default values
read -p "Enter image tag value (default: $DEFAULT_IMAGE): " imagename
read -p "Enter value for Dockerfile to use (default: $DEFAULT_DOCKERFILE): " dockerfile

# Use default values if variables are empty
imagename=${imagename:-$DEFAULT_IMAGE}
dockerfile=${dockerfile:-$DEFAULT_DOCKERFILE}

echo "[+] Building the Docker container"
sudo docker build . -t $imagename -f $dockerfile


set "imagename=myrfswift:latest
set /p "imagename=Enter image tag value (default: %imagename%): "
echo %imagename%

set "dockerfile=myrfswift:latest
set /p "dockerfile=nter value for Dockerfile to use (default: %dockerfile%): "
echo %dockerfile%


start "" "docker build . -t %imagename% -f %dockerfile%"