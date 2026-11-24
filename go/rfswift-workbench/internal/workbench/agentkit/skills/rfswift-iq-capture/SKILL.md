---
name: rfswift-iq-capture
description: Record IQ samples correctly and document them as evidence in an RF Swift mission. Per-device capture commands (hackrf_transfer -r -f -s -n -l -g -a -b, rtl_sdr, UHD rx_samples_to_file and uhd_rx_cfile, bladeRF-cli rx config scripts, airspy_rx, hydrasdr_rx, a headless GNU Radio gr-soapy flowgraph for LimeSDR, PlutoSDR or any SoapySDR device, rtl_433 -S signal grabbing), sample formats cs8/cu8/cs16/cf32 and conversions with numpy or sox, file sizes and timed captures (-n sample counts, max_seconds), off-centre tuning around the DC spike, level and clipping checks, SigMF metadata (.sigmf-meta with core:datatype, core:sample_rate, core:frequency, core:datetime, core:hw, core:dataset), naming convention, sha256sum checksums and the mission note. Use for record, capture, IQ file, raw samples, cfile, recording a remote, keyfob, sensor or beacon.
---
# IQ capture and documentation

## Scope
How to record raw IQ that someone else can re-open and trust: right centre, rate, gain and length,
a self-describing file name, SigMF metadata, a checksum, and a note entry. Prerequisites: a validated
radio and its ppm (`rfswift-sdr-setup`), a known target frequency (`rfswift-spectrum-survey`).
Not covered: decoding (`rfswift-signal-analysis`), replay/transmission of captures
(`rfswift-subghz-ism`, `rfswift-gnss`), protocol sniffers that write pcap instead of IQ
(`rfswift-iot-lpwan-mesh`, `rfswift-bluetooth-ble`, `rfswift-wifi-assessment`).

## Ground rules
- Capturing is receive-only. Never add a TX option to a capture command (`hackrf_transfer -t`, `-c`,
  `-R`, UHD/bladeRF `tx`, a GNU Radio sink). Replaying a capture is TX and belongs to the domain skill,
  behind the full gate: operator enabled "Allow RF transmission", explicit OK in chat for that
  transmission, `transmits: true`, and frequency, gain, duration and target in `purpose`; licensed
  bands only conducted or in a shielded enclosure.
- A capture also records everything else in its bandwidth. Treat files as evidence: keep them in the
  workspace, do not copy them elsewhere, do not demodulate third-party content found in them.
- Disk: 20 Msps cs8 is 2.4 GB per minute. Check `df -h .` first; ask the operator before any capture
  above about 2 GB or longer than 5 minutes.
- Captures longer than `wait_seconds` continue as background jobs: always bound them by a sample
  count (`-n`) and set `max_seconds` a little above the expected duration as a safety net.

## Tools in RF Swift
| Tool | Writes | Docker image | Nix env |
|---|---|---|---|
| `hackrf_transfer -r` | cs8 | sdr_light, sdr_full, sdr_hacks | sdr_light, sdr_full, sdr_hacks |
| `rtl_sdr` | cu8 | sdr_light+ | sdr_* |
| `/usr/libexec/uhd/examples/rx_samples_to_file`, `uhd_rx_cfile` | cs16 (`--type short`) or cf32 | sdr_light+ | sdr_*, telecom* (path differs: `find / -name rx_samples_to_file`) |
| `bladeRF-cli` (`rx config`) | SC16 Q11 binary (cs16, full scale 2048) | sdr_light+ | sdr_*, telecom |
| `airspy_rx`, `hydrasdr_rx` | cs16 (`-t 2`) or cf32 (`-t 0`) | sdr_light+ | sdr_* |
| GNU Radio (`from gnuradio import soapy, blocks`) | cf32 | sdr_light+ | sdr_* (gnuradio-rfswift) |
| `rtl_433 -S`, `-w` | cu8 per detected signal / stream | sdr_light+ | sdr_* |
| `python3` + numpy | checks, conversions, SigMF | every image | python3 everywhere, numpy listed only in sdr_full: check |
| `sox` | raw format conversion | every image | not shipped |
| `sha256sum` | checksums | everywhere | everywhere |
Not shipped by RF Swift: the `sigmf` Python package and SigMF CLI tools (write the JSON with the
script below; SDRangel in sdr_full can also record SigMF from its GUI), `csdr`, `rx_tools`/`rx_sdr`.

