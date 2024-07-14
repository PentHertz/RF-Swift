@echo off
setlocal

if "%1" == "" goto help

REM Common build
if "%1" == "common" goto common
if "%1" == "sdrlight" goto sdrlight
if "%1" == "sdrfull1" goto sdrfull1
if "%1" == "sdrfull2" goto sdrfull2
if "%1" == "sdrfull3" goto sdrfull3
if "%1" == "sdrfull" goto sdrfull
if "%1" == "rfid" goto rfid
if "%1" == "wifi" goto wifi
if "%1" == "bluetooth" goto bluetooth
if "%1" == "reversing" goto reversing
if "%1" == "automotive" goto automotive
if "%1" == "telecom" goto telecom
if "%1" == "build" goto build

goto help

:common
docker build -f SDR/corebuild.docker -t corebuild:latest ..
goto end

:sdrlight
call :common
docker build -f SDR/sdr_light.docker -t sdr_light:latest ..
goto end

:sdrfull1
call :common
docker build -f SDR/sdr_light.docker -t sdr_light:latest ..
docker build -f SDR/sdr_full.docker --target extraoot ..
goto end

:sdrfull2
call :common
docker build -f SDR/sdr_light.docker -t sdr_light:latest ..
docker build -f SDR/sdr_full.docker --target extrasofts ..
goto end

:sdrfull3
call :common
docker build -f SDR/sdr_light.docker -t sdr_light:latest ..
docker build -f SDR/sdr_full.docker --target mldlsofts ..
goto end

:sdrfull
call :common
docker build -f SDR/sdr_light.docker -t sdr_light:latest ..
docker build -f SDR/sdr_full.docker --target sdr_full:latest ..
goto end

:rfid
call :common
docker build -f rfid.docker -t rfid:latest ..
goto end

:wifi
call :common
docker build -f wifi.docker -t wifi:latest ..
goto end

:bluetooth
call :common
docker build -f bluetooth.docker -t bluetooth:latest ..
goto end

:reversing
call :common
docker build -f reversing.docker -t reversing:latest ..
goto end

:automotive
call :common
docker build -f automotive.docker -t automotive:latest ..
goto end

:telecom
call :common
docker build -f telecom.docker -t telecom:latest ..
goto end

:build
call :bluetooth
call :wifi
call :rfid
call :reversing
call :automotive
call :telecom
call :sdrfull
echo "Build process completed!"
goto end

:help
echo Usage: %0 ^<target^>
echo Targets:
echo    common
echo    sdrlight
echo    sdrfull1
echo    sdrfull2
echo    sdrfull3
echo    sdrfull
echo    rfid
echo    wifi
echo    bluetooth
echo    reversing
echo    automotive
echo    telecom
echo    build
goto end

:end
endlocal
pause
