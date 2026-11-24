---
name: rfswift-signal-analysis
description: Offline analysis of captured IQ in an RF Swift mission, no radio needed. Burst detection and duty cycle with numpy, inspectrum and Universal Radio Hacker (urh GUI, urh_cli) for the operator, rtl_433 -r on cu8/cs8/cs16/cf32 files with the -A pulse analyzer and -X flex decoders, GNU Radio (gnuradio-companion, grcc, headless Python), SigDigger, modulation identification (OOK/ASK, 2-FSK/GFSK, BPSK/QPSK, LoRa CSS, OFDM), FSK deviation and symbol-rate estimation, line codes (PWM, PPM, NRZ, Manchester, differential Manchester), preambles, sync words, whitening, bit order, CRC and checksum search, comparing several captures (static ID, button, counter, rolling/encrypted fields) and producing a decoded-frame table as evidence. Use for demodulate, decode, reverse engineer an RF protocol, unknown signal, bitstream, symbol rate, Manchester, CRC.
---
# Offline signal analysis and protocol reverse engineering

## Scope
From a documented capture (`rfswift-iq-capture`) to a decoded-frame table with a field map: where the
bursts are, what modulation, which symbol rate and line code, how frames are built, and which fields
change between captures. Everything here works on files.
Not covered: recording (`rfswift-iq-capture`), finding the frequency (`rfswift-spectrum-survey`),
replay and fixed/rolling-code proof on sub-GHz devices (`rfswift-subghz-ism`), LoRa/Zigbee/Z-Wave
stacks (`rfswift-iot-lpwan-mesh`), BLE (`rfswift-bluetooth-ble`), GNSS (`rfswift-gnss`).

## Ground rules
- Offline analysis is passive. These analysis tools can also transmit, and then they are TX: URH
  "Send" in the Generator tab and `urh_cli -tx`, GNU Radio flowgraphs with an osmocom/Soapy/UHD sink,
  any script calling a radio's TX. They need the operator's "Allow RF transmission", an explicit OK
  in chat for that transmission, `transmits: true`, and frequency, gain, duration and target in
  `purpose`; licensed bands only conducted or in a shielded enclosure.
- Decode only the in-scope target. Captures contain neighbours' devices: identify and skip them.
- Decoded values that grant access (a fixed remote code, a keypad PIN, a key) are credentials: store
  them with `save_secret` (kind `other` or `pin`, exact source: file and burst index) and refer to them
  by frame index elsewhere instead of pasting the value into notes or findings.
- GUIs (inspectrum, URH, GNU Radio Companion, SigDigger) are for the operator: suggest the exact
  command and what to look at; launch only on request.

## Tools in RF Swift
| Tool | Role | Docker image | Nix env |
|---|---|---|---|
| `python3` + numpy (+ scipy) | burst list, FSK estimate, bit diff, CRC search (scripts below) | every image; scipy comes with GNU Radio | python3 everywhere; numpy listed only in sdr_full, check imports |
| `rtl_433 -r` | file decoding, `-A` pulse analyzer, `-X` flex decoder | sdr_light, sdr_full, sdr_hacks | sdr_light, sdr_full, sdr_hacks |
| `inspectrum` | spectrogram, amplitude/frequency/phase plots, symbol cursors | sdr_light+ | sdr_* |
| `urh` (GUI), `urh_cli` | demod/decoding GUI; CLI for live RX/TX with a radio | sdr_light+ (`urh_cli` is `/opt/urh-venv/bin/urh_cli`, not on PATH) | sdr_* (urh-ng) |
| `gnuradio-companion`, `grcc`, Python `gnuradio` | flowgraphs; `grcc -o DIR file.grc` compiles, `-r` runs | sdr_light+ | sdr_* |
| gr-inspector, gr-reveng, gr-lora_sdr, gr-keyfob | OOT blocks for detection, RE, LoRa | sdr_full | not listed |
| `SigDigger` | analyzer with baud-rate estimation | sdr_full | sdr_full |
| `jupyter` | notebooks for the operator | sdr_light+ | sdr_*, telecom |
Not shipped: `csdr`, CRC RevEng (`reveng`), Suscan CLI is not verified (check `command -v suscli`).

