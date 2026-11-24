---
name: rfswift-subghz-ism
description: Assess 300-928 MHz ISM/SRD devices from an RF Swift mission (garage and gate remotes, car keyfobs (RKE), alarm door/window/PIR sensors and keypads, doorbells, weather stations, TPMS tyre sensors, smart meters such as wM-Bus and ERT) on 315, 433.92, 868 and 915 MHz. Covers rtl_433 live decoding to JSON, capture and analysis workflow, fixed versus rolling code determination (KeeLoq HCS, Somfy RTS, Security+, Princeton/EV1527), replay testing under the TX gate with HackRF hackrf_transfer -t, YARD Stick One with rfcat -r, Flipper Zero .sub files, URH, rolling-code control tests (used code, unheard code, resync, modified frame), RollJam and RollBack described without jamming, TPMS decoding, alarm supervision checks, proof criteria, CVSS hints and remediation. LoRa, Z-Wave and Zigbee live in rfswift-iot-lpwan-mesh.
---
# Sub-GHz ISM devices (remotes, keyfobs, sensors, TPMS, meters)

## Scope
Short-range devices between 300 and 928 MHz that use simple OOK/FSK packets: gate/garage remotes,
vehicle remote keyless entry, alarm sensors and keyfobs, doorbells, weather stations, TPMS, wM-Bus
and ERT meters. Not here: LoRa/LoRaWAN, Meshtastic, Z-Wave, Zigbee (`rfswift-iot-lpwan-mesh`); radio
setup (`rfswift-sdr-setup`); finding the frequency (`rfswift-spectrum-survey`); recording rules
(`rfswift-iq-capture`); demodulation and bit-level reverse engineering (`rfswift-signal-analysis`).

## Ground rules
- Written authorization from the owner of each target (gate, vehicle, alarm system) recorded in the
  mission note. Physical safety: a moving gate or garage door, a vehicle that unlocks in a public
  place, an alarm that calls a monitoring centre. Agree with the operator who watches the target.
- Receiving is free (rtl_433, captures, analysis). TX is everything that radiates: `hackrf_transfer -t`
  (and `-c`), rfcat `d.RFxmit(...)`, URH Generator "Send" and `urh_cli -tx`, Flipper Zero sending a
  `.sub`, GNU Radio sinks. For each transmission: the operator enabled "Allow RF transmission", gave
  an explicit OK in chat for that transmission, `transmits: true` is set, and `purpose` states
  frequency, gain/power, duration and target, e.g. "TX replay captures/x_cut.cs8 at 433.92 MHz
  (centre 433.42 + 500 kHz), HackRF -x 10 -a 0, 0.4 s, target: client gate receiver".
- Transmit only on the frequency the device itself uses, at the lowest power that works, a few
  frames at a time, close to the target, inside a shielded box or over coax with attenuators when
  the receiver can be taken out. Do not transmit near other people's receivers. Licensed bands
  (cellular, aviation, public safety, GNSS) are never transmitted over the air.
- Jamming is never done: it is illegal in most jurisdictions and blocks everyone's devices, including
  alarms. Refuse jamming requests (no carrier/noise TX, no "RollJam" tooling); the non-jamming tests
  below show the same weakness.
- Rolling-code tests can desynchronise the owner's remote. Agree the recovery procedure (re-pairing,
  resync presses from the manual) before the first test.
- Bystanders: rtl_433 also decodes neighbours' sensors and passing cars' TPMS. Keep only the target's
  IDs; filter outputs and never store third-party identifiers in notes or findings.
- Decoded fixed codes and Flipper key values are credentials: `save_secret` (kind `other`, source =
  file and frame) and refer to them by file/frame elsewhere.

## Tools in RF Swift
| Tool | Role | Docker image | Nix env |
|---|---|---|---|
| `rtl_433` | live/file decoding of 200+ device types, `-A` analyzer | sdr_light, sdr_full, sdr_hacks | sdr_light, sdr_full, sdr_hacks |
| `hackrf_transfer` | capture and TX replay of IQ | sdr_light+ | sdr_*, bluetooth, telecom |
| `rfcat` (`rflib`) | YARD Stick One / CC1111 packet radio, interactive `rfcat -r` | sdr_light+ | sdr_* (python3Packages.rfcat) |
| `urh`, `/opt/urh-venv/bin/urh_cli` | analysis, generator/TX | sdr_light+ | sdr_* (urh-ng) |
| `inspectrum`, scripts of `rfswift-signal-analysis` | offline analysis | sdr_light+ | sdr_* |
| `hydrasdr_433` | rtl_433-style decoder for HydraSDR | sdr_light+ | sdr_* |
| gr-keyfob | GNU Radio keyfob transceiver blocks | sdr_full | not listed |
| `qFlipper`, `ufbt` | Flipper Zero desktop tool and SDK | rfid | rfid |
Flipper `.sub` files are plain text: if the operator added them to the mission, read them with
`read_artifact_content` or `cat`.