## Formats
| Ext | Layout | Bytes per sample | Produced by | SigMF `core:datatype` | Full scale |
|---|---|---|---|---|---|
| `.cs8` | int8 I, int8 Q | 2 | hackrf_transfer | `ci8` | 128 |
| `.cu8` | uint8 I, uint8 Q (zero at 127.5) | 2 | rtl_sdr, rtl_433 | `cu8` | 127.5 |
| `.cs16` | int16 LE I, Q | 4 | UHD `--type short`, bladeRF bin, airspy `-t 2`, Pluto | `ci16_le` | 32768 (bladeRF and Pluto 12-bit: 2048) |
| `.cf32` | float32 LE I, Q | 8 | GNU Radio, UHD `--type float` | `cf32_le` | 1.0 |
Size = rate x bytes per sample x seconds: 2 Msps cs8 = 4 MB/s, 2.4 Msps cu8 = 4.8 MB/s,
10 Msps cs8 = 20 MB/s, 4 Msps cs16 = 16 MB/s, 2 Msps cf32 = 16 MB/s.
Tools recognise the extension: rtl_433 reads `.cu8`, `.cs8`, `.cs16`, `.cf32` (it logs cs8 as format
"Unknown" but decodes it correctly) and takes the centre and rate from name tokens such as `433.42M`
and `2Msps`; inspectrum accepts the same extensions plus `.sigmf-meta`. The Workbench Artifacts panel
auto-classifies `.cs8`, `.cs16`, `.cf32`, `.cfile`, `.iq`, `.sigmf-data` as IQ, but not `.cu8` (the
operator can add a custom capture type).

## Phase 1: plan
- Centre: tune off the target so it does not sit on the DC spike and LO leakage: centre = target -
  offset, with offset about 200-500 kHz and the whole signal inside +/-0.4 x rate. Example: target
  433.92 MHz, 2 Msps, tune 433.42 MHz, the signal appears at +500 kHz. UHD can do this internally with
  `--lo-offset`.
- Rate: at least 1.25-2 x the occupied bandwidth plus twice the offset. HackRF needs 2-20 Msps (its
  docs advise 8 Msps or more for best quality; 2-4 Msps is common for narrow sub-GHz signals), RTL-SDR
  up to 2.4 Msps, never above the drop-free rate measured during setup.
- Length: samples = rate x seconds. Agree a timeline with the operator: "capture starts, wait 2 s,
  press button A five times one second apart". Start recording first, then ask for the action.
- Gain: start from the setup values; verify with a 2 s test capture and the level check below.
- Name: `captures/<tuned centre>M_<rate>sps_<UTC start>_<label>.<ext>`, e.g.
  `captures/433.42M_2Msps_2026-11-24T10-05Z_gate-btnA.cs8`. The frequency in the name is the tuned
  centre (what tools need); the target frequency goes in the SigMF description and the note.
  Labels: lowercase, hyphens, what and which action (`fob1-unlock-x5`, `tpms-fl-drive`).

