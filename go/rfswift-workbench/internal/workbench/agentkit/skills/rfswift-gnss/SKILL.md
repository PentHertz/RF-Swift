---
name: rfswift-gnss
description: GNSS (GPS, Galileo, GLONASS, BeiDou) receive-side analysis from an RF Swift mission and strictly conducted lab simulation. L1/E1 1575.42 MHz spectrum checks with hackrf_sweep, recording L1 with HackRF, RTL-SDR, USRP or bladeRF at suitable rates, bias tee for active antennas, ppm versus Doppler search, gnss-sdr with a File_Signal_Source config (ibyte/ishort/gr_complex), Galileo OSNMA, jamming and spoofing indicators (noise floor, chirps, uniform C/N0, position and time jumps), u-blox receivers over serial with ubxtool (MON-HW, MON-RF, MON-SPAN, NAV-STATUS spoofDetState, NAV-SAT, SEC-SIG), gpsmon, gpsd and NMEA, and GNSS simulation with gps-sdr-sim plus hackrf_transfer -t only into coax with attenuators or a shielded enclosure, never over the air, with explicit operator OK. Use for GPS spoofing, GNSS jamming, timing receivers, trackers, drones, vehicle telematics.
---
# GNSS analysis and lab simulation

## Scope
Receive and analyse GNSS signals around a target (tracker, drone, telematics unit, timing receiver),
interrogate its GNSS receiver over serial, detect jamming/spoofing indicators, and test how the target
behaves under simulated GNSS in a conducted or shielded lab setup.
Not covered: radio setup and ppm (`rfswift-sdr-setup`), generic capture rules (`rfswift-iq-capture`),
cellular parts of telematics units (`rfswift-cellular-lab` if present), serial/debug interfaces
beyond the GNSS port (`rfswift-hardware-interfaces`).