## Phase 1: identify and decode live (passive)
Frequency from the label/FCC filing or a survey; then let rtl_433 try (background job, operator
presses buttons or triggers the sensor while it runs):
```sh
TS=$(date -u +%Y-%m-%dT%H-%MZ)
rtl_433 -f 433.92M -M level -M time:iso:utc -F json:logs/rtl433_433.92M_${TS}.json -F kv -T 300
rtl_433 -f 868.3M -s 1024k ...       # EU 868 SRD;  -f 315M for US/JP;  -f 915M for US 915
rtl_433 -f 433.92M -f 868.3M -H 30 ...                       # hop between bands every 30 s
rtl_433 -d driver=hackrf -g "LNA=24,VGA=20,AMP=0" -f 433.92M ...   # any SoapySDR radio; gain names from SoapySDRUtil --probe
```
`-M level` adds RSSI/SNR; move closer to the target and watch which `model`/`id` grows: that is the
target. List decoders with `rtl_433 -R help`; in this build, for example, `[131]` Microchip
HCS200/HCS300 KeeLoq, `[167]` Somfy RTS, `[164]`/`[178]` Security+ 2.0/1.0 keyfobs, `[169]` Nice
Flor-s, TPMS decoders (Schrader, Toyota, Ford, Renault, Citroen, Hyundai VDO, Jansite...), `[104]`/`[105]`
Wireless M-Bus C&T (`-f 868.95M -s 1200k`) and S (`-f 868.3M -s 1000k`), `[149]`/`[160]` ERT SCM/IDM
(US meters around 912 MHz). Numbers change between versions: grep the help, then `-R <n>` to keep
only the relevant ones. Nothing decodes: record IQ (`rfswift-iq-capture`, `rtl_433 -S unknown`) and
go to `rfswift-signal-analysis`.

## Phase 2: fixed or rolling?
Capture 3-5 presses of the same button spread over time, plus each other button, then compare
decoded frames (`bitdiff.py` in `rfswift-signal-analysis`).
- Identical frames for the same button: fixed code. Typical families: Princeton PT2262/EV1527 style
  24-bit frames (20-bit ID + 4 data bits), 12-bit DIP-switch remotes, many doorbells and sensors.
- A block of about 32 bits where half the bits flip on each press plus a constant serial: hopping
  code. KeeLoq HCS frames carry 66 bits, sent LSB first: 32-bit encrypted hopping part, 28-bit serial,
  4 button bits, 2 status bits (battery low, repeat); rtl_433 `[131]` prints them separately.
- A clear counter that increments per press with only a CRC: rolling counter without cryptographic
  protection (forgeable in principle; Somfy RTS obfuscates rather than encrypts).
- Two-way systems (the receiver answers) need challenge-response analysis; capture both directions.
Static frames make replay likely; the replay test confirms it.

## Phase 3: replay tests (TX, gated)
Before any TX: authorization recorded, operator OK for this transmission, recovery plan for rolling
codes, a second receiver (RTL-SDR with rtl_433 or a capture) recording your own transmission as
evidence of exactly what was sent.
A. HackRF replay of a recorded burst (works for OOK and FSK): cut the frames of one press, with a few
   ms of margin, from the original (`dd` recipe in `rfswift-iq-capture`), then transmit at the same
   centre and rate it was recorded with:
```sh
hackrf_transfer -t captures/433.42M_2Msps_2026-11-24T10-05Z_gate-btnA_cut.cs8 -f 433420000 -s 2000000 -x 0 -a 0
```
   Start at `-x 0 -a 0` and raise `-x` in steps of 5-10 dB only until the target reacts; never `-R`
   (endless repeat) and always `max_seconds`. A raw replay also re-radiates the recorded noise; for OOK
   a synthesized signal is cleaner (B).