## Phase 2: capture commands
Set the variables in the same command as the capture (each `execute_command` is a fresh shell):
```sh
mkdir -p captures logs; TS=$(date -u +%Y-%m-%dT%H-%MZ); T0=$(date -u +%Y-%m-%dT%H:%M:%SZ)
# HackRF: 10 s at 2 Msps, 1.75 MHz baseband filter, amp off, LNA 16, VGA 20
hackrf_transfer -r captures/433.42M_2Msps_${TS}_gate-btnA.cs8 -f 433420000 -s 2000000 -b 1750000 \
  -n 20000000 -a 0 -l 16 -g 20
# RTL-SDR: 10 s at 2.048 Msps, gain 30 dB, ppm correction from setup
rtl_sdr -f 433.42M -s 2.048M -g 30 -p 0 -n 20480000 captures/433.42M_2.048Msps_${TS}_sensor.cu8
# USRP (examples are not on PATH); --lo-offset moves the DC spike out of the way
/usr/libexec/uhd/examples/rx_samples_to_file --args "type=b200" --freq 433.92e6 --lo-offset 500e3 \
  --rate 2e6 --gain 35 --ant RX2 --nsamps 20000000 --type short --file captures/433.92M_2Msps_${TS}_x.cs16
# Airspy R2 (-f in MHz; rates per airspy_info, e.g. 2500000 or 10000000 on R2, 3000000/6000000 on Mini)
airspy_rx -f 433.42 -a 2500000 -t 2 -l 10 -m 8 -v 8 -n 25000000 -r captures/433.42M_2.5Msps_${TS}_x.cs16
```
`hydrasdr_rx` takes airspy-style options (`-r -f -a -t -n` and device-specific gains: run it with an
invalid option to print its usage). `uhd_rx_cfile -f 433.92e6 -r 2e6 -g 35 -N 20000000 -s out.cs16`
is the GNU Radio alternative (`-s` = shorts, otherwise cf32).
bladeRF: write a script so the sequence is explicit, then run it:
```sh
cat > logs/bladerf_rx_${TS}.txt <<EOF
set frequency rx 433.42M
set samplerate rx 2M
set bandwidth rx 1.5M
set gain rx 30
rx config file=captures/433.42M_2Msps_${TS}_x.cs16 format=bin n=20000000
rx start
rx wait
EOF
bladeRF-cli -s logs/bladerf_rx_${TS}.txt
```
Channel and gain syntax differ between bladeRF 1 and 2.0 (`rx1`, AGC): print the exact form with
`bladeRF-cli -e 'set gain'` and `bladeRF-cli -e 'set agc'` first. `rx wait` is required in scripts.
LimeSDR, PlutoSDR or any SoapySDR device: a headless GNU Radio 3.10 flowgraph (save as
`scripts/soapy_capture.py`; get the device string from `SoapySDRUtil --find`, gain ranges from
`--probe`):
```python
#!/usr/bin/env python3
"""soapy_capture.py <device> <freq_hz> <rate> <gain_db> <nsamps> <out.cf32>
e.g. driver=lime | driver=plutosdr,hostname=192.168.2.1 | driver=rtlsdr"""
import sys
from gnuradio import blocks, gr, soapy
dev, freq, rate, gain, n, out = sys.argv[1], *map(float, sys.argv[2:6]), sys.argv[6]
tb = gr.top_block()
src = soapy.source(dev, "fc32", 1, "", "", [""], [""])
src.set_sample_rate(0, rate)
src.set_frequency(0, freq)
src.set_gain_mode(0, False)
src.set_gain(0, gain)
tb.connect(src, blocks.head(gr.sizeof_gr_complex, int(n)), blocks.file_sink(gr.sizeof_gr_complex, out, False))
tb.run()
```
Grabbing rare bursts without a long file: `cd captures && rtl_433 -f 433.92M -s 1024k -S unknown -T 600`
saves one `gNNN_433.92M_1024k.cu8` per undecoded signal (use `-S all` to keep decoded ones too). Rename
them into the convention afterwards and keep rtl_433's console output in `logs/`.

