@echo off
rem Start Menu -> "RF Swift Console": opens an interactive prompt with the
rem rfswift CLI already on PATH (the MSI adds %ProgramFiles%\RF Swift to it).
title RF Swift Console
echo.
echo   RF Swift console
echo   The 'rfswift' command is on your PATH. Examples:
echo.
echo       rfswift --help
echo       rfswift doctor
echo       rfswift usb attach
echo       rfswift run -i sdr_light -n lab
echo       rfswift run --engine nix -i sdr_light -n lab    (Nix engine, inside WSL 2)
echo       rfswift nix wsl status
echo.
cd /d "%USERPROFILE%"
cmd /k