B. Synthesize OOK from timings (Flipper RAW `.sub`, `rtl_433 -A` timings, or a modified frame for the
   integrity test). Save as `scripts/ook_synth.py`:
```python
#!/usr/bin/env python3
"""ook_synth.py <rate> <offset_hz> <out.cs8> <file.sub | timings...>
Builds a HackRF cs8 file from OOK timings in microseconds (+ = carrier on, - = off), taken from a
Flipper RAW .sub file (RAW_Data lines) or the command line. The tone sits at +offset_hz from the centre."""
import re, sys
import numpy as np
rate, off, out = float(sys.argv[1]), float(sys.argv[2]), sys.argv[3]
args = sys.argv[4:]
if len(args) == 1 and args[0].endswith(".sub"):
    args = re.findall(r"-?\d+", " ".join(re.findall(r"RAW_Data:(.*)", open(args[0]).read())))
t = np.array([int(a) for a in args])
env = np.concatenate([np.zeros(int(rate * 0.01))] +
                     [np.full(int(round(abs(d) * rate / 1e6)), 1.0 if d > 0 else 0.0) for d in t] +
                     [np.zeros(int(rate * 0.01))])
iq = 100 * env * np.exp(2j * np.pi * off * np.arange(env.size) / rate)
buf = np.empty(2 * iq.size, np.int8)
buf[0::2], buf[1::2] = np.round(iq.real), np.round(iq.imag)
buf.tofile(out)
print(f"{out}: {iq.size} samples, {iq.size / rate * 1e3:.1f} ms, {np.count_nonzero(t > 0)} pulses")
```
   `python3 scripts/ook_synth.py 2000000 500000 captures/433.42M_2Msps_${TS}_synth-btnA.cs8 captures/btnA.sub`
   then `hackrf_transfer -t <that file> -f 433420000 -s 2000000 -x 0 -a 0` (target = centre + 500 kHz).
   Verify the file first with `rtl_433 -R 0 -A -r <that file>`: the decoded bits must equal the
   original. Flipper presets named `...Ook...` are OOK; `...2FSK...` presets need FSK, use A instead.
   Flipper "Key" files (`Protocol: Princeton`, `Bit:`, `Key:`, `TE:`) already show a static code.
C. YARD Stick One (CC1111: 300-348, 391-464, 782-928 MHz) with rflib. Transmit from a one-shot
   script through `execute_command` with `transmits: true` (a session has no TX flag); keep the
   interactive `session_start {name: "ys1", command: "rfcat -r"}` (prompt `In [n]:`) for receive-side
   exploration such as `d.lowball()` and `d.RFrecv()`.
```sh
python3 - <<'EOF'
from rflib import *
d = RfCat()
d.setFreq(433920000); d.setMdmModulation(MOD_ASK_OOK); d.setMdmDRate(2500)   # 1 chip = Te = 400 us
d.setMdmSyncMode(SYNCM_NONE); d.setEnablePktCRC(False)
bits = "<frame bits from analysis>"                    # PWM with s=1 Te high (bit 1), l=3 Te high (bit 0)
chips = "".join("1000" if b == "1" else "1110" for b in bits) + "0" * 24
chips += "0" * (-len(chips) % 8); pkt = int(chips, 2).to_bytes(len(chips) // 8, "big")
d.makePktFLEN(len(pkt)); d.RFxmit(pkt, repeat=3); d.setModeIDLE()
EOF
```
   Adapt the chip patterns to the measured timings. Do not call `d.setMaxPower()`; keep the default
   power or set a low PA value with `d.setPower()`. Close any `ys1` session first (one program per
   dongle) and with `session_close` when done.
D. URH Generator (operator GUI) or Flipper Zero playback by the operator on the device: you record the
   result and the second receiver's capture.
Result: note for each attempt the file sent, gain, time (UTC), number of frames, and the observed
reaction (operator's observation, ideally video or the receiver's log).

## Phase 4: rolling-code control tests (no jamming)
Run in order, with the owner's remote at hand and the recovery plan agreed:
1. Used code: press the real remote within range (target reacts) while recording; replay that
   recording. Expected: rejected. Accepted = replay of consumed codes.
2. Unheard code: record a press with the receiver unable to hear it (receiver powered off or the remote
   far away, operator's choice); replay it once. Accepted is normal for rolling codes (look-ahead
   window) but shows that an intercepted, unused code stays valid: the precondition of RollJam. Then
   use the real remote and replay the same code again: it must now be rejected.