## Phase 3: verify the capture
Check that the size matches (samples x bytes); a short file means drops or an early kill. Then run the
level check (save as `scripts/iqcheck.py`):
```python
#!/usr/bin/env python3
"""iqcheck.py <file> <cs8|cu8|cs16|cf32> <rate> [full_scale]: duration, RMS/peak dBFS, clipping, DC."""
import os, sys
import numpy as np
p, fmt, rate = sys.argv[1], sys.argv[2], float(sys.argv[3])
dt = {"cs8": np.int8, "cu8": np.uint8, "cs16": "<i2", "cf32": "<f4"}[fmt]
fs = float(sys.argv[4]) if len(sys.argv) > 4 else {"cs8": 128, "cu8": 127.5, "cs16": 32768, "cf32": 1.0}[fmt]
raw = np.memmap(p, dtype=dt, mode="r")
n = raw.size // 2
v = raw[: n * 2].reshape(-1, 2)[:: max(1, n // 4_000_000)].astype(np.float32)  # subsample big files
if fmt == "cu8":
    v -= 127.5
x = (v[:, 0] + 1j * v[:, 1]) / fs
db = lambda a: 20 * np.log10(a + 1e-12)
print(f"{os.path.basename(p)}: {n} samples = {n / rate:.2f} s | RMS {db(np.sqrt(np.mean(abs(x) ** 2))):.1f} dBFS"
      f" | peak {db(abs(x).max()):.1f} dBFS | clipped {np.mean(abs(v) >= 0.98 * fs) * 100:.3f} %"
      f" | DC {db(abs(x.mean())):.1f} dBFS")
```
Pass full scale 2048 for bladeRF and Pluto cs16. Decide: clipped above about 0.01 % or peak near
0 dBFS means re-capture with less gain; target bursts below about -40 dBFS mean more gain or a closer
antenna; an 8-bit noise floor much below -45 dBFS RMS means quantisation dominates (raise gain).
Then confirm the bursts are there (burst list in `rfswift-signal-analysis`); the operator can look with
`inspectrum -r 2000000 captures/<file>.cs8`.

## Phase 4: conversions and cuts
Never overwrite an original; derived files get a suffix (`_cut`, `_cf32`) and their own metadata.
```sh
python3 - captures/in.cu8 captures/in_cf32.cf32 <<'EOF'
import sys, numpy as np
x = np.fromfile(sys.argv[1], np.uint8).astype(np.float32)
((x - 127.5) / 127.5).tofile(sys.argv[2])
EOF
```
Other one-liners: cs8 to cf32 `np.fromfile(f, np.int8).astype(np.float32) / 128`; cs16 to cf32
`np.fromfile(f, "<i2").astype(np.float32) / 32768` (2048 for bladeRF/Pluto); cu8 to cs8
`(np.fromfile(f, np.uint8).astype(np.int16) - 128).astype(np.int8)`; cf32 to cs8 (HackRF TX files)
`np.clip(np.round(x * 127), -127, 127).astype(np.int8)`.
sox (Docker images): `sox -t raw -e signed-integer -b 8 -c 2 -r 2000000 in.cs8 -t raw -e floating-point -b 32 -c 2 out.cf32`;
a WAV baseband from SDR++ (`sdrpp` recorder): `sox in.wav -t raw -e floating-point -b 32 out.cf32`.
Cut samples [start, start+count) with GNU dd, B = bytes per sample:
`dd if=in.cs8 of=in_cut.cs8 iflag=skip_bytes,count_bytes bs=1M skip=$((start*B)) count=$((count*B))`.
Filtering and decimation: GNU Radio or scipy.signal (present when GNU Radio is; check `import scipy`).