## Ground rules
- Receiving GNSS is free. Transmitting anything in a GNSS band (1164-1300 MHz and 1559-1610 MHz) is
  illegal almost everywhere without a specific licence and endangers aviation, maritime, timing and
  emergency services. GNSS TX is therefore only ever conducted (coax, DC block, attenuators straight
  into the target's RF input) or inside an RF-shielded enclosure, never over the air, not even "at
  minimum power".
- Each simulation run needs: the operator's "Allow RF transmission", an explicit OK in chat for that
  run naming the setup (conducted path or enclosure), `transmits: true`, and `purpose` with frequency,
  gain, attenuation, duration and target, e.g. "TX gps-sdr-sim static scenario 1575.42 MHz, HackRF
  -x 0 -a 0, conducted via DC block + 70 dB into tracker RF input in shielded box, 120 s". If any
  element is missing, stop and ask.
- No jamming of any kind, conducted noise injection included, unless the operator explicitly designs
  and authorises a shielded lab test; GNSS loss is tested by disconnecting the antenna instead.
- Use lab units: simulated time and position can corrupt device state (stored almanac, clocks,
  certificate validity, logs, geofences). Agree a restore procedure (cold start, power cycle).
- Receiver configuration writes (ubxtool `-e`, `-d`, `-z`, `-x`, `-S`, `-c`, saving to flash) change
  the target: operator OK first. Polls (`-p`) and passive reading are fine.

## Tools in RF Swift
| Tool | Role | Docker image | Nix env |
|---|---|---|---|
| `gnss-sdr` (0.0.22 in current images), `front-end-cal` | software receiver; example configs in `/usr/local/share/gnss-sdr/conf/` | sdr_light, sdr_full, sdr_hacks | sdr_light, sdr_full, sdr_hacks |
| `gps-sdr-sim`, `gps-sdr-sim-uhd.py` | GPS L1 C/A baseband generator (TX when played) | sdr_full | sdr_full |
| `hackrf_sweep`, `hackrf_transfer`, `rtl_sdr`, `rtl_biast`, UHD examples, `bladeRF-cli` | survey, record, play | sdr_light+ | sdr_* |
| `ubxtool`, `gpsmon`, `cgps`, `gpspipe`, `gpsd` | u-blox/NMEA receivers | sdr_light+ (gpsd packages) | not shipped |
| `screen` / `picocom`, `minicom`, `tio` | raw serial | screen every image; picocom/tio in sdr_hacks, hardware | picocom/tio in hardware, sdr_hacks |
| `gnsslogger`, Leo Bodnar GPSDO helpers | GNSS logging/calibration, reference clocks | sdr_light+ | not shipped |
| `inspectrum`, scripts of `rfswift-spectrum-survey`/`rfswift-iq-capture` | spectrogram, heatmap, level checks | sdr_light+ | sdr_* |
Not shipped: RTKLIB command-line tools (`rtkrcv`, `rnx2rtkp`, `convbin`; gnss-sdr embeds RTKLIB for
its PVT), u-center, pyubx2/pygnssutils. Galileo/GLONASS/BeiDou simulators are not shipped
(gps-sdr-sim is GPS L1 C/A only).

## Phase 1: is L1 clean? (passive)
```sh
TS=$(date -u +%Y-%m-%dT%H-%MZ)
hackrf_sweep -f 1555:1615 -w 20000 -a 1 -l 32 -g 20 -p 1 -N 60 -r logs/sweep_1555-1615M_20k_${TS}.csv
python3 scripts/sweep_heatmap.py logs/sweep_1555-1615M_20k_${TS}.csv logs/sweep_1555-1615M_20k_${TS}.png
```
`-p 1` powers an active antenna (3.3 V); omit it for passive antennas or anything without a DC block.
Real GNSS signals are about 20 dB below the thermal noise floor, so a clean band looks flat. Any
carrier, sweeping line, pulsed energy or raised floor in 1559-1610 MHz is interference (personal
"privacy" jammers sweep, repeaters and spoofers can show as structure). Compare with a reference
place or time; adjacent licensed services (for example 1452-1492 MHz downlink, Inmarsat 1525-1559 MHz)
are not interference.

## Phase 2: record
| Signal | Centre | Minimum complex rate | Notes |
|---|---|---|---|
| GPS L1 C/A | 1575.42 MHz | 2 Msps (4 recommended) | main lobe 2.046 MHz |
| Galileo E1 (BOC(1,1)), BeiDou B1C | 1575.42 MHz | 4 Msps (8 better) | |
| BeiDou B1I | 1561.098 MHz | 4 Msps | separate capture or 32 Msps around both |
| GLONASS L1 (FDMA) | about 1602 MHz | 10 Msps | |
| GPS L5 / Galileo E5a | 1176.45 MHz | 20+ Msps | USRP/bladeRF; HackRF is marginal |
```sh
# HackRF, 60 s GPS/Galileo L1 at 4 Msps (480 MB). With a high-gain active antenna start with -a 0.
hackrf_transfer -r captures/1575.42M_4Msps_${TS}_l1-site.cs8 -f 1575420000 -s 4000000 -b 3500000 \
  -a 1 -l 32 -g 20 -p 1 -n 240000000
# RTL-SDR with TCXO (Blog V3/V4): bias tee first, 2.048 Msps, 60 s
rtl_biast -b 1; rtl_sdr -f 1575.42M -s 2.048M -g 40 -n 122880000 captures/1575.42M_2.048Msps_${TS}_l1.cu8
# USRP: sc16, LO offset keeps the DC spike off the signal
/usr/libexec/uhd/examples/rx_samples_to_file --args "type=b200" --freq 1575.42e6 --lo-offset 2e6 \
  --rate 4e6 --gain 50 --nsamps 240000000 --type short --file captures/1575.42M_4Msps_${TS}_l1.cs16
```
Run as a background job with `max_seconds` about duration + 20. Check levels with `iqcheck.py`
(`rfswift-iq-capture`): it is all noise, so aim for an RMS around -25 to -15 dBFS on 8-bit radios with
no clipping. Frequency error matters: 20 ppm is 31.5 kHz at L1, far beyond a +/-5 kHz Doppler search.
Measure the ppm (`rfswift-sdr-setup`) and either correct the centre or widen
`Acquisition_1C.doppler_max` to cover ppm offset + 5 kHz. Some RTL drivers reset the bias GPIO when a
program opens the device: if nothing is acquired, check the antenna is powered. HackRF puts its DC spike
in the middle of L1; gnss-sdr usually copes, and the shipped HackRF examples show the
`Freq_Xlating_Fir_Filter` + IF approach for offset tuning. Write SigMF metadata and checksums as in
`rfswift-iq-capture`; record at least 60 s (ephemeris needs 30 s or more), better 120-300 s.

## Phase 3: process with gnss-sdr
Start from the shipped examples (`/usr/local/share/gnss-sdr/conf/File_input/GPS/` in Docker images,
`find / -path '*gnss-sdr/conf*' -name '*.conf'` in Nix). Minimal GPS L1 C/A file config, tested with
gnss-sdr 0.0.22 (save as `logs/gnss/gps_l1_file.conf`):
```ini
[GNSS-SDR]
GNSS-SDR.internal_fs_sps=4000000
SignalSource.implementation=File_Signal_Source
SignalSource.filename=captures/1575.42M_4Msps_2026-11-24T10-05Z_l1-site.cs8
SignalSource.item_type=ibyte
SignalSource.sampling_frequency=4000000
SignalSource.samples=0
SignalSource.repeat=false
SignalSource.enable_throttle_control=false
SignalConditioner.implementation=Signal_Conditioner
DataTypeAdapter.implementation=Ibyte_To_Complex
InputFilter.implementation=Pass_Through
InputFilter.input_item_type=gr_complex
InputFilter.output_item_type=gr_complex
Resampler.implementation=Pass_Through
Resampler.item_type=gr_complex
Channels_1C.count=8
Channels.in_acquisition=1
Channel.signal=1C
Acquisition_1C.implementation=GPS_L1_CA_PCPS_Acquisition
Acquisition_1C.item_type=gr_complex
Acquisition_1C.coherent_integration_time_ms=1
Acquisition_1C.pfa=0.01
Acquisition_1C.doppler_max=10000
Acquisition_1C.doppler_step=250
Tracking_1C.implementation=GPS_L1_CA_DLL_PLL_Tracking
Tracking_1C.item_type=gr_complex
Tracking_1C.pll_bw_hz=35.0
Tracking_1C.dll_bw_hz=2.0
TelemetryDecoder_1C.implementation=GPS_L1_CA_Telemetry_Decoder
Observables.implementation=Hybrid_Observables
PVT.implementation=RTKLIB_PVT
PVT.positioning_mode=Single
PVT.iono_model=Broadcast
PVT.trop_model=Saastamoinen
PVT.output_rate_ms=100
PVT.display_rate_ms=500
PVT.output_path=logs/gnss
PVT.flag_rtcm_server=false
```
Format mapping: cs8 = `item_type=ibyte` + `Ibyte_To_Complex`; cs16 = `ishort` + `Ishort_To_Complex`;
cf32 = `gr_complex` + `DataTypeAdapter.implementation=Pass_Through`; cu8 has no direct type: convert
to cf32 first. Keep `internal_fs_sps` equal to the file rate (or configure a resampler). Run:
`mkdir -p logs/gnss && gnss-sdr --config_file=logs/gnss/gps_l1_file.conf --log_dir=logs/gnss 2>&1 | tee logs/gnss/run_${TS}.txt`
(background job; a file source stops at end of file). Good signs: "Tracking of GPS L1 C/A signal
started on channel N for satellite GPS PRN xx", navigation messages decoded, then "Position at ...
using N observations"; KML/GPX/NMEA/RINEX files appear in `logs/gnss`. Galileo E1: start from the
`File_input/Galileo` examples; recent gnss-sdr can authenticate Galileo navigation data with OSNMA
(`GNSS-SDR.osnma_enable=true`; needs the public key and Merkle tree files published by the European
GNSS Service Centre, see gnss-sdr docs). `front-end-cal` estimates front-end frequency/rate error from
GPS (designed for RTL-SDR E4000 front ends).

## Phase 4: jamming and spoofing indicators
- Spectrum/IQ: floor raised compared with a reference capture, CW lines, sawtooth chirps in
  inspectrum, pulsed bursts; gnss-sdr acquires nothing despite open sky.
- gnss-sdr/receiver output: C/N0 suspiciously uniform and high on all satellites (above about
  50 dB-Hz outdoors is unusual), position or time inconsistent with ground truth, sudden jumps in
  position, clock bias or time, satellites with implausible Doppler for the location, the same PRN
  acquired at two code phases, missing satellites that should be visible.
- u-blox flags: MON-HW `jamInd`/AGC and jamming state, MON-RF per-band jamming state and noise,
  MON-SPAN spectrum (newer generations), NAV-STATUS `spoofDetState` (2 or 3 = spoofing indicated),
  SEC-SIG jamming/spoofing state, NAV-SAT C/N0 per satellite.
Benign causes exist (indoor reception, multipath, a damaged antenna cable, nearby electronics). Site
interference is an observation for the report, not a target vulnerability.

## Phase 5: the target's receiver over serial
- The serial port must be in the mission: the operator binds it (`rfswift config bindings add -c NAME
  -d -t /dev/ttyACM0`); u-blox USB IDs start with `1546:`. Check `ls -l /dev/ttyACM* /dev/ttyUSB*`.
- NMEA, passive: `stty -F /dev/ttyACM0 9600 raw && timeout 20 cat /dev/ttyACM0 | tee logs/nmea_${TS}.txt`
  (GSV carries per-satellite SNR), or an interactive `session_start {name: "gnss", command: "screen /dev/ttyACM0 9600"}`.
- u-blox polls (read `ubxtool -h -v 2` for presets; set `-P` to the PROTVER shown by MON-VER):
```sh
ubxtool -f /dev/ttyACM0 -s 9600 -p MON-VER -w 3 | tee logs/ubx_monver_${TS}.txt
ubxtool -f /dev/ttyACM0 -s 9600 -P 18 -p MON-HW -p MON-RF -p NAV-STATUS -p NAV-SAT -p SEC-SIG -w 5 -tt | tee logs/ubx_status_${TS}.txt
ubxtool -f /dev/ttyACM0 -s 9600 -r -R logs/ubx_raw_${TS}.ubx -w 60      # read-only raw log as evidence
```
- `gpsmon /dev/ttyACM0` is interactive (use a session); with `gpsd -N -n /dev/ttyACM0` as a background
  job, `gpspipe -w -n 100 | tee logs/gpsd_${TS}.json` records JSON reports.

## Phase 6: lab simulation (TX, conducted or shielded only)
Preconditions, all of them: operator OK for this run; the target's RF input is reached by coax
through a DC block (receivers feed antenna DC) and fixed attenuators, or the target sits in a
shielded enclosure; total attenuation chosen so the simulated signal reaches the receiver around
-120 to -110 dBm (real signals are near -130 dBm; HackRF at `-x 0 -a 0` still outputs roughly
-50 dBm at L1, so 60-80 dB is typical: measure, do not guess); leakage check with a second receiver
or phone outside the box, which must keep its real fix. Never connect a TX port directly to a receiver.
```sh
# operator supplies the GPS broadcast ephemeris (RINEX nav, e.g. daily brdcDDD0.YYn)
gps-sdr-sim -e captures/brdcDDD0.YYn -l 48.8584,2.2945,100 -b 8 -s 2600000 -d 120 \
  -o captures/1575.42M_2.6Msps_${TS}_gpssim-static.cs8
hackrf_transfer -t captures/1575.42M_2.6Msps_${TS}_gpssim-static.cs8 -f 1575420000 -s 2600000 -x 0 -a 0
```
gps-sdr-sim: `-b 8` is required for HackRF, `-s` defaults to 2.6 Msps, `-d` is limited to 300 s in
dynamic mode (86400 s static), `-t` sets the scenario start time, `-u`/`-x`/`-g` take motion files,
`-p` holds power constant; check `gps-sdr-sim` without arguments for its usage. The file is 5.2 MB/s.
USRP playback uses `gps-sdr-sim-uhd.py` (check `--help`), same rules.
Test ideas (one at a time, each with its own OK): static false position; slow drift away from the true
position (does any plausibility check trigger?); position jump; time jump (past/future: certificate
checks, logs, OTA scheduling); simulated loss (stop TX) and recovery. Record the target's NMEA/UBX
output and application behaviour throughout. Afterwards stop TX, have the operator cold-start or
power-cycle the target, and confirm it returns to its real fix.
GNSS loss without TX: disconnect the antenna and watch alarms, holdover and fallback behaviour.

## Evidence
- `logs/sweep_1555-1615M_*` (CSV/PNG), L1 captures with `.sigmf-meta` and `captures/SHA256SUMS`,
  `logs/gnss/` (config, run log, KML/NMEA/RINEX), `logs/ubx_*`/`logs/nmea_*`, gps-sdr-sim command
  line and ephemeris file name, photos/description of the conducted or shielded setup (operator).
- Mission note: antenna and bias, radio and ppm, rates, gnss-sdr result (satellites, C/N0 range, fix),
  receiver flags, and for every TX run: scenario, attenuation, enclosure, duration, UTC, operator OK
  reference, observed target behaviour, restore steps.

## Typical findings
Save with `save_finding` (stored with status `draft`; only the operator confirms); see
`rfswift-findings-qualification` for scoring.
- "Target accepts simulated GNSS position/time without plausibility checks": confirmed when the target
  reports or acts on the simulated position/time in the conducted test; real-world vector is radio:
  `CVSS:3.1/AV:A/AC:H/PR:N/UI:N/S:U/C:N/I:H/A:H` (6.8). Candidate if only the receiver model is known
  to lack spoofing detection.
- "GNSS time trusted without bounds" (time jump accepted, affecting certificate validation, logs or
  schedules): confirmed by the time-jump test; I:H, consider S:C when other components rely on it.
- "No alarm or unsafe fallback on GNSS loss": confirmed by the antenna-disconnect test,
  `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H` (6.5) when the function depends on GNSS.
- "Receiver spoofing/jamming flags ignored by the application": flags set during the lab test but no
  reaction by the target.
Remediation: act on receiver integrity flags, multi-constellation with authenticated navigation data
(Galileo OSNMA), plausibility checks against inertial/odometry/network time and position, bounded time
steps and monotonic clocks, holdover with a stable oscillator plus alarms, antenna placement, signed
time sources (NTS/PTP with authentication) as a cross-check.

## Pitfalls
- Indoors there is no GNSS: record with sky view, or the "jamming" you see is a roof.
- Bias tee on a passive antenna or into a splitter/attenuator without DC block can damage parts; an
  active antenna without bias shows only noise.
- Uncorrected ppm on a non-TCXO radio hides every satellite outside the Doppler window.
- Too short recordings never yield a fix; too long ones fill the disk (4 Msps cs8 = 480 MB/min).
- A shielded box with a cable feed-through can leak: check outside before every simulation run.
- Spoofed time persists: receivers keep it in battery-backed RAM; cold start after tests.
