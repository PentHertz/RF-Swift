---
name: rfswift-hardware-interfaces
description: Board-level interfaces of an embedded/IoT target in RF Swift: locating test points and headers, voltage levels (1.8/2.5/3.3/5 V), logic analysers (sigrok-cli, PulseView, DSView, Saleae Logic 2) with UART/SPI/I2C decoders, UART baud detection and serial consoles (picocom, tio, minicom, screen, baudrate), U-Boot interruption (printenv, bdinfo, md, bootargs init=/bin/sh), JTAG/SWD with OpenOCD (ST-Link, J-Link, CMSIS-DAP, FT2232H/Tigard, Raspberry Pi GPIO, JTAGulator), STM32 RDP, nRF APPROTECT, ESP32 eFuses, SPI/I2C flash and EEPROM dumps (flashrom ch341a_spi/ft2232_spi/buspirate_spi/serprog, pyftdi), esptool, dfu-util, avrdude, st-flash, mtkclient, Glasgow, HydraBus, ChipWhisperer/PicoGlitcher glitching. Use for /dev/ttyUSB*, /dev/ttyACM*, firmware dumping, UART root shells, unlocked debug ports.
---
# Hardware interfaces: UART, U-Boot, JTAG/SWD, SPI/I2C flash, logic analysis

## Scope
- Covers: finding and identifying debug/storage interfaces on a board the client owns or has authorised, observing them passively, getting a console, reading memory and flash, assessing debug-port protection, and (advanced, gated) fault injection.
- Not covered: analysing a dumped image (`rfswift-firmware-analysis`), CAN/OBD buses (`rfswift-automotive-can`), cellular modules and their diag ports (`rfswift-cellular-lab`), sub-GHz radios on the board (`rfswift-subghz-ism`).
- Nothing in this skill radiates. If a step makes the target's own radio transmit (flashing a test firmware, enabling a transmitter from a console), the RF transmission rule applies: the operator enables "Allow RF transmission", you set `transmits: true` with frequency, power, duration and target in `purpose`, and you get an explicit OK in chat for that run.

## Ground rules
- The operator does all physical work: opening the enclosure, photos of both board sides, chip markings, multimeter measurements, soldering, clips, wiring, power cycling. Ask for what you need (photo, voltage, which pin went where); never assume a wire is connected or a board is powered.
- Voltage first. Ask the operator to measure the candidate pins and the rail (power on, referenced to GND) before anything is connected. The adapter I/O level must match the target: a 3.3 V adapter driving a 1.8 V SoC pin can damage it, 5 V into a 3.3 V adapter input can damage the adapter, RS-232 levels (+/-12 V on old equipment) need a converter. Never connect the adapter VCC to the target "to be safe". Connect GND first, then target TX to adapter RX only; connect adapter TX only when you need to type.
- Free (inside the mission scope): logic captures, listening on UART RX, reading boot banners, reading the U-Boot environment, JTAG/SWD ID reads on a known pinout, read-only probes (`flashrom` probe, `st-info --probe`, `espefuse.py summary`, `dfu-util -l`).
- Explicit operator OK in chat, per action, before:
  - typing anything on a Linux console beyond Enter (login attempts, commands on a production device), or interrupting a boot on a device that controls something physical;
  - halting or resetting a CPU over JTAG/SWD (`halt`, `reset halt`), or JTAGulator/Glasgow pin scans (they drive unknown pins);
  - changing U-Boot state: `setenv`, booting with modified `bootargs`, `saveenv` (persistent), `env default`, `run` of vendor scripts;
  - any write or erase: `flashrom -w/-E`, OpenOCD `flash write_image`/`program`/`flash erase_*`, `st-flash write`, `esptool write_flash`/`erase_flash`, `dfu-util -D`, `avrdude -U ...:w:`, mtkclient writes, U-Boot `sf erase/write`, `nand erase/write`, `mmc write`, `mw`;
  - readout-protection changes: STM32 RDP regression/`unlock` (mass erase), `nrf5 mass_erase`, any `lock`; and every fault-injection or side-channel campaign.
- Irreversible operations (`espefuse.py burn_*`, STM32 RDP level 2, OTP writes, enabling secure boot): never propose them. If the operator asks, restate that they are permanent and require an OK quoting the exact command.

## When to stop and ask the operator
- You need a physical action: (re)wiring, a photo, a voltage, a power-cycle, holding a button (boot mode, reset), removing a chip.
- The measured level does not match the adapter, or nobody measured it yet.
- The next command is in the OK list above, or its effect on the device is unclear to you.
- The serial port or USB adapter is not visible inside the mission.
- Two reads of the same memory differ, or the target starts behaving differently (reboot loops, new error messages).

## Tools in RF Swift
- Docker image `hardware` (profile `hardware`: host network, `SYS_RAWIO`, `/dev/bus/usb`):
  - serial: picocom, minicom, tio, screen, `baudrate` (RF Swift helper);
  - logic/scope: PulseView, DSView, Logic 2 (`Logic`, x86_64), `slogic-pulseview`, ngscopeclient, `dsl2sigrok`;
  - debug/flash: OpenOCD (git build with ftdi, stlink, jlink, cmsis-dap, bcm2835gpio, linuxgpiod, remote-bitbang), flashrom, avrdude, dfu-util, `esptool.py`/`espefuse.py`/`espsecure.py`, openFPGALoader, mtkclient (`mtk`), SeerGDB;
  - bus adapters: pyftdi (FT232H/FT2232H, Tigard), pyHydrabus, Glasgow (`glasgow`);
  - fault injection and others: ChipWhisperer and `findus` (PicoGlitcher) Python modules, `rd6006` (bench PSU), SPITkey (`/hardware/SPITkey`) with the `spi-tpm` sigrok decoder.
- Nix environment `hardware`:
  - sigrok-cli, pulseview, dsview, saleae-logic2, scopehal-apps;
  - openocd, flashrom, avrdude, dfu-util, dfu-programmer, esptool, openfpgaloader, stlink (`st-info`, `st-flash`), urjtag (`jtag`), gcc-arm-embedded (`arm-none-eabi-gdb`), platformio, arduino-cli, mtkclient;
  - picocom, minicom, tio, baudrate, pyftdi, glasgow, chipwhisperer.
- `sigrok-cli` is certain in Nix `hardware`; in the Docker image check `command -v sigrok-cli` and fall back to PulseView/DSView (operator) if absent.
- Not shipped by RF Swift: pyOCD, probe-rs, i2c-tools (`i2cdetect`), a Bus Pirate client (use picocom or flashrom/OpenOCD drivers), gdb-multiarch, telnet/nc in the `hardware` image (they are in the `network` image). JTAGulator needs no host software: it is driven over its own USB serial menu with picocom.
- Check before starting (`recommend_tools {task: "SPI flash dump"}` suggests environments when something is missing), and run everything through `execute_command` (timed or background jobs) or the session tools:
```sh
for t in sigrok-cli pulseview picocom tio minicom screen baudrate openocd flashrom esptool.py esptool espefuse.py dfu-util avrdude st-info glasgow mtk; do printf '%-12s %s\n' "$t" "$(command -v $t || echo -)"; done
python3 -c 'import pyftdi, chipwhisperer; print("pyftdi+chipwhisperer ok")'
ls -l /dev/ttyUSB* /dev/ttyACM* /dev/serial/by-id/ 2>/dev/null; lsusb
```
- Serial ports in RF Swift containers are attached on demand: the container may open ttyACM/ttyUSB/ttyS nodes, and RF Swift creates the node inside the container when it starts and each time a shell or command is opened. After the operator plugs an adapter in, just run a new command. If it still does not appear, ask the operator: rootless Podman needs the port mapped at mission creation; on macOS/Windows the device must be forwarded with the Workbench "USB passthrough"; hot-plug can be switched off per container. Nix missions run on the host: the user needs serial access (dialout/uucp group or the RF Swift udev rules).
- Prefer `/dev/serial/by-id/...` paths. An FT2232H (Tigard) exposes two ttyUSB nodes; when OpenOCD, flashrom or pyftdi claim it through libusb those nodes disappear until it is released.

## Workflow (passive first)
### Phase 0: recon (operator + agent)
- From photos and markings, list: SoC/MCU, external flash (SOIC-8 SPI NOR such as W25Q/MX25, WSON, NAND, eMMC BGA), EEPROMs (24Cxx, 93Cxx), unpopulated headers (4-pin rows: UART; 2x5 1.27 mm: Cortex SWD/JTAG; 2x10 2.54 mm: ARM JTAG; 14-pin: TI/MIPS EJTAG), test pads near the SoC.
- Multimeter (operator): power off, continuity to GND; power on, VCC steady, TX toggling during boot then idling high, RX steady high (pull-up). Write header id, pin numbers, voltages and photo names to the note.

### Phase 1: logic analyser (passive)
```sh
sigrok-cli --scan
sigrok-cli -d fx2lafw --show                     # channels (D0..D7), sample rates, options
sigrok-cli -d fx2lafw -C D0,D1 -c samplerate=4m --time 20s -o captures/uart_J3_D0D1_4MHz_2026-11-24T10-05Z.sr
sigrok-cli -i captures/uart_J3_D0D1_4MHz_2026-11-24T10-05Z.sr -P uart:rx=D0:baudrate=115200 -A uart=rx-data
sigrok-cli -i captures/<file>.sr -P uart:rx=D0:baudrate=115200 -B uart=rx > logs/uart_J3_rx_2026-11-24T10-05Z.bin
sigrok-cli -i captures/<file>.sr -P spi:clk=D0:mosi=D1:miso=D2:cs=D3,spiflash -A spiflash
sigrok-cli -i captures/<file>.sr -P i2c:scl=D0:sda=D1 -A i2c=address-read:address-write:data-read:data-write
```
- Driver names: `fx2lafw` (Saleae-style clones), `dreamsourcelab-dslogic`; channel names differ per driver, read `--show`. Sample at 4x the fastest signal at least (8-10x is better). Start the capture, then ask the operator to power-cycle the target; use `max_seconds` for timed captures.
- Baud: the narrowest pulse width is one bit; baud ~ 1/width. Common rates: 115200, 57600, 38400, 9600, 921600, 1500000 (Rockchip), 74880 (ESP8266 ROM). PulseView/DSView/Logic 2 are for the operator; `dsl2sigrok` converts DSView files for sigrok-cli.

### Phase 2: UART console
- Rate scan while the operator power-cycles: `baudrate /dev/ttyUSB0 -t 2` (cycles 115200..921600; lines prefixed `ok` are mostly printable). Run it with `max_seconds: 60`.
- Open the console as an interactive session and log it:
  - `session_start {name: "uart", command: "picocom -b 115200 --logfile logs/uart_J3_115200_2026-11-24T10-05Z.log /dev/ttyUSB0", purpose: "UART console J3, read-only"}`
  - read with `session_send {name: "uart", input: "", enter: false, wait_seconds: 10}`; exit picocom with input `"\u0001\u0018"` (Ctrl-A Ctrl-X), `enter: false`.
  - Alternatives: `tio -b 115200 --log --log-file logs/<f>.log /dev/ttyUSB0` (quit Ctrl-T q; log flags differ between tio versions, check `tio --help`), `minicom -D /dev/ttyUSB0 -b 115200 -C logs/<f>.log` (Ctrl-A X), `screen -L -Logfile logs/<f>.log /dev/ttyUSB0 115200` (Ctrl-A k; Docker images). Without MCP: `tmux new -s uart 'picocom ...'`.
- Look for: bootloader banner and version, SoC, DRAM, flash type and partition layout (`mtdparts=`), kernel command line (`console=`, `root=`, `init=`), init messages, and how the console ends: shell prompt without login (`/ #`, `root@...#`), login prompt, vendor CLI or silence.
- A login prompt is not an invitation to guess. Credentials found during firmware analysis or vendor documentation can be tried only with an OK (each attempt is input to the device; lockouts exist).

### Phase 3: U-Boot
- Interrupting autoboot changes nothing persistent but stops the normal boot: confirm the operator agrees, then ask for a power-cycle and send keys during the countdown (`Hit any key to stop autoboot`, ESC, Ctrl-C `"\u0003"`, or a vendor string). With `bootdelay=0`, repeat `session_send {input: " ", enter: false, wait_seconds: 1, expect: "=>|U-Boot>|# $"}` from power-on.
- Read-only commands: `version`, `help` (vendors strip commands), `printenv` (save it first), `bdinfo`, `mtdparts` or `mtd list`, `sf probe 0`, `nand info`, `mmc list`, `mmc info`, `iminfo <addr>`, `md.b <addr> <len>`. Note: `md` alone prints 32-bit words; use `md.b` for byte dumps.
- In `printenv`, record: `bootcmd`, `bootargs`, `bootdelay`, `verify` (`n` skips image checksum checks in `bootm`), `ipaddr`/`serverip` (netboot use), `ethaddr`, `mtdparts`, vendor secure-boot or "silent" variables, any credential-looking value (save as secret).
- Flash over UART when no other path exists: `sf probe 0; sf read <ram_addr> 0x0 0x100000; md.b <ram_addr> 0x100000`, with picocom logging. `sf read` only copies flash into RAM; take `<ram_addr>` from `printenv loadaddr` or the DRAM range in `bdinfo`, away from U-Boot's relocation address. At 115200 baud this moves about 3-4 KB/s of data (16 MB takes over an hour); prefer a direct SPI read. Convert the log back to binary:
```sh
python3 -I -c 'import re,sys
out=open(sys.argv[2],"wb")
for l in open(sys.argv[1],errors="ignore"):
    m=re.match(r"^[0-9a-fA-F]{8,16}:((?: [0-9a-fA-F]{2}){1,16})",l.strip())
    if m: out.write(bytes.fromhex(m.group(1).replace(" ","")))' logs/uboot_md_<f>.log dumps/uboot_md_0x0_1M_2026-11-24T10-05Z.bin
```
- Boot to a root shell (explicit OK; RAM-only until `saveenv`): `setenv bootargs "${bootargs} init=/bin/sh"` then `boot` (or `run bootcmd`). Some boards rebuild `bootargs` inside `bootcmd` and overwrite the edit; then inspect `bootcmd` and change the variable it uses. The shell runs without init; the rootfs is often read-only, and `mount -o remount,rw /` is a further modification (OK). Never `saveenv`, `env default -a`, or any write/erase command without OK. Restore by power-cycling and confirm the normal boot in the log.