## Phase 5: SigMF metadata
Write `<same base>.sigmf-meta` next to every capture. The data file keeps its tool-friendly extension
and is referenced with `core:dataset` (SigMF non-conforming dataset). Save as `scripts/sigmf_meta.py`:
```python
#!/usr/bin/env python3
"""sigmf_meta.py <data> <cs8|cu8|cs16|cf32> <rate> <center_hz> <utc_iso> <hw> [description]
Writes <data without extension>.sigmf-meta (SigMF 1.x; the data file keeps its name via core:dataset)."""
import hashlib, json, os, sys
data, fmt, rate, freq, when, hw = sys.argv[1:7]
h = hashlib.sha512()
with open(data, "rb") as f:
    for chunk in iter(lambda: f.read(1 << 20), b""):
        h.update(chunk)
meta = {
    "global": {
        "core:datatype": {"cs8": "ci8", "cu8": "cu8", "cs16": "ci16_le", "cf32": "cf32_le"}[fmt],
        "core:sample_rate": float(rate), "core:version": "1.2.0", "core:num_channels": 1,
        "core:dataset": os.path.basename(data), "core:sha512": h.hexdigest(),
        "core:hw": hw, "core:recorder": "RF Swift mission",
        "core:description": sys.argv[7] if len(sys.argv) > 7 else "",
    },
    "captures": [{"core:sample_start": 0, "core:frequency": float(freq), "core:datetime": when}],
    "annotations": [],
}
out = os.path.splitext(data)[0] + ".sigmf-meta"
with open(out, "w") as f:
    json.dump(meta, f, indent=2)
print(out)
```
```sh
python3 scripts/sigmf_meta.py captures/433.42M_2Msps_2026-11-24T10-05Z_gate-btnA.cs8 cs8 2000000 433420000 \
  2026-11-24T10:05:12Z "HackRF One SN <serial>, fw <version>, amp 0 LNA 16 VGA 20, ppm +3.1 (GPSDO), 433 MHz whip at 1 m" \
  "Gate remote <model>, button A x5 from T+2 s; target 433.92 MHz (+500 kHz in file)"
```
`core:datetime` is the UTC start (`T0` taken just before the capture), not the file's mtime. Add
burst annotations later (`core:sample_start`, `core:sample_count`, `core:freq_lower_edge`,
`core:freq_upper_edge`, `core:label`) by loading and re-dumping the JSON. A tool that insists on a
conforming pair can get a symlink `<base>.sigmf-data -> <base>.cs8`. Older SigMF readers may expect
`"core:version": "1.0.0"`.

## Phase 6: checksum and note
```sh
sha256sum captures/433.42M_2Msps_2026-11-24T10-05Z_gate-btnA.cs8 \
  captures/433.42M_2Msps_2026-11-24T10-05Z_gate-btnA.sigmf-meta | tee -a captures/SHA256SUMS
```
Append to the mission note (`write_note`, `append: true`; `read_note` first to find the note in use):
```
### Capture 2026-11-24T10:05:12Z gate-btnA
- files: captures/433.42M_2Msps_2026-11-24T10-05Z_gate-btnA.cs8 (+ .sigmf-meta), sha256 in captures/SHA256SUMS
- command: <exact command line>
- radio: <model, serial, firmware>, gains <...>, ppm <value + method>, antenna <type, distance>
- target: <device>, action <button A x5, T+2 s..T+6 s, performed by operator>
- check: 10.00 s, peak -12 dBFS, clipped 0.000 %, 5 bursts at +500 kHz
```
Ask the operator to register important captures in the Artifacts panel.

## Findings
Captures are evidence for findings raised in other skills (fixed codes, cleartext telemetry, GNSS
behaviour); they are not findings by themselves. A finding should cite the capture file name and its
sha256. Keep the chain intact: hash at capture time, never edit originals, document every derived file.

## Pitfalls
- Starting the capture after the event: start recording first, then ask the operator to act.
- Format confusion: reading cu8 as cs8 (or the reverse) produces a huge DC term and garbage. The
  extension must match the producing tool; rtl_433 decodes wrongly if you force `cu8:` on cs8 data.
- Disk full or `max_seconds` kills leave short files: compare size with the expected sample count and
  record the real duration.
- HackRF amp (`-a 1`) a metre from a keyfob clips; 8-bit radios need modest gain on strong signals.
- Off-centre sign errors: centre = target - offset puts the signal at +offset in the file. Write it in
  the SigMF description.
- Two radios recording the same event need distinct labels (`_hackrf`, `_rtl`) in the name.
- Local time in names or metadata: always UTC with a `Z`.
- One program per radio: stop live `rtl_433` or a GUI before recording with the same device.