## Phase 0: inventory
`sha256sum -c captures/SHA256SUMS`, read the `.sigmf-meta` (`core:datatype`, `core:sample_rate`,
`core:frequency`, description with target offset) or the note entry. Wrong rate or format ruins every
later step. Write results to `logs/<capture base>_*.{csv,txt}`.

## Phase 1: find bursts
Save as `scripts/bursts.py` (loads the whole file: cut files above ~500 MB first, see `rfswift-iq-capture`):
```python
#!/usr/bin/env python3
"""bursts.py <file> <cs8|cu8|cs16|cf32> <rate> [thresh_db=10] [merge_ms=2]
CSV of bursts (start, duration, gap, peak over floor, strongest frequency offset); summary on stderr."""
import sys
import numpy as np
p, fmt, rate = sys.argv[1], sys.argv[2], float(sys.argv[3])
th = float(sys.argv[4]) if len(sys.argv) > 4 else 10.0
merge = int(rate * (float(sys.argv[5]) if len(sys.argv) > 5 else 2.0) / 1e3)
v = np.fromfile(p, dtype={"cs8": np.int8, "cu8": np.uint8, "cs16": "<i2", "cf32": "<f4"}[fmt]).astype(np.float32)
if fmt == "cu8":
    v -= 127.5
x = v[0::2] + 1j * v[1::2]
w = max(1, int(rate * 20e-6))                                   # 20 us envelope smoothing
env = np.convolve(np.abs(x), np.ones(w) / w, mode="same")
floor = np.median(env)
d = np.diff(np.concatenate(([0], (env > floor * 10 ** (th / 20)).astype(np.int8), [0])))
segs = []
for s, e in zip(np.flatnonzero(d == 1), np.flatnonzero(d == -1)):
    if segs and s - segs[-1][1] < merge:                         # glue pulses of one frame together
        segs[-1][1] = e
    else:
        segs.append([s, e])
print("idx,start_s,dur_ms,gap_prev_ms,peak_db_over_floor,f_off_hz")
for i, (s, e) in enumerate(segs):
    nfft = 1 << min(18, max(8, int(np.ceil(np.log2(e - s)))))
    spec = np.abs(np.fft.fftshift(np.fft.fft(x[s:e][:nfft], nfft)))
    gap = (s - segs[i - 1][1]) / rate * 1e3 if i else float("nan")
    print(f"{i},{s / rate:.6f},{(e - s) / rate * 1e3:.3f},{gap:.3f},"
          f"{20 * np.log10(env[s:e].max() / floor):.1f},{(np.argmax(spec) - nfft // 2) * rate / nfft:.0f}")
period = np.median(np.diff([s for s, _ in segs])) / rate if len(segs) > 1 else float("nan")
sys.stderr.write(f"{len(segs)} bursts in {x.size / rate:.2f} s, duty cycle "
                 f"{100 * sum(e - s for s, e in segs) / x.size:.3f} %, median period {period:.4f} s\n")
```
`python3 scripts/bursts.py captures/<f>.cs8 cs8 2000000 | tee logs/<f>_bursts.csv`. Group bursts by
`f_off_hz` and level: one device = one offset and similar level; repeats of a frame come at a fixed
gap. `merge_ms` decides what counts as one frame (raise it for slow PWM). For a CW reference (ppm
measurement in `rfswift-sdr-setup`) the whole file is one burst and `f_off_hz` is the offset.

## Phase 2: look at it (operator)
- `inspectrum -r <rate> captures/<f>.cs8`: zoom on a burst; right-click the signal to add amplitude
  (OOK/ASK), frequency (FSK) or phase (PSK) plots and a threshold plot; enable cursors, set the
  symbol count, align them over many symbols to read the symbol period precisely; export symbols.
- `urh` (open the file, set the sample rate): Interpretation tab "Autodetect parameters" gives
  modulation, samples/symbol, centre and tolerance; Analysis tab shows bits, applies decodings
  (NRZ, Manchester I/II, differential Manchester, invert), aligns messages, shows differences, and
  lets you label fields including checksum labels with CRC presets. Ask the operator to save the
  project so parameters are reproducible.
