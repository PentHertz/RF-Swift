---
name: rfswift-spectrum-survey
description: Find and identify RF emitters from an RF Swift mission. Wideband sweeps with hackrf_sweep and rtl_power to CSV, heatmaps and peak/occupancy tables with a small numpy/matplotlib script, target on/off differential surveys, narrowing to one channel, burst and duty-cycle observation, operator GUIs shipped by RF Swift (gqrx, SDR++, SDRangel, CubicSDR, CyberEther, inspectrum, retrogram-soapysdr, Artemis signal database), signal identification with SigIDWiki and FCC ID/ISED lookups done by the operator, band plans (315/433/868/915 MHz ISM and SRD, 2.4/5 GHz, cellular, GNSS L1/L2/L5, aviation VHF/ADS-B, AIS), recognising overload, DC spike, IQ images and intermodulation, and logging a survey as evidence. Use for spectrum scan, waterfall, sweep, occupancy, unknown transmitter, which frequency does this device use.
---
# Spectrum survey and emitter identification

## Scope
Answer "what transmits, where, when and how strongly" before any capture or decoding: wideband
sweeps, heatmaps, attribution of emitters to the target, a first guess of modulation, and a band-plan
based shortlist of protocols. Prerequisite: a validated radio (`rfswift-sdr-setup`).
Not covered: recording IQ properly (`rfswift-iq-capture`), demodulating and decoding
(`rfswift-signal-analysis`), sub-GHz device testing (`rfswift-subghz-ism`), GNSS interference work
(`rfswift-gnss`), Wi-Fi (`rfswift-wifi-assessment`), Bluetooth (`rfswift-bluetooth-ble`), LoRa,
Zigbee, Thread, Meshtastic (`rfswift-iot-lpwan-mesh`).

## Ground rules
- This skill only receives. No command here needs `transmits: true`. If the survey leads to a replay or
  any active test, switch to the skill for that domain and follow its TX gate (operator enables
  "Allow RF transmission", explicit OK in chat for that transmission, `transmits: true`, frequency,
  gain, duration and target in `purpose`; licensed bands only conducted or in a shielded enclosure).
- A survey sees everybody's radios. Measure power and timing; decode only the in-scope target. Do not
  demodulate or record the content of third-party communications (cellular, pagers, voice, public
  safety): interception law applies even to passive receivers. Keep bystander identifiers (TPMS IDs,
  sensor IDs, MACs) out of notes unless they belong to the target.
- Online lookups (FCC ID, SigIDWiki, vendor pages) are for the operator, or need the operator's OK;
  never send mission data to web services.
- Sweeps are long-running: start them as background jobs (`wait_seconds` short, `max_seconds` set),
  follow them with `job_output`, stop with `cancel_job`. One program per radio at a time.

## Tools in RF Swift
| Tool | Role | Docker image | Nix env |
|---|---|---|---|
| `hackrf_sweep` | fast sweep 1 MHz-6 GHz, CSV | sdr_light, sdr_full, sdr_hacks | sdr_light, sdr_full, sdr_hacks |
| `rtl_power` | hopping sweep about 24-1766 MHz, CSV | sdr_light+ | sdr_* |
| `python3` + numpy + matplotlib | heatmap and peak table (script below) | every image | python3 everywhere; numpy only listed in sdr_full, check `python3 -c 'import numpy, matplotlib'` |
| `rtl_433` | quick ID of ISM devices | sdr_light+ | sdr_* |
| `retrogram-soapysdr` | text-mode spectrogram (interactive session, check `--help`) | sdr_light+ | sdr_* |
| `gqrx`, `sdrpp` (SDR++), `gqrx-scanner` | operator waterfall/demod, scanner via gqrx remote control | sdr_light+ | sdr_* |
| `sdrangel` | operator GUI | sdr_full | sdr_light, sdr_full |
| `CubicSDR` | operator GUI | not in Docker images | sdr_* |
| `cyberether` | GPU waterfall | sdr_full | cyberether, cyberether_release |
| `inspectrum`, `SigDigger` | offline spectrogram / analyzer | inspectrum sdr_light+, SigDigger sdr_full | inspectrum sdr_*, SigDigger sdr_full |
| `Artemis` | offline signal identification database (AresValley) | sdr_light+ | not shipped |
Not shipped by RF Swift: SoapyPower (`soapy_power`) and the classic `heatmap.py`; the script below
replaces them. `dump1090`/`readsb` (ADS-B) exist in sdr_light if aviation context is needed.

