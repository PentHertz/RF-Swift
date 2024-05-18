@ECHO OFF

:: This code is part of RF Switch by @Penthertz
::  Author(s): Sébastien Dudek (@FlUxIuS)

set oldpath=%cd%

TITLE Installing RF Switch for Windows

echo [+] Compiling RF Switch Go project
cd go/rfswift
start "" "C:\Program Files\Go\bin\go.exe" "build" "."
move "rfswift.exe" %oldpath%
cd %oldpath%

echo [+] Building the Docker images
set "imagename=myrfswift:latest
set /p "imagename=Enter image tag value (default: %imagename%): "
echo %imagename%

set "dockerfile=Dockerfile
set /p "dockerfile=Enter value for Dockerfile to use (default: %dockerfile%): "
echo %dockerfile%

start "" "docker" "build" "." "-t" "%imagename%" "-f" "%dockerfile%"