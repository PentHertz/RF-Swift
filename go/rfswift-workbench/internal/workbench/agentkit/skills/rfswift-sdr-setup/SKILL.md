---
name: rfswift-sdr-setup
description: Detect, validate and troubleshoot SDR hardware in an RF Swift mission (container or Nix) before any capture or transmission. HackRF One, RTL-SDR (R820T/R828D, Blog V3/V4), Ettus USRP/UHD (B200/B210, networked), bladeRF, LimeSDR, Airspy/HF+, PlutoSDR (libiio), HydraSDR and any SoapySDR device. Covers lsusb, hackrf_info, rtl_test, uhd_find_devices, uhd_usrp_probe, uhd_images_downloader, bladeRF-cli -p, LimeUtil --find, airspy_info, iio_info -s, SoapySDRUtil --find/--probe/--rate; USB passthrough, udev, LIBUSB_ERROR_ACCESS/BUSY, hot-plug, dvb_usb_rtl28xxu grabbing RTL dongles, firmware/FPGA image mismatch, USB bandwidth and dropped samples, gain stages, PPM/clock calibration (rtl_test -p, kal) and choosing a radio (RX-only vs TX, bandwidth, frequency range).
---
# SDR hardware setup and validation

## Scope
Run this first whenever a mission uses a radio: prove the device is visible, has the right firmware,
streams without drops at the rate you need, has sane gain settings and a known frequency error.
Not covered: finding emitters (`rfswift-spectrum-survey`), recording IQ (`rfswift-iq-capture`),
decoding (`rfswift-signal-analysis`), sub-GHz devices (`rfswift-subghz-ism`), GNSS (`rfswift-gnss`),
LoRa/802.15.4 (`rfswift-iot-lpwan-mesh`), Wi-Fi adapters (`rfswift-wifi-assessment`),
Bluetooth dongles (`rfswift-bluetooth-ble`).

## Ground rules
- Everything here is receive-only unless marked TX. These radiate and are TX: `hackrf_transfer -t`
  and `-c`, `uhd_siggen`/`uhd_siggen_gui`, `SoapySDRUtil --rate ... --direction=TX`, `LimeQuickTest`
  (exercises the TX path), bladeRF-cli `tx` commands, any GNU Radio flowgraph with a hardware sink.
  In the Workbench they need "Allow RF transmission" enabled by the operator, an explicit OK in chat
  for that run, `transmits: true`, and `purpose` stating frequency, gain/power, duration and target.
  Run such tests into a 50-ohm load or through attenuators; licensed bands (cellular, GNSS, aviation)
  only conducted over coax with attenuators or inside a shielded enclosure, never over the air.
- Changing the device is the operator's decision, ask first and log it: firmware/FPGA flashing
  (`hackrf_spiflash -w`, `bladeRF-cli -f`/`-L`, `LimeUtil --update`, `uhd_image_loader`,
  `airspy_spiflash`, Pluto firmware), EEPROM writes (`rtl_eeprom -s/-w`).
- Bias tee (`hackrf_transfer -p 1`, `rtl_biast -b 1`) puts DC on the antenna port: only with an active
  antenna/LNA that expects it, never into a signal generator, attenuator chain or another radio.
- Host fixes (kernel modules, udev rules, USB forwarding) cannot be done from inside a container.
  Hand the operator the exact host command and wait.

## Tools in RF Swift
Docker: the device layer (`sdrsa_devices`) is in `sdr_light`, and therefore in `sdr_full` and
`sdr_hacks` which build on it. Nix: `sdr_light`, `sdr_full`, `sdr_hacks` carry the full device stack.
| Purpose | Commands | Docker | Nix |
|---|---|---|---|
| USB listing | `lsusb`, `lsusb -t` | every image (usbutils) | host's `lsusb` |
| HackRF | `hackrf_info`, `hackrf_transfer`, `hackrf_sweep`, `hackrf_spiflash`, `hackrf_clock` | sdr_light+ | sdr_*, bluetooth, telecom |
| RTL-SDR | `rtl_test`, `rtl_sdr`, `rtl_power`, `rtl_eeprom`, `rtl_biast` | sdr_light+ | sdr_* (rtl-sdr-osmocom) |
| USRP | `uhd_find_devices`, `uhd_usrp_probe`, `uhd_images_downloader`, `uhd_config_info`, examples `rx_samples_to_file`, `benchmark_rate` | sdr_light+ | sdr_*, telecom* |
| bladeRF | `bladeRF-cli` | sdr_light+ | sdr_*, telecom (libbladeRF) |
| LimeSDR | `LimeUtil`, `LimeSuiteGUI`, `LimeQuickTest` (TX) | sdr_light+ | sdr_* (limesuite) |
| Airspy | `airspy_info`, `airspy_rx`, `airspyhf_info`, `airspyhf_rx` | sdr_light+ | sdr_* |
| PlutoSDR | `iio_info`, `iio_attr`, `iio_readdev`, `avahicontainer_start` (container helper) | sdr_light+ | sdr_* (libiio) |
| HydraSDR | `hydrasdr_info`, `hydrasdr_rx` | sdr_light+ | sdr_* (libhydrasdr) |
| SoapySDR | `SoapySDRUtil` | sdr_light+ | sdr_*, cyberether, telecom |
| Calibration | `kal` = kalibrate-hydrasdr (HydraSDR only); kalibrate-rtl `kal` only in Nix `telecom` | sdr_light+ | sdr_*, telecom |
| GPSDO helpers | `lbe-142x`, `/rftools/calibration/lbgpsdo` (Leo Bodnar) | sdr_light+ | not shipped |
Presence check (one call):
```sh
for t in lsusb hackrf_info rtl_test uhd_find_devices bladeRF-cli LimeUtil airspy_info iio_info \
  SoapySDRUtil hydrasdr_info kal; do printf '%-18s %s\n' "$t" "$(command -v "$t" || echo MISSING)"; done
UHDEX=$(dirname "$(find /usr/libexec/uhd /usr/lib/uhd /usr/local/libexec/uhd /usr/local/lib/uhd -name rx_samples_to_file 2>/dev/null | head -1)"); echo "UHD examples: $UHDEX"
```
UHD's example programs (`rx_samples_to_file`, `benchmark_rate`) are not on `PATH`; in the RF Swift
images they sit in `/usr/libexec/uhd/examples/`. In the Workbench, call
`recommend_tools {task: "SDR hardware detection"}` first, then run everything with `execute_command`.

## Phase 1: is the device on the bus?
```sh
ls /dev/bus/usb >/dev/null && lsusb && lsusb -t     # -t: 480M = USB 2.0, 5000M or more = USB 3
```
Common IDs: HackRF One `1d50:6089` (DFU/recovery mode `1fc9:000c`), RTL2832U `0bda:2838`/`0bda:2832`,
USRP B200/B210 `2500:0020`, bladeRF 1 `1d50:6066`, bladeRF 2.0 micro `2cf0:5250`, LimeSDR-USB
`1d50:6108`, LimeSDR Mini `0403:601f`, Airspy R2/Mini `1d50:60a1`, PlutoSDR `0456:b673`,
YARD Stick One `1d50:605b`. Not in `lsusb` at all: go to Troubleshooting, row 1.
If `/dev/bus/usb` does not exist in a container, USB is not mapped into this mission.

## Phase 2: per-device identification (all short, read-only)
- HackRF: `hackrf_info`. Read `Serial number`, `Board ID`, `Firmware Version` and compare it with the
  `hackrf_info version` / `libhackrf version` lines. Firmware older than the host tools usually still
  receives, but newer features (sweep options, `-B` buffer stats, clock/`-C` ppm settings) can fail.
  Updating means `hackrf_spiflash -w <hackrf_one_usb.bin from the same release>` then a reset:
  operator OK only. A HackRF stuck at `1fc9:000c` needs DFU recovery (`dfu-util`, hardware image).
- RTL-SDR: `timeout 10 rtl_test -t` prints the dongle, tuner (`R820T`, `R828D` on Blog V4) and the
  supported gain table, then "No E4000 tuner found, aborting" (normal for R820T). `rtl_eeprom` with no
  options only reads. A Blog V4 needs a V4-aware librtlsdr; RF Swift images built with
  `RTLSDR_BASE=rtlsdrv4` use the rtl-sdr-blog driver. If a V4 is deaf or HF does not work, ask the
  operator which build the mission uses.
- USRP: `uhd_find_devices`, then `uhd_usrp_probe --args "type=b200"` (loads firmware and FPGA, prints
  the device tree, daughterboard ranges). Networked models: `uhd_find_devices --args "addr=192.168.10.2"`;
  the mission must reach that subnet (host networking) and large frames may need host MTU changes.
  Images live in `$UHD_IMAGES_DIR` (`/usr/local/share/uhd/images` in RF Swift images; check with
  `uhd_config_info --images-dir`).
- bladeRF: `bladeRF-cli -p` (probe), then `bladeRF-cli -e version -e info` (multiple `-e` run in
  order). "FPGA not loaded": RF Swift does not ship bladeRF FPGA bitstreams. The operator supplies the
  `hostedxA4/xA9/x40/x115.rbf` matching the libbladeRF version; `bladeRF-cli -l file.rbf` loads it
  into RAM only (no flash write); `-L` stores it for autoload (device write, operator OK).
- LimeSDR: `LimeUtil --find`, `SoapySDRUtil --probe="driver=lime"`. `LimeUtil --update` downloads
  and flashes gateware (operator OK, needs network).
- Airspy: `airspy_info` (R2/Mini: firmware, serial, supported sample rates), `airspyhf_info` (HF+).
- PlutoSDR: `iio_info -s` lists USB and network contexts. If it complains that Avahi is not running,
  run `avahicontainer_start` (container helper), then `iio_info -s` again. Details and firmware:
  `iio_info -u ip:192.168.2.1 | head -40` (or the `usb:` URI from the scan); look for `fw_version`.
  The network URI only works if the mission can reach the Pluto's USB-Ethernet interface.
- HydraSDR: `hydrasdr_info`.
- SoapySDR (any device with a module): `SoapySDRUtil --info` (modules loaded), `SoapySDRUtil --find`,
  `SoapySDRUtil --probe="driver=<name>"` (antennas, gain elements and ranges, sample rates,
  frequency ranges, sensors). Use the probe output as the source of truth for gain names.

## Phase 3: throughput (does it stream without drops?)
Run each for 10-15 s with `max_seconds`; nothing is written to the workspace. Every
`execute_command` is a fresh shell, so recompute `UHDEX` in the same command line when you need it.
```sh
hackrf_transfer -r /dev/null -f 433920000 -s 20000000 -n 200000000      # expect ~40 MiB/s lines
timeout 15 rtl_test -s 2400000                                           # "lost at least N bytes" = drops
SoapySDRUtil --args="driver=rtlsdr" --rate=2.4e6 --direction=RX          # never --direction=TX here
"$UHDEX/benchmark_rate" --args "type=b200" --rx_rate 20e6 --duration 10  # RX only (no --tx_rate)
```
Drops (UHD prints `O`/`D`, HackRF shows low MiB/s or "Couldn't transfer any bytes", RTL "lost at
least") come from USB 2 ports or hubs, VM forwarding (WSL usbipd, macOS Lima), CPU load, or a slow disk
when writing to a file. Find the highest clean rate and record it; captures must stay below it.

## Phase 4: gain stages
| Device | RX gain elements | Start with |
|---|---|---|
| HackRF | RF amp `-a` 0/1 (about +11 dB), LNA/IF `-l` 0-40 dB in 8 dB steps, VGA/baseband `-g` 0-62 dB in 2 dB steps | `-a 0 -l 16 -g 20` |
| RTL-SDR | one tuner gain from the table `rtl_test` prints (0.0-49.6 dB on R820T); `-g 0` = auto in rtl_sdr | 20-35 dB |
| Airspy R2/Mini | LNA 0-14, mixer 0-15, VGA 0-15 (`airspy_rx -l -m -v`) or presets linearity/sensitivity 0-21 (`-g`/`-h`) | linearity 10-15 |
| USRP B2xx | one RX gain 0-76 dB | 30-40 dB |
| bladeRF 2.0 | overall gain, range depends on frequency, manual or AGC | 20-40 dB |
| LimeSDR | LNA, TIA, PGA elements (see SoapySDR probe) | overall 30-40 dB |
| PlutoSDR | `hardwaregain` with `gain_control_mode` manual/slow_attack/fast_attack | manual 30-50 dB |
Method: raise gain until the displayed noise floor rises a few dB above the floor seen at minimum gain
(front-end noise dominates), stop there, and back off whenever the strongest in-band signal approaches
full scale or spurs appear that move or grow faster than 1 dB per dB of gain. Verify on a short test
capture with the level check in `rfswift-iq-capture`. HackRF RX input above -5 dBm can damage it:
never enable `-a 1` next to a transmitter, and never connect any TX port to an RX port without at
least 30-40 dB of attenuation.

## Phase 5: frequency error (PPM)
Offset = ppm x f / 1e6: 20 ppm is 8.7 kHz at 433.92 MHz and 31.5 kHz at 1575.42 MHz, which breaks
narrow FSK channel identification and GNSS acquisition windows. Measure, best method first:
1. Known reference: a signal generator or GPSDO output (configure Leo Bodnar units with the shipped
   helpers). Capture a few seconds near the reference and take the FFT peak (snippet in
   `rfswift-signal-analysis`); ppm = (f_measured - f_true) / f_true x 1e6.
2. GSM FCCH with kalibrate (`kal -s GSM900` to find channels, then `kal -c <ARFCN>`): the `kal` in
   the Docker images and Nix `sdr_*` is kalibrate-hydrasdr (HydraSDR only, `kal -h` shows
   "v0.5.1-hydrasdr"); kalibrate-rtl for RTL dongles exists only in Nix `telecom`. `kal -W` writes
   calibration to the HydraSDR's flash (device write, operator OK). GSM is switched off in many countries.
3. `rtl_test -p60` (RTL only; the number is the report interval in seconds): sample-clock ppm against
   the host clock. Run 10+ minutes (`max_seconds=900`, background job) and read the last "cumulative
   PPM"; only as good as the host's NTP-disciplined clock.
Apply with `rtl_sdr -p`, `rtl_433 -p`, `rtl_power -p`, `hackrf_transfer -C` (recent releases, check
`--help`), or the GUI's correction field; external 10 MHz: HackRF CLKIN, USRP `--ref external`.
Always note the measured ppm and method; captures keep the tuned frequency in their metadata.

## Choosing a device
| Device | Frequency | Max sample rate / bits | TX | Use it for |
|---|---|---|---|---|
| RTL-SDR (R820T/R828D) | about 24-1766 MHz (Blog V4 adds HF) | 2.4 Msps stable (3.2 with drops) / 8 | no | long passive monitoring, rtl_433, cheap second receiver |
| HackRF One | 1 MHz-6 GHz | 20 Msps / 8 (docs advise >= 8 Msps for best results) | half-duplex | wide sweeps, quick captures, lab replay |
| Airspy R2 / Mini | 24-1800 MHz | 10 / 6 Msps / 12 | no | weak signals near strong ones (dynamic range) |
| Airspy HF+ | HF and 60-260 MHz | under 1 MHz BW | no | HF/VHF narrowband |
| USRP B200/B210 | 70 MHz-6 GHz | 56 MHz BW (61.44 Msps) on USB 3 / 12 | full duplex, B210 2x2 | precise/timed work, GNSS with GPSDO, cellular lab |
| bladeRF 2.0 micro | 47 MHz-6 GHz | 61.44 Msps on USB 3 / 12 | full duplex 2x2 | wideband, lab TX/RX |
| bladeRF x40/x115 | 300 MHz-3.8 GHz | 40 Msps / 12 | full duplex | legacy lab setups |
| LimeSDR USB / Mini | 100 kHz (Mini 10 MHz)-3.8 GHz | 61.44 Msps (Mini lower over USB) / 12 | full duplex | lab TX/RX, cellular lab |
| PlutoSDR | 325 MHz-3.8 GHz stock | 20 MHz BW stock; USB 2 sustains only a few Msps / 12 | full duplex 1x1 | low-cost lab TX/RX |
Rules: complex sample rate at least 1.25x the occupied bandwidth plus room for offset tuning; prefer
12-bit receivers near strong emitters; RX-only devices cannot transmit by accident, so use them for
pure observation; pick a TCXO/external reference device for GNSS and narrowband FSK; a YARD Stick One
(`rfswift-subghz-ism`) is a packet radio, not an SDR.

## Troubleshooting
| Symptom | Likely cause | Action (who) |
|---|---|---|
| Not in `lsusb` inside the mission, visible on the host | USB not forwarded: Windows needs `rfswift usb attach` (usbipd), macOS needs the Lima engine plus `rfswift usb attach`; custom mission without `/dev/bus/usb` mapping | operator; a device plugged in after creation appears in a new shell, so re-run detection |
| In `lsusb`, tool says not found or `LIBUSB_ERROR_ACCESS` (-3) | permissions: rootless Podman/Nix run as the user; cgroup rule `c 189:* rwm` missing | operator on host: `rfswift host udev`, re-plug, log out/in (or `newgrp plugdev`); RF Swift maps `/dev/bus/usb` and allows `c 189:* rwm` by default |
| `usb_claim_interface error -6`, "Kernel driver is active", `LIBUSB_ERROR_BUSY` | `dvb_usb_rtl28xxu` owns the RTL dongle, or another program holds the device | check `grep dvb /proc/modules`; operator on host: `sudo rmmod dvb_usb_rtl28xxu` and blacklist it (`/etc/modprobe.d/`); close gqrx/SDR++/rtl_433 (`lsof /dev/bus/usb/*/*`) |
| `LIBUSB_ERROR_NO_DEVICE`, device disappears under load | power, cable, unpowered hub, re-enumeration | operator: direct port, short cable, USB 3 port for B2xx/bladeRF 2.0 |
| USRP worked once then not found | B2xx re-enumerates after firmware load; mission maps a single `/dev/bus/usb/BBB/DDD` | map the whole `/dev/bus/usb` (default) |
| "Could not find path for image ..." | UHD images missing | `uhd_images_downloader -t b2xx` (needs network; files.ettus.com may block automated clients), or operator copies images for `uhd_config_info --version` into `$UHD_IMAGES_DIR` |
| "Expected FPGA compatibility number X, but got Y" | flashed FPGA (N2xx/X3xx/E3xx) differs from host UHD | operator decides on `uhd_image_loader` |
| PlutoSDR not listed | Avahi not running, no route to 192.168.2.1 | `avahicontainer_start`, use the `usb:` URI |
| Overflows/drops | rate too high for the path | lower the rate, USB 3, no VM forwarding, write to a fast disk |

## Evidence
- Save outputs: `logs/sdr-setup_<UTC>.txt`, e.g.
  `{ date -u; lsusb; lsusb -t; hackrf_info; } 2>&1 | tee logs/sdr-setup_$(date -u +%Y-%m-%dT%H-%MZ).txt`.
- Mission note (`write_note`, `append: true`): device model, serial, firmware/FPGA and host library
  versions, USB speed, highest drop-free rate, gain settings chosen, measured ppm and method, antenna
  and any bias-tee use, and every device change the operator approved.
- Captures should later reference this setup entry (serial, ppm) in their SigMF `core:hw`.

## Findings
Setup problems are not target findings. Do not raise findings about drivers, firmware or the RF Swift
environment (`read_audit` covers environment CVEs and is never promoted to mission findings). If a
target-relevant fact emerges (for example the target's radio only works with a specific band plan),
note it and continue in the matching skill.

## Pitfalls
- A device can enumerate but stream garbage: always run the Phase 3 test before long captures.
- `rtl_test` without `-t` runs forever: wrap it in `timeout` or `max_seconds`.
- Two programs cannot share one SDR: stop the GUI or `rtl_433` job (`cancel_job`) before CLI captures.
- 8-bit radios (RTL-SDR, HackRF) clip early; strong nearby emitters create intermodulation that looks
  like real signals. Lower gain and see whether the "signal" drops faster than the gain change.
- HackRF and Pluto/Lime show a DC spike at the tuned frequency; tune off-center (see
  `rfswift-iq-capture`). An uncalibrated ppm shifts every frequency you report.
- In Nix missions the tools run on the host as the user: host udev rules matter, `/proc/modules` is the
  host's and nothing is containerized.