## Phase 1: plan
1. Read the mission note: target, bands in scope, antenna, radio and its measured ppm.
2. Ask the operator for the FCC ID / ISED / CE label on the target. The operator's FCC ID lookup
   (grantee + product code) gives the authorized frequency range, output power, test reports and
   internal photos; the radio chip in the photos (CC1101, Si4463, SX126x, nRF52, ESP32...) narrows the
   protocol family.
3. Plan a differential survey: baseline with the target off or idle, then with it active (button
   pressed, sensor triggered, app action, power-up). Ask the operator to perform the actions and say
   when; note UTC times. Put the antenna close to the target so it dominates.

## Phase 2: wide sweep
```sh
TS=$(date -u +%Y-%m-%dT%H-%MZ)
# HackRF: 300-930 MHz, 50 kHz bins, 120 sweeps (seconds to minutes). -f is in MHz, -w in Hz (2445-5000000).
hackrf_sweep -f 300:930 -w 50000 -a 0 -l 24 -g 20 -N 120 -r logs/sweep_300-930M_50k_${TS}_baseline.csv
# Whole HackRF range, coarse: -f 1:6000 -w 1000000
# RTL-SDR: hops of up to ~2.8 MHz, 2 s integration, 10 min, crop 20 % of each hop's edges
rtl_power -f 430M:440M:5k -g 30 -i 2 -e 10m -c 20% logs/rtlpower_430-440M_5k_${TS}_baseline.csv
```
Then repeat with the target active (`..._active.csv`). Sweep time grows with span and with smaller bins;
rtl_power over wide spans is slow (hops x integration). `hackrf_sweep -1` does one sweep; newer
releases also have `-n` (one timestamp per sweep): check `hackrf_sweep -h`. Levels are relative dB,
not dBm.

## Phase 3: heatmap and peak table
Save it once as `scripts/sweep_heatmap.py` in the workspace (`mkdir -p scripts`, then a quoted heredoc),
then run it per CSV:
```python
#!/usr/bin/env python3
"""sweep_heatmap.py <sweep.csv> <out.png> [top_n]: heatmap + peak table for hackrf_sweep/rtl_power CSV."""
import csv, sys
import numpy as np
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

rows = [r for r in csv.reader(open(sys.argv[1])) if len(r) > 6]
start, step = min(float(r[2]) for r in rows), float(rows[0][4])
sweeps, cur = [], []
for r in rows:                      # a new sweep starts each time the lowest segment comes back
    lo = float(r[2])
    if lo == start and cur:
        sweeps.append(cur); cur = []
    cur += [(lo + i * step, float(v)) for i, v in enumerate(r[6:])]
sweeps.append(cur)
sweeps = [s for s in sweeps if len(s) >= 0.9 * max(len(x) for x in sweeps)]
grid = np.arange(start, max(f for s in sweeps for f, _ in s) + step, step)
m = np.array([np.interp(grid, *np.array(sorted(s)).T) for s in sweeps])
floor = np.nanmedian(m)
peak, occ = m.max(axis=0), (m > floor + 10).mean(axis=0) * 100
plt.figure(figsize=(14, 6))
plt.imshow(m, aspect="auto", cmap="viridis", extent=[grid[0] / 1e6, grid[-1] / 1e6, len(m), 0])
plt.colorbar(label="dB (uncalibrated)"); plt.xlabel("MHz"); plt.ylabel("sweep #"); plt.title(sys.argv[1])
plt.savefig(sys.argv[2], dpi=110, bbox_inches="tight")
picked, sep = [], max(1, int(100e3 / step))
for i in np.argsort(peak)[::-1]:
    if all(abs(i - j) > sep for j in picked):
        picked.append(i)
    if len(picked) == (int(sys.argv[3]) if len(sys.argv) > 3 else 15):
        break
print(f"# {len(m)} sweeps, {grid.size} bins of {step:.0f} Hz, median floor {floor:.1f} dB")
print("freq_MHz,max_dB,mean_dB,above_floor+10dB_%")
for i in sorted(picked):
    print(f"{grid[i] / 1e6:.4f},{peak[i]:.1f},{m[:, i].mean():.1f},{occ[i]:.1f}")
```
```sh
python3 scripts/sweep_heatmap.py logs/<sweep>.csv logs/<sweep>.png 20 | tee logs/<sweep>_peaks.csv
```
Reading it: vertical lines that are always present are continuous carriers (broadcast, local
oscillators, spurs); dotted vertical traces are bursty emitters, and the `above_floor` column is a
rough occupancy. Compare baseline and active tables: peaks that appear or grow only when the target is
active are attribution candidates. Keep `max_dB` and `mean_dB` apart: a high max with a low mean is a
short burst. Show the PNG to the operator (it lands in the Artifacts panel).