3. Resync (RollBack-style): record two or more consecutive unheard presses, use the real remote a few
   times, then replay the recorded codes in order. Accepted = resynchronisation flaw.
4. Integrity: flip one bit in the hopping or button field (synthesize with B) and transmit. Accepted =
   the receiver does not check authenticity.
RollJam, described only: an attacker jams the receiver while recording press 1, then jams and records
press 2 while replaying press 1, and keeps press 2 for later. Never performed by the agent; test 2
demonstrates the same property legally. A real-jamming variant belongs only in a shielded lab with
written authorization and regulatory permission, run by the operator, not by this skill.

## TPMS, alarms, meters
- TPMS (315 MHz US, 433.92 MHz EU; mostly FSK Manchester): sensors send when the wheel rotates, after
  a pressure change, or when woken by a 125 kHz service tool (operator's, not shipped). Parked cars
  rarely transmit. Try `-s 1024k` if the default rate misses frames. Findings: static sensor IDs
  (tracking) and unauthenticated pressure frames (spoofed warnings, test only with TX gate into the
  target vehicle with the owner present).
- Alarm sensors and keyfobs: look for static sensor IDs and state bits (open/closed, tamper, battery);
  replaying "closed/restore" or a disarm keyfob code is a TX test. Supervision without jamming: the
  operator removes a sensor battery (or puts the sensor in a shielding pouch) and you measure how long
  the panel takes to report the loss.
- Meters (wM-Bus, ERT): readings and IDs in clear are confidentiality findings; wM-Bus mode 5/7 frames
  are AES-encrypted (only the header is readable).

## Evidence
- `logs/rtl433_<freq>_<UTC>.json` (target IDs only, filtered with `jq` if needed), IQ captures with
  SigMF and checksums, decoded-frame tables and bitdiff output, synthesized TX files with their
  verification output, second-receiver recordings of every transmission.
- Mission note: device model, frequency, modulation, frame layout, fixed/rolling verdict with frames
  compared, every TX attempt (file, frequency, gain, duration, UTC, operator OK reference, result).

## Typical findings and proof
| Finding | Confirmed when | Candidate when | CVSS v3.1 hint |
|---|---|---|---|
| Static code replay opens gate/garage/vehicle | replay of an old capture actuates the target, repeatable | identical frames, no TX done | `AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N` 6.5 |
| Alarm disarmed or sensor state forged by replay | panel disarms / shows "closed" | static frames | disarm `AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:H` 8.1 |
| Consumed rolling code accepted, or resync flaw | test 1 or 3 succeeds | decoder shows counter only | 6.5, or `AC:H` 5.3 if unheard captures are needed |
| Unused codes stay valid (RollJam precondition) | test 2: accepted once, rejected after real use | design analysis | `AV:A/AC:H/PR:N/UI:R/S:U/C:N/I:H/A:N` 4.8 |
| Forged frame accepted (no authenticity) | test 4 succeeds | CRC-only frames | 6.5 |
| TPMS / meter / sensor IDs and data in clear | stable ID across captures | single capture | `AV:A/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N` 4.3 |
| Sensor loss not reported (no supervision) | panel silent beyond its stated window | vendor docs | `AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H` 6.5 |
Save with `save_finding` (stored with status `draft`; only the operator confirms). Remediation: cryptographic rolling codes
with per-device keys and a small look-ahead window, rejection of consumed codes, authenticated
resynchronisation, challenge-response (two-way) for access functions, timestamped or session-bound
codes, sensor supervision with short timeouts and jam detection reported to the panel, random or
rotating identifiers for TPMS/meters, wM-Bus encryption with per-meter keys.

## Pitfalls
- Recording the real remote next to its receiver consumes the code: plan which presses the receiver
  may hear.
- Replay at the wrong centre (or with the signal on the DC spike) fails silently; re-use the recorded
  centre and rate, check with the second receiver.
- HackRF TX also emits LO leakage at the centre and a weak image at minus the offset: keep power low.
- Clipped or noisy captures replay badly; synthesize from clean timings.
- Rolling-code receivers may need two presses to resync after tests; owner remotes may need
  re-pairing: tell the operator before you start.
- rtl_433 decoder numbers and flags differ by version: always confirm with `rtl_433 -h`/`-R help`.