- `SigDigger` (sdr_full): channel inspector with baud-rate estimator and constellation.

## Phase 3: identify the modulation
| Observation | Modulation | Confirm with |
|---|---|---|
| carrier switches on/off, one spectral line | OOK/ASK | `rtl_433 -A` pulse/gap histograms |
| constant envelope, two humps or two tones | 2-FSK (GFSK if transitions are smooth) | `fskinfo.py` below |
| constant envelope, no tones, phase jumps | PSK | spectral line of x^2 (BPSK) or x^4 (QPSK) at 2x/4x the offset |
| diagonal sweeps across 125/250/500 kHz | LoRa CSS | `rfswift-iot-lpwan-mesh` (gr-lora_sdr in sdr_full) |
| flat-topped wide block, cyclic prefix | OFDM (Wi-Fi, LTE, DVB) | out of scope here, see the domain skill |
PSK check on one burst `seg` (numpy): `np.argmax(abs(np.fft.fftshift(np.fft.fft(seg**2))))` gives a
sharp line for BPSK; use `seg**4` for QPSK. Save as `scripts/fskinfo.py`:
```python
#!/usr/bin/env python3
"""fskinfo.py <file> <cs8|cu8|cs16|cf32> <rate> <start_s> <dur_ms>: 2-FSK tones, deviation, symbol rate of one burst."""
import sys
import numpy as np
p, fmt, rate, t0, dur = sys.argv[1], sys.argv[2], float(sys.argv[3]), float(sys.argv[4]), float(sys.argv[5])
bps = {"cs8": 2, "cu8": 2, "cs16": 4, "cf32": 8}[fmt]
s, n = int(t0 * rate), int(dur * rate / 1e3)
v = np.fromfile(p, dtype={"cs8": np.int8, "cu8": np.uint8, "cs16": "<i2", "cf32": "<f4"}[fmt],
                count=2 * n, offset=s * bps).astype(np.float32)
if fmt == "cu8":
    v -= 127.5
x = v[0::2] + 1j * v[1::2]
f = np.convolve(np.angle(x[1:] * np.conj(x[:-1])) * rate / (2 * np.pi), np.ones(5) / 5, mode="same")
mid = np.median(f)
lo, hi = np.median(f[f < mid]), np.median(f[f >= mid])
runs = np.diff(np.flatnonzero(np.diff((f > (lo + hi) / 2).astype(np.int8))))
runs = runs[runs > 2]
sym = np.percentile(runs, 10) if runs.size else float("nan")
print(f"tones {lo:.0f} / {hi:.0f} Hz from centre, deviation +/-{(hi - lo) / 2:.0f} Hz, "
      f"shortest run ~{sym:.1f} samples -> ~{rate / sym:.0f} symbols/s (runs should be multiples of it)")
```
Take `start_s`/`dur_ms` inside a burst from Phase 1. Modulation index h = 2 x deviation / symbol rate.
With Manchester the shortest run is half a bit, so the bit rate is half the printed figure.

## Phase 4: demodulate and slice
1. Known device? `rtl_433 -r captures/<f>.cs8 -F json:logs/<f>_rtl433.json -M bits` runs every
   built-in decoder. rtl_433 takes rate and centre from name tokens (`433.42M`, `2Msps`); check its
   start-up lines and add `-s <rate>` if needed. FSK files may need `-Y minmax` or `-Y classic`.
2. Pulse analyzer: `rtl_433 -R 0 -A -r captures/<f>.cs8 | tee logs/<f>_pulses.txt` prints pulse,
   gap and period distributions, a modulation guess and a ready `-X` line.
3. Flex decoder with those timings (microseconds; for PWM `s` is the "1" pulse, `l` the "0" pulse):
   `rtl_433 -R 0 -X 'n=target,m=OOK_PWM,s=400,l=1200,r=10000,g=2000,t=200,bits>=24' -r captures/<f>.cs8 -F json:logs/<f>_flex.json`.
   Modulations: `OOK_PWM`, `OOK_PPM`, `OOK_PCM`, `OOK_RZ`, `OOK_MC_ZEROBIT`, `OOK_DMC`, `FSK_PCM`,
   `FSK_PWM`, `FSK_MC_ZEROBIT`; options `invert`, `reflect`, `decode_mc`, `decode_dm`,
   `preamble={16}0xaaaa`, `match=`, `repeats>=3`, `unique` (`rtl_433 -X help`).