## Phase 4: narrow to a channel
1. Zoom sweep around each candidate: `hackrf_sweep -f 433:435 -w 5000 ...` or
   `rtl_power -f 433.7M:434.2M:1k -i 1 -e 2m ...`.
2. Make a short IQ capture tuned 200-500 kHz away from the candidate so it does not sit on the DC spike
   (`rfswift-iq-capture`), then measure bursts, bandwidth and frequency offset
   (`rfswift-signal-analysis`).
3. ISM candidates (315/433/868/915 MHz): a quick live `rtl_433 -f 433.92M -M level -F json:logs/rtl433_${TS}.json`
   often names the device outright (`rfswift-subghz-ism`).
4. Operator GUIs (open on the operator's display; launch only when asked): `gqrx`, `sdrpp`, `sdrangel`,
   `CubicSDR` (Nix), `cyberether`, offline `inspectrum -r <rate> captures/<file>`. gqrx can replay a
   cf32 file with the device string `file=/path/x.cf32,rate=2e6,freq=433.92e6,repeat=true,throttle=true`.
   Stop agent jobs on that radio first.

## Phase 5: bursts and duty cycle
Record 2-10 minutes of the channel at a low rate (RTL-SDR 250 ksps = 0.5 MB/s, HackRF 2 Msps cs8 =
4 MB/s) while the operator logs actions, then run the burst detector from `rfswift-signal-analysis`:
it lists burst start, duration, gap and level, and prints the duty cycle and typical period.
Periodic beacons (fixed interval, often with jitter) are sensors and meters; event-driven bursts that
match operator actions are remotes and alarms; continuous wide signals are video links, Wi-Fi or
digital broadcast. A measured duty cycle far above the band's limit (for example 1 % in EU 868.0-868.6
MHz) is a compliance remark, not a security finding.