### Phase 4: JTAG/SWD
- Pinout: SWD = SWDIO, SWCLK, GND, VTref (+ nRESET); JTAG = TCK, TMS, TDI, TDO (+ TRST, nSRST), VTref. Unknown pads: JTAGulator (operator wires it; its serial menu at 115200 via picocom: set the target voltage first with `V` to the measured value, then `J` for JTAG with IDCODE `I` / BYPASS `B` scans, `U` for UART) or Glasgow (`glasgow run --help` lists applets such as a JTAG pinout finder; set `-V` to the target voltage). Pin scans drive the target: explicit OK.
- Config files: `ls "$(dirname "$(readlink -f "$(command -v openocd)")")/../share/openocd/scripts/"{interface,target}`. Typical: `interface/stlink.cfg`, `interface/jlink.cfg`, `interface/cmsis-dap.cfg`, `interface/ftdi/<board>.cfg` (FT2232H boards; for a Tigard use the Tigard project's cfg or an ftdi cfg matching its layout), Raspberry Pi GPIO cfgs (`raspberrypi-native.cfg` / `raspberrypi-gpio-connector.cfg` depending on the OpenOCD version, only when the RF Swift host is the Pi).
- Chain probe on JTAG (no target cfg: OpenOCD auto-creates taps and prints IDCODEs):
```sh
openocd -f interface/jlink.cfg -c "transport select jtag" -c "adapter speed 500" -c "init" -c "scan_chain" -c "shutdown" 2>&1 | tee logs/openocd_scan_2026-11-24T10-05Z.log
```
- Prefer batch mode (non-interactive, reproducible). Reading flash with a halted core (`halt` needs OK on a device that drives hardware):
```sh
openocd -f interface/stlink.cfg -f target/stm32f1x.cfg -c "init" -c "halt" -c "flash banks" \
  -c "flash read_bank 0 dumps/stm32f103_flash_2026-11-24T10-05Z.bin" -c "dump_image dumps/stm32f103_sram_2026-11-24T10-05Z.bin 0x20000000 0x5000" \
  -c "resume" -c "shutdown" 2>&1 | tee logs/openocd_read_2026-11-24T10-05Z.log
```
  `mdw 0x08000000 8` peeks memory. Interactive use: run `openocd ...` as a background job (`execute_command`, returns `job_id`), then `session_start {name: "ocd", command: "telnet localhost 4444"}` only if `telnet` or `nc` exists; GDB on port 3333 needs a cross debugger (`arm-none-eabi-gdb` in Nix `hardware`).
- Protection checks:
  - STM32 RDP: level 0 open; level 1 blocks debugger access to flash (reads fail or return zeros while SRAM/peripherals may still read) and going back to 0 mass-erases; level 2 disables debug permanently. Read option bytes with the family command (`help stm32f1x`, `stm32f1x options_read 0`, `stm32f2x options_read 0`, other families differ). Never `stm32*x unlock`, `lock`, `mass_erase`, `flash erase_*`, `program` without OK.
  - nRF52/nRF53: APPROTECT; `nrf5 mass_erase` erases the chip (OK).
  - ESP32: `espefuse.py --port /dev/ttyUSB0 summary` (read-only) shows JTAG disable, secure boot and flash-encryption eFuses. Flash read (bootloader mode: GPIO0 low at reset, done by the operator or the board's auto-reset): `esptool.py --port /dev/ttyUSB0 flash_id` then `esptool.py --port /dev/ttyUSB0 read_flash 0 0x400000 dumps/esp32_flash_4MB_2026-11-24T10-05Z.bin` (esptool v5 renamed commands to `read-flash` etc.; check `--help`). With flash encryption on, the dump is ciphertext.
- Other read paths: `st-info --probe` and `st-flash read dumps/<f>.bin 0x08000000 0x20000` (Nix); `dfu-util -l`, then `dfu-util -a 0 -U dumps/<f>.bin` (UPLOAD = device to host, read) whereas `-D` (DOWNLOAD) writes; `avrdude -c usbasp -p m328p -U flash:r:dumps/<f>.hex:i` reads, `:w:` writes; mtkclient (`mtk printgpt`, `mtk rl dumps/mtk/`) uses BootROM exploits: active, OK first.

### Phase 5: SPI/I2C flash and EEPROM
- In-circuit (SOIC clip) or desoldered is the operator's call. In-circuit, the programmer's VCC back-powers the board and the SoC may boot and fight the bus; the operator can hold the SoC in reset or remove the chip. Check the chip voltage in its datasheet (1.8 V parts exist, e.g. W25Q..FW); many CH341A "black" boards drive 5 V on data lines and need a 3.3 V fix or level shifter.
```sh
flashrom -p ch341a_spi                                    # probe only: prints the chip name(s)
flashrom -p ch341a_spi -c "W25Q128.V" -r dumps/spi_U5_W25Q128_read1_2026-11-24T10-05Z.bin -o logs/flashrom_read1_2026-11-24T10-05Z.log
flashrom -p ch341a_spi -c "W25Q128.V" -r dumps/spi_U5_W25Q128_read2_2026-11-24T10-05Z.bin -o logs/flashrom_read2_2026-11-24T10-05Z.log
sha256sum dumps/spi_U5_W25Q128_read*.bin | tee -a dumps/SHA256SUMS
```
  - Use `-c` only when the probe lists several matching definitions; copy the exact name. Reads take minutes: background job with `wait_seconds` and `job_output`.
  - Other programmers: `ft2232_spi:type=2232H,port=B,divisor=4` (FT2232H/Tigard SPI on channel B), `buspirate_spi:dev=/dev/ttyUSB0,spispeed=1M`, `serprog:dev=/dev/ttyACM0:115200` (Pico/STM32 serprog), `linux_spi:dev=/dev/spidev0.0,spispeed=2000`. `flashrom -L` lists supported chips/programmers; parameter syntax changes between versions, check `flashrom --help`.
  - Different hashes: reseat the clip, lower the speed (`divisor=`/`spispeed=`), check power, read again; never treat an unstable dump as authoritative.
  - `-w`/`-E` (write/erase): explicit OK, two identical verified backups recorded in `dumps/SHA256SUMS`, and the operator's acceptance of the brick risk.
- I2C EEPROM (24Cxx): with an FT232H/FT2232H (Tigard), `ftdi_urls.py` lists device URLs and `i2cscan.py ftdi://ftdi:2232h/2` scans (pyftdi scripts; check `command -v`). Read a small 1-byte-address part (24C02; 2-byte addresses from 24C32 up):
```sh
python3 -I -c 'from pyftdi.i2c import I2cController; c=I2cController(); c.configure("ftdi://ftdi:2232h/2", frequency=100000)
p=c.get_port(0x50); open("dumps/eeprom_U7_24C02_2026-11-24T10-05Z.bin","wb").write(p.exchange([0x00], 256)); c.terminate()'
```
  Bus Pirate (I2C mode and scan macro through its menu in picocom), HydraBus (`pyHydrabus`) and Glasgow I2C/25x/24x memory applets are alternatives.
- eMMC (operator removes it or uses ISP pads with a reader): image the block device read-only, double-check the device name with `lsblk` first: `dd if=/dev/sdX of=dumps/emmc_<f>.img bs=4M status=progress`. Raw NAND needs dedicated programmers and OOB handling (out of scope for flashrom).
- TPM sniffing (BitLocker VMK on discrete SPI TPMs): passive logic capture with the `spi-tpm` decoder, then SPITkey (`/hardware/SPITkey`, Docker `hardware`).

### Phase 6: fault injection and side channel (advanced lab work)
- ChipWhisperer (`import chipwhisperer as cw`), PicoGlitcher (`findus`), bench PSU control (`rd6006`). Only with an explicit OK per campaign, a board the client accepts losing, and the operator building and supervising the setup. Characterise normal behaviour and a trigger first (UART output, GPIO), log every parameter set and outcome to `logs/glitch_<target>_2026-11-24T10-05Z.csv`, never run unattended without OK.

## Reading results
- Garbled console: wrong baud, wrong level, missing common ground, or inverted/RS-232 signalling. Boot noise then silence: console disabled after the bootloader (`quiet`, removed `console=`); U-Boot may still answer.
- U-Boot ignores keys: adapter TX not wired, keyed autoboot (`CONFIG_AUTOBOOT_KEYED`), or input disabled. Call it "not interruptible" only after verifying wiring.
- OpenOCD "scan chain interrogation failed: all ones/zeroes": wrong pins, no VTref, target unpowered or wrong speed (start at 100-500 kHz). A valid IDCODE/DPIDR with "Failed to read memory": readout protection likely; confirm with option bytes.
- IDCODE decoding: bit 0 is always 1, bits 11:1 are the JEDEC manufacturer, bits 27:12 the part, bits 31:28 the version. Values ending in `477` are ARM debug ports (`0x4BA00477` JTAG-DP of Cortex-M3/M4 class parts; SWD DPIDRs such as `0x2BA01477`, `0x0BB11477`, `0x0BC11477`); vendor boundary-scan TAPs appear next to them on the chain. Write every IDCODE to the note; do not guess the exact chip from it, ask the operator to match it with the marking.
- A responding debug port is not yet "unprotected": the finding needs a successful read of protected content (flash, keys), not only an ID.
- flashrom "No EEPROM/flash device found": wiring, power, SoC holding the bus, wrong voltage. All 0xFF: wrong CS, blank chip, or reading while held in deep power-down. High entropy everywhere in an ESP32 dump: flash encryption (confirm with `espefuse.py summary`).

## Evidence
- `dumps/`: `spi_<ref>_<chip>_read{1,2}_<UTC>.bin`, `stm32f103_flash_<UTC>.bin`, `esp32_flash_4MB_<UTC>.bin`, `uboot_md_<offset>_<len>_<UTC>.bin`, `eeprom_<ref>_<chip>_<UTC>.bin`, and `dumps/SHA256SUMS` (`sha256sum dumps/*.bin >> dumps/SHA256SUMS`). UTC stamp: `date -u +%Y-%m-%dT%H-%MZ`.
- `logs/`: console logs `uart_<header>_<baud>_<UTC>.log`, `uboot_printenv_<UTC>.txt`, `openocd_<action>_<UTC>.log`, `flashrom_<action>_<UTC>.log`. `captures/`: logic captures `.sr` named with header, channels and sample rate.
- Photos are taken and registered by the operator; reference their file names.
- Mission note (`write_note`, `append: true`): header and pin map with measured voltages, adapter and level, exact commands, start/end times, hashes, anything modified (env, boot, protection) and how it was restored.
- Secrets seen on a console or in U-Boot env (passwords, Wi-Fi PSKs, tokens, keys): `save_secret {label, kind, value, source: "logs/uart_J3_115200_<UTC>.log, line 214"}`; elsewhere write only "stored as secret <label>".

## Typical findings
- "Unauthenticated root shell on UART console (header J3)". Confirmed: console log showing the shell without credentials and the output of a read-only `id` (uid=0). Candidate: shell prompt seen but not exercised. `CVSS:3.1/AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H` (6.8, medium). Remediation: disable or authenticate the production console, no shell on `inittab` respawn.
- "Interruptible U-Boot with writable boot arguments". Confirmed: log of the prompt and `printenv`; with OK, `init=/bin/sh` reaching a root shell. Same vector. Remediation: `bootdelay=-2`/keyed autoboot with a strong secret, environment not modifiable (fixed or signed in FIT), verified boot.
- "JTAG/SWD debug access enabled in production". Confirmed: IDCODE/DPIDR plus a successful memory or flash read (hash, recognisable content). Candidate: port answers, readout not attempted. `AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H` (6.8). Remediation: RDP level 2/APPROTECT/JTAG-disable eFuse, authenticated debug.
- "Readout protection not enabled (STM32 RDP level 0)". Confirmed: option-byte read showing RDP 0xAA and a flash dump.
- "External flash readable in clear". Confirmed: two identical dumps, content identified. `AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N` (4.6). If extracted secrets are fleet-wide or grant remote access, report that consequence as its own finding (often AV:N). Remediation: flash encryption bound to the SoC, per-device secrets, no credentials in flash.
- Save with `save_finding` (stored as draft; the operator confirms). Include `observation` with commands and file names, `poc` with exact steps, `cvss` vector, `remediation`.

## Pitfalls
- Two programs on one adapter (picocom on ttyUSB0 while OpenOCD/flashrom claims the FTDI): "busy" errors and vanishing nodes. Close sessions first.
- Back-powering through the adapter TX line half-boots an unpowered target; connect TX last and remove it when idle.
- Long jumper wires at high SWD/JTAG/SPI speeds corrupt data; reduce speed before blaming protection.
- Ground loops between a mains-powered target and a laptop on its charger; use a common ground and the operator's isolation judgement.
- In-circuit SPI reads that differ between runs; a resetting watchdog during long reads.
- The container may not see a freshly plugged adapter until a new command runs; on macOS/Windows it must be forwarded with "USB passthrough".