4. Live demodulation with a radio uses the GUI's parameters: `/opt/urh-venv/bin/urh_cli -d RTL-SDR
   -f 433.92e6 -s 1e6 -g 30 -mo ASK -sps <samples/symbol> -c <centre> -t <tolerance> -rx -rt 10
   -file logs/urh_rx.txt` (RX; `-file` stores messages, it does not read IQ files).
5. Headless GNU Radio for anything custom: build the flowgraph (operator in GRC, or Python), compile
   with `grcc -o scripts/ flowgraph.grc`, run the generated `.py`; file source in, file sink out.

## Phase 5: line code and framing
- PWM: fixed period, the high time carries the bit (very common in remotes: Te of 200-500 us, "0" and
  "1" as 1:3 and 3:1). PPM: fixed pulse, the gap carries the bit. PCM/NRZ: bit time = shortest
  pulse, runs are multiples. Manchester: no run longer than two half-bits; try both polarities
  (IEEE vs Thomas) and a one-half-bit shift. Differential Manchester: a transition at every bit start.
- Frame: preamble (alternating bits or a train of short pulses), sync word (vendor-specific, often
  16 bits such as 0x2DD4 or 0xD391 on CC1101-class radios), optional length byte, payload, check value;
  some radios whiten data (PN9) or send LSB first (KeeLoq does): try `reflect` and inverted bits.
- Repeats inside one transmission (3-20 copies) must decode identically; disagreement means slicing
  errors, fix thresholds before interpreting fields. Align frames on the sync word, not on the first
  bit (receivers lose preamble bits while their gain settles).
- Check value search (save as `scripts/crcfind.py`; frames as hex without preamble/sync):
```python
#!/usr/bin/env python3
"""crcfind.py <frames.txt> [skip_bytes=0]: hex frames, one per line ('#' comments), check value in the last 1 or 2
bytes. Tries sum8/xor8, every CRC-8 polynomial and common CRC-16s; prints parameter sets valid for ALL frames."""
import sys
from functools import reduce
skip = int(sys.argv[2]) if len(sys.argv) > 2 else 0
frames = [bytes.fromhex(t.removeprefix("0x"))[skip:] for t in
          (l.split("#")[0].strip().replace(" ", "") for l in open(sys.argv[1])) if t]
rev = lambda v, w: int(f"{v:0{w}b}"[::-1], 2)

def crc(data, w, poly, init, ref):
    top, mask, c = 1 << (w - 1), (1 << w) - 1, init
    for b in data:
        c ^= (rev(b, 8) if ref else b) << (w - 8)
        for _ in range(8):
            c = ((c << 1) ^ poly) & mask if c & top else (c << 1) & mask
    return rev(c, w) if ref else c

def ok(fn, k):
    return all(fn(f[:-k]) == int.from_bytes(f[-k:], "big") for f in frames)