## Identification
Collect for each emitter: corrected centre frequency, occupied bandwidth (about -20 dB), envelope (on/off
= OOK/ASK, constant = FSK/PSK), two spectral humps = 2-FSK, diagonal chirps = LoRa CSS, flat-topped wide
= OFDM (Wi-Fi, LTE, DVB), hopping = Bluetooth/FHSS, burst length, repetition period.
Then match: `Artemis` (operator GUI, filter by frequency, bandwidth, modulation), SigIDWiki (operator,
online), FCC test report (operating frequency, modulation, duty cycle). Record a confidence level.
| Band | Typical occupants | Follow-up skill |
|---|---|---|
| 300-350 MHz (315 MHz) | US/JP remotes, garage doors, car fobs, TPMS (US) | rfswift-subghz-ism |
| 433.05-434.79 MHz | Region 1 ISM/SRD: remotes, weather stations, sensors, TPMS (EU), LoRa EU433 | subghz-ism / iot-lpwan-mesh |
| 863-870 MHz | EU SRD: alarms, Z-Wave 868.42, wM-Bus 868.3/868.95, LoRaWAN EU868, Sigfox ~868.1 | subghz-ism / iot-lpwan-mesh |
| 902-928 MHz | US ISM: LoRaWAN US915, Z-Wave 908.42/916, ERT meters, FHSS links | subghz-ism / iot-lpwan-mesh |
| 2400-2483.5 MHz | Wi-Fi, BLE/BR-EDR, 802.15.4, nRF24 proprietary, drones | wifi-assessment / bluetooth-ble / iot-lpwan-mesh |
| 5150-5850 MHz | Wi-Fi 5 GHz, 5.8 GHz FPV video | rfswift-wifi-assessment |
| 700-960, 1710-2170, 2500-2690, 3300-3800 MHz | LTE / 5G NR (licensed) | power only, no content |
| 1559-1610 MHz (L1/E1 1575.42, B1I 1561.098, GLONASS ~1602), 1227.60 (L2), 1176.45 (L5/E5a) | GNSS (protected) | rfswift-gnss |
| 108-137 MHz, 978/1090 MHz | aviation voice/nav, UAT/ADS-B (safety of life) | receive only |
| 161.975/162.025 MHz | AIS (marine) | receive only |
| 1880-1900 MHz (EU) | DECT | power only |
Band edges and channel plans differ by country: confirm against the national table with the operator.

## Telling real signals from artefacts
- Retune test: move the centre by an odd amount (for example +0.37 MHz). Real emitters stay at their
  absolute frequency; the DC spike moves with the centre, IQ images move the opposite way (mirror
  around the centre), LO spurs move or vanish.
- Gain test: drop gain 10 dB. Real signals drop 10 dB; third-order intermodulation drops about 30 dB;
  a rising noise floor or "new" signals at high gain mean overload. 8-bit radios overload first.
- Antenna test: disconnect the antenna (or swap for a 50-ohm load). Anything still there is local:
  SDR clocks, host USB 3 noise (notorious around 2.4 GHz), switching supplies, the laptop.
- Sweep seams: hackrf_sweep stitches 5 MHz segments; level steps at regular spacing are seams, not
  emitters. Strong out-of-band signals (FM broadcast, cellular downlink) can fold into sub-GHz views:
  use a band-pass filter if the operator has one.

## Evidence
- Keep raw CSVs, PNGs and peak tables in `logs/` with band, bin width, UTC time and condition in the
  name: `logs/sweep_300-930M_50k_2026-11-24T10-05Z_active.csv`, `..._active.png`, `..._active_peaks.csv`.
- Mission note (`write_note`, append): radio, serial, antenna, gain, ppm, the exact sweep commands, the
  operator action timeline (UTC), and an emitter table:
  `| freq (corrected) | BW | envelope/mod guess | timing | level | attributed to target? (how) | ID guess + confidence | next step |`.
- Attribution must say how it was shown: on/off differential, level rises near the target, timing
  matches operator actions.

## Typical findings
Surveys mostly produce leads, not vulnerabilities. Possible target findings once confirmed:
- "Undocumented radio interface active on <freq>" (target emits outside its declared bands or has a
  debug radio enabled). Confirmed: differential and proximity attribution plus capture; candidate:
  only a coincident peak. Informational until a security impact is shown.
- "Device broadcasts identifiers continuously" (tracking/privacy): needs decoding of a stable ID across
  captures (`rfswift-signal-analysis`); typical vector `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N` (4.3).
Save them with `save_finding` (stored with status `draft`; only the operator confirms). Remediation patterns: disable
unused radios and debug links in production firmware, randomise or rotate identifiers, encrypt
telemetry.

## Pitfalls
- Uncorrected ppm shifts every reported frequency; apply or state the correction.
- hackrf_sweep and rtl_power levels are uncalibrated and differ between devices and gains; compare
  only runs made with the same radio, gain and antenna.
- A burst shorter than one sweep revisit is easily missed: use a fixed-tuned capture for rare bursts.
- Wide spans with fine bins produce huge CSVs; check `df -h .` and narrow the span instead.
- The operator's GUI and your job cannot share one radio; coordinate before starting either.
