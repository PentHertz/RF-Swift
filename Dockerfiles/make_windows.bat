@echo off

if "%1" == "common" goto common
if "%1" == "sdrlight" goto sdrlight
if "%1" == "sdrfull" goto sdrfull
if "%1" == "rfid" goto rfid
if "%1" == "wifi" goto wifi
if "%1" == "bluetooth" goto bluetooth
if "%1" == "reversing" goto reversing
if "%1" == "automotive" goto automotive
if "%1" == "build" goto build
echo "Invalid target specified"
exit /b 1

:common
docker build -f SDR/corebuild.docker -t corebuild:latest ..
if %errorlevel% neq 0 exit /b %errorlevel%
goto :eof

:sdrlight
call :common
docker build -f SDR/sdr_light.docker -t sdr_light:latest ..
if %errorlevel% neq 0 exit /b %errorlevel%
goto :eof

:sdrfull
call :common
call :sdrlight
docker build -f SDR/sdr_full.docker -t sdr_full:latest ..
if %errorlevel% neq 0 exit /b %errorlevel%
goto :eof

:rfid
call :common
docker build -f rfid.docker -t rfid:latest ..
if %errorlevel% neq 0 exit /b %errorlevel%
goto :eof

:wifi
call :common
docker build -f wifi.docker -t wifi:latest ..
if %errorlevel% neq 0 exit /b %errorlevel%
goto :eof

:bluetooth
call :common
docker build -f bluetooth.docker -t bluetooth:latest ..
if %errorlevel% neq 0 exit /b %errorlevel%
goto :eof

:reversing
call :common
docker build -f reversing.docker -t reversing:latest ..
if %errorlevel% neq 0 exit /b %errorlevel%
goto :eof

:automotive
call :common
docker build -f automotive.docker -t automotive:latest ..
if %errorlevel% neq 0 exit /b %errorlevel%
goto :eof

:build
call :sdrfull
call :bluetooth
call :wifi
call :rfid
call :reversing
echo Done!
goto :eof