if ok(lambda d: sum(d) & 0xFF, 1): print("sum8 of bytes")
if ok(lambda d: reduce(lambda a, b: a ^ b, d, 0), 1): print("xor8 of bytes")
for w, polys, inits in ((8, range(1, 256), (0, 0xFF)),
                        (16, (0x1021, 0x8005, 0x3D65, 0x8BB7, 0x0589, 0xA02B, 0xC867, 0x5935), (0, 0xFFFF, 0x1D0F))):
    for poly in polys:
        for init in inits:
            for ref in (False, True):
                for xo in (0, (1 << w) - 1):
                    if ok(lambda d: crc(d, w, poly, init, ref) ^ xo, w // 8):
                        print(f"CRC-{w} poly=0x{poly:0{w // 4}X} init=0x{init:0{w // 4}X} reflected={ref} xorout=0x{xo:0{w // 4}X}")
print(f"tested {len(frames)} frames (skip {skip} leading bytes); need 3+ distinct frames to trust a hit")
```
  No hit: vary `skip_bytes`, try the frame bit-reflected, a little-endian check value, or a check that
  covers only part of the frame; a byte that changes randomly with everything else may be a MAC.

## Phase 6: compare captures
Collect at least 3 frames per action (same button pressed repeatedly) and frames of other actions
(other buttons, other device, state changes). Save as `scripts/bitdiff.py`:
```python
#!/usr/bin/env python3
"""bitdiff.py <frames.txt>: one frame per line, bits (0101...) or hex (0x...); '#' starts a comment.
Prints the aligned frames, a variability ruler (^ = position changes) and bit flips between neighbours."""
import sys
frames = []
for line in open(sys.argv[1]):
    t = line.split("#")[0].strip()
    if t:
        frames.append(bin(int(t, 16))[2:].zfill(4 * (len(t) - 2)) if t.lower().startswith("0x") else t)
n = min(map(len, frames))
ruler = "".join("^" if len({f[i] for f in frames}) > 1 else "." for i in range(n))
for f in frames:
    print(f[:n])
print(ruler)
flips = [sum(a != b for a, b in zip(f1[:n], f2[:n])) for f1, f2 in zip(frames, frames[1:])]
print(f"{len(frames)} frames, {n} bits compared, {ruler.count('^')} positions vary, flips between neighbours: {flips}")
```
Reading: no position varies between presses = static code (replayable; prove it in
`rfswift-subghz-ism`). A short field that changes only with the button = button code. A field that
increases by one per press = counter in clear. A block where about half the bits flip on every press
= encrypted/hopping code (rolling code). The trailing byte(s) that change whenever anything changes =
check value (Phase 5). A constant field shared by all buttons = device ID/serial.

## Evidence
- `logs/<base>_bursts.csv`, `_pulses.txt`, `_rtl433.json`/`_flex.json`, `_frames.txt` (input to
  bitdiff/crcfind), and `logs/<base>_frames.csv` as the decoded-frame table:
  `capture,burst_idx,start_s,f_off_hz,modulation,symbol_rate,encoding,frame_hex,field_id,field_button,field_counter,check,check_ok,notes`.
- Mission note (`write_note`, append): parameters (modulation, Te or symbol rate, deviation, line code,
  preamble, sync, bit order, check algorithm), the exact rtl_433/urh/python commands, a field map, and
  which captures each conclusion rests on. Credentials go to `save_secret`, not the note.

## Typical findings
Save with `save_finding` (stored with status `draft`; only the operator confirms). Analysis alone gives candidates;
confirmation of replay/forgery needs the TX test in the domain skill.
- "Static (non-rolling) RF code": candidate from identical frames across 3+ presses at different times;
  confirmed only by a replay that actuates the target. `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N` (6.5).
- "Unauthenticated RF protocol, integrity by CRC only" (forgeable frames): confirmed when a crafted
  frame with a recomputed CRC is accepted. Same vector; I:L if only cosmetic state changes.
- "Rolling counter sent in clear without cryptographic authentication": candidate when the counter is
  readable and the check is a CRC/sum.
- "Sensitive data in cleartext" (keypad PIN, alarm state, meter readings, identifiers enabling
  tracking): `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N` (4.3), C:H for credentials.
Remediation patterns: authenticated encryption with per-device keys (AES-CCM/GCM or vendor rolling
codes with strong crypto), replay counters checked by the receiver, challenge-response for access
functions, no secrets or tracking identifiers in clear.

## Pitfalls
- Wrong sample rate scales every timing; wrong format (cu8 vs cs8) yields noise. Re-check Phase 0.
- Clipped captures produce flat tops and false FSK runs; noisy captures produce split pulses: fix the
  capture rather than tuning thresholds forever.
- Two devices in one file decode as one garbled protocol: separate by offset and level first.
- Manchester phase, inverted polarity and LSB-first order produce consistent but wrong values; prove a
  field mapping with a controlled change (press another button, change a setting) and watch only that
  field move.
- An apparent counter may be a timestamp or a sequence that resets on battery removal; test it.
- Do not overstate: "looks random" is a hypothesis about encryption until a replay test says more.
