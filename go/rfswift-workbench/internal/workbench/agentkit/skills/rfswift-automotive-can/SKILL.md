---
name: rfswift-automotive-can
description: In-vehicle networks with RF Swift: SocketCAN setup (ip link bitrate, listen-only, restart-ms, CAN FD dbitrate, vcan for offline replay), adapters (CANable/candleLight gs_usb, slcan/LAWICEL with slcand, PEAK PCAN-USB, Kvaser, 8devices USB2CAN, CANtact), can-utils (candump, cansniffer, canbusload, canplayer, cansend, cangen, isotpsend/isotprecv/isotpdump), SavvyCAN, ICSim, cantools with DBC files, python-can, caringcaribou, gallia, udsoncan, doipclient (DoIP 13400), ISO-TP and UDS diagnostics (diagnostic session control 0x10, security access 0x27 seed/key, ReadDataByIdentifier 0x22), OBD-II PIDs on 0x7DF/0x7E8, J1939 notes, bench harness and vehicle safety rules. Use for CAN bus sniffing, ECU diagnostics, replay, gateway filtering and seed/key analysis. Key fobs and TPMS are radio: see rfswift-subghz-ism.
---
# Automotive networks: CAN, ISO-TP, UDS, OBD-II

## Scope
- Covers: CAN and CAN FD through Linux SocketCAN, passive sniffing and reverse engineering of frames, ISO-TP transport, UDS and OBD-II diagnostics, DoIP (diagnostics over Ethernet) at a high level, V2G charging-protocol tooling as a pointer.
- Not covered: keyless entry, remote keyless (RKE), TPMS and other vehicle radios (`rfswift-subghz-ism`; replaying a key fob is RF transmission under the TX rule), telematics/cellular units (`rfswift-cellular-lab`), ECU firmware (`rfswift-firmware-analysis`), ECU debug ports and flash chips (`rfswift-hardware-interfaces`).

## Ground rules and safety
- Authorisation: the vehicle, ECU or bench harness belongs to the client or is explicitly in scope, with written rules of engagement for live vehicles (who may drive, where, which ECUs are off-limits).
- Passive work (listen-only sniffing, logging, offline decoding, replay into `vcan`) needs no further OK.
- Anything that puts a frame on a real bus needs the operator's explicit OK in chat for that action:
  - `cansend`, `cangen`, `canplayer` to a real interface, `isotpsend`, SavvyCAN "send" or playback;
  - any UDS or OBD-II request, even tester present or a session change;
  - caringcaribou modules other than `listener`/`dump`, gallia scans, DoIP requests.
- Where active work happens: preferably on a bench harness (ECU, power supply, terminators) or `vcan`. On a vehicle:
  - stationary, gear in P/neutral, parking brake on, nobody in moving-part areas, engine off unless the test needs it;
  - a battery maintainer for long sessions, the operator present and able to cut power;
  - never transmit on a moving vehicle.
- Extra care on safety-relevant ECUs (brakes/ABS/ESC, airbag/SRS, steering/EPS, ADAS, powertrain, battery management). Each of these UDS services needs its own OK naming the ECU:
  - ECUReset `0x11`, ClearDiagnosticInformation `0x14`, CommunicationControl `0x28`, ControlDTCSetting `0x85`;
  - WriteDataByIdentifier `0x2E`, InputOutputControlByIdentifier `0x2F`, RoutineControl `0x31`, RequestDownload/TransferData `0x34/0x36`, WriteMemoryByAddress `0x3D`;
  - the programming session `0x10 0x02` (it can take an ECU off the bus).
- Fuzzing (random `cangen`, caringcaribou fuzzer, gallia fuzzing) and seed/key brute force: bench only, explicit OK, and only after the operator accepts lockout or ECU-recovery risk.
- Joining a live bus at the wrong bitrate creates error frames that can push ECUs to error-passive or bus-off and set DTCs: discover the bitrate in listen-only mode first.

## When to stop and ask the operator
- Before the first frame you would send on a real interface, and again whenever the target ECU, service or vehicle state changes.
- When you need a physical action: plugging the adapter, ignition on/off, pressing a button for correlation, power-cycling an ECU.
- When the interface type is unknown, the adapter is missing, or `ip link` needs privileges you do not have.
- When the bus shows error frames, an ECU stops answering, warning lights or DTCs appear: stop transmitting and report.

## Tools in RF Swift
- Docker image `automotive` (profile `automotive`: host network, `NET_ADMIN`+`NET_RAW`, `/dev/bus/usb`):
  - can-utils: `candump`, `cansend`, `cangen`, `canplayer`, `cansniffer`, `canbusload`, `isotpsend`, `isotprecv`, `isotpdump`, `isotpsniffer`, `slcand`, `log2asc`/`asc2log`, J1939 tools;
  - Python: python-can (`python3 -m can.logger`, `can.viewer`, `can.player`), cantools, udsoncan, doipclient, scapy (automotive layers);
  - frameworks and GUIs: caringcaribou (`caringcaribou`), gallia (x86_64/aarch64), SavvyCAN (`SavvyCAN`), ICSim (`icsim-setup-vcan`, `icsim`, `icsim-controls`), V2GInjector (EV charging, HomePlug GP), the CANtact Rust CLI (in `~/.cargo/bin`);
  - base: tcpdump, iproute2 (`ip`).
- Nix environment `automotive`: can-utils, savvycan, python-can, cantools, gallia, socketcand, caringcaribou, cantact, v2ginjector, wireshark, udsoncan, doipclient, icsim.
- Not shipped / not RF Swift's job: kernel drivers and modules (`can`, `can_raw`, `can_dev`, `can_isotp`, `vcan`, `gs_usb`, `slcan`, `peak_usb`, `kvaser_usb`, `usb_8dev`) come from the host kernel; vendor GUIs (PCAN-View, Kvaser CANKing, Vector tools), BUSMASTER and Kayak are not shipped; Wireshark is not in the Docker `automotive` image (capture with `tcpdump`, open the pcap in the operator's Wireshark or the Nix env).
- Check:
```sh
for t in ip candump cansend cansniffer canplayer cangen canbusload isotpsend isotprecv isotpdump slcand caringcaribou gallia cantools SavvyCAN savvycan socketcand icsim tcpdump; do printf '%-14s %s\n' "$t" "$(command -v $t || echo -)"; done
python3 -c 'import can, cantools; print("python-can", can.__version__)'
ip -details link show type can 2>/dev/null; ip link show | grep -E 'can|slcan|vcan'
lsmod 2>/dev/null | grep -E '^(can|vcan|gs_usb|slcan|peak_usb|kvaser_usb|usb_8dev|can_isotp)'
ls -l /dev/ttyACM* /dev/ttyUSB* 2>/dev/null; lsusb
```
- Adapters:
  - candleLight firmware (CANable, CANable 2.0, many USB2CAN clones): `gs_usb` driver, appears directly as `can0`.
  - slcan/LAWICEL serial firmware (CANable slcan, original CANtact, CANUSB): a `/dev/ttyACM*` port plus `slcand`; classic CAN only.
  - PEAK PCAN-USB (`peak_usb`), Kvaser USB (`kvaser_usb`), 8devices USB2CAN (`usb_8dev`): mainline drivers, appear as `canN`; FD variants need FD-capable firmware/driver.
  - ELM327 clones are not SocketCAN adapters; avoid them for security work.
- Interface plumbing in RF Swift:
  - the `automotive` profile uses the host network namespace with `NET_ADMIN`, so `ip link` inside the container configures the host's CAN interfaces (and other missions on the same host see them too);
  - containers cannot load kernel modules: when an interface type is unknown, ask the operator to `modprobe` it on the host;
  - slcan adapters need their serial node in the container: it appears on demand once plugged in (run a new command); otherwise ask the operator (rootless Podman maps ports at creation, macOS/Windows need "USB passthrough");
  - in Nix missions commands run on the host and `ip link set` needs root: ask the operator if `sudo` is not available;
  - SocketCAN needs a Linux host; Docker Desktop on macOS/Windows usually lacks the CAN modules in its VM.

## Workflow
### Phase 0: topology and bench preparation
- With the operator: where you connect (OBD-II pins 6/14 = high-speed CAN on most passenger cars; other pins are maker-specific), whether a gateway sits between the OBD port and internal buses (often only diagnostics pass), harness pin-outs, ECU part numbers, DBC files from the client.
- Bench: exactly two 120 ohm terminations (about 60 ohm between CAN-H and CAN-L with power off), stable supply, ignition/KL15 line handled by the operator.
- Typical bitrates: 500 kbit/s powertrain/chassis, 125 or 250 kbit/s body/comfort, 250/500 kbit/s J1939 trucks, CAN FD data phase 2 or 5 Mbit/s.

### Phase 1: offline practice and replay (no bus contact)
```sh
ip link add dev vcan0 type vcan && ip link set vcan0 up      # "Unknown device type": operator loads vcan on the host
icsim vcan0 & icsim-controls vcan0                           # GUI training target (operator desktop)
canplayer -I captures/can0_500k_bench_2026-11-24T10-05Z.log vcan0=can0
```
- Always give `canplayer` an explicit mapping to `vcan0` when replaying for analysis: without one it sends on the interface named inside the log, i.e. the real bus.

### Phase 2: bring the interface up passively
```sh
ip link set can0 down
ip link set can0 type can bitrate 500000 listen-only on restart-ms 100
ip link set can0 up
ip -details -statistics link show can0
```
- `listen-only on`: the controller never acknowledges or sends error frames, so you are invisible on the bus. On a single-ECU bench this means nobody ACKs and the ECU repeats frames: expected.
- Bitrate discovery: try 500000, 250000, 125000, 1000000 (and 83333/33333 for low-speed buses), each in listen-only for a few seconds. The right rate gives clean frames and the error counters in `ip -details -statistics` stay flat; a wrong rate gives bus errors and no frames.
- CAN FD: `ip link set can0 type can bitrate 500000 dbitrate 2000000 fd on` (adapter and driver must support FD).
- slcan adapters: `slcand -o -c -s6 /dev/ttyACM0 slcan0 && ip link set slcan0 up` (`-s0`..`-s8` = 10k, 20k, 50k, 100k, 125k, 250k, 500k, 800k, 1M; `-l` opens in listen-only on can-utils versions that support it, check `slcand -h`).

### Phase 3: passive capture and reverse engineering
```sh
candump -L can0 > captures/can0_500k_bench_2026-11-24T10-05Z.log            # log format, absolute timestamps
candump -L can0,700:700 > captures/can0_500k_diag_ids_2026-11-24T10-05Z.log   # filter id:mask (0x700-0x7FF)
tcpdump -i can0 -w captures/can0_500k_bench_2026-11-24T10-05Z.pcap            # Wireshark-readable
canbusload can0@500000 -t -b -r                                                # bus load (interactive display)
```
- Run captures as timed jobs (`max_seconds`) or background jobs and stop them with `cancel_job`. `candump -l can0` instead writes `candump-<date>_<time>.log` into the current directory.
- python-can alternative (other log formats such as ASC/BLF for client tools): `python3 -m can.logger -i socketcan -c can0 -f captures/can0_500k_<UTC>.asc`; options differ between python-can releases, check `--help`.
- Several readers can share one SocketCAN interface: keep a raw `candump -L` running for the whole session as the reference log while you use other tools.
- `cansniffer -c can0` shows changing bytes per ID: interactive, run it in a session and stop with `"\u0003"` (`enter: false`).
- Correlate: ask the operator to perform one action at a time (unlock, indicator, pedal, steering) and to say when; record the UTC time of each action in the note, then diff the logs around those times.
- With a DBC: `cantools dump vehicle.dbc`, `cantools decode --single-line vehicle.dbc < captures/<file>.log` (reads candump output formats). SavvyCAN (operator GUI) for graphs, flow view and DBC overlays; it can open candump logs.
- Diagnostic traffic markers: 11-bit OBD-II functional request `0x7DF`, physical requests `0x7E0`-`0x7E7`, responses `0x7E8`-`0x7EF`; 29-bit OBD `0x18DB33F1` (functional) and `0x18DAxxF1` (physical). J1939 uses 29-bit IDs carrying PGNs (can-utils `j1939*` tools).
- Watch passively for a workshop tester or the vehicle's own diagnostics: captured seed/key exchanges can be analysed offline without sending anything.

### Phase 4: active diagnostics (explicit OK per step)
- Turn listen-only off first: `ip link set can0 down && ip link set can0 type can bitrate 500000 listen-only off && ip link set can0 up`.
- Single frames: `cansend can0 7DF#02010C0000000000` (OBD-II mode 01 PID 0C, engine speed); CAN FD: `cansend can0 123##1DEADBEEF` (`##` plus a flags nibble).
- Targeted replay: extract a subset first (`grep ' 244#' captures/<file>.log > captures/replay_244_<UTC>.log`), state IDs, count and timing in `purpose`, then `canplayer -I captures/replay_244_<UTC>.log can0=can0`.
- ISO-TP request/response with the kernel ISO-TP sockets (`can_isotp` on the host), listener as a background job first:
```sh
isotprecv -s 7E0 -d 7E8 -l can0 > logs/uds_7E0_2026-11-24T10-05Z.log &
echo "10 03"    | isotpsend -s 7E0 -d 7E8 can0     # DiagnosticSessionControl: extended session
echo "22 F1 90" | isotpsend -s 7E0 -d 7E8 can0     # ReadDataByIdentifier: VIN
echo "3E 00"    | isotpsend -s 7E0 -d 7E8 can0     # TesterPresent (keeps a non-default session alive)
isotpdump -s 7E0 -d 7E8 -c -u -a can0              # passive ISO-TP view with UDS service names
```
  Some ECUs only answer padded 8-byte frames: enable padding with `-p` (format in `isotpsend -h`).
- Low-impact UDS reads: `0x10 0x01` default session, `0x22` with DIDs `F190` (VIN), `F18C` (ECU serial), `F187`/`F189`/`F195` (part and software versions), `0x19 0x02 <mask>` ReadDTCByStatusMask, `0x27 0x01` request seed (each wrong key counts towards a lockout; never send keys in a loop without OK).
- Discovery and scanning (more invasive, still explicit OK; check each module's `-h` because options change):
```sh
caringcaribou -i can0 listener            # passive
caringcaribou -i can0 uds discovery       # sends session requests across arbitration IDs
caringcaribou -i can0 uds services 0x7E0 0x7E8
caringcaribou -i can0 uds dump_dids 0x7E0 0x7E8
gallia --help                             # discover/scan/primitive commands and target URIs differ between gallia versions
```
- DoIP (Ethernet diagnostics, UDP/TCP 13400): `doipclient` (Python) for vehicle announcement, routing activation and UDS over DoIP; on the same network as the vehicle's DoIP interface; active.
- udsoncan scripts over kernel ISO-TP need the Python `isotp` package (`python3 -c 'import isotp'`); otherwise prefer can-utils, caringcaribou or gallia.
- Seed/key analysis: collect several seed/key pairs (passively from a legitimate tester, or seeds only with `27 01` under OK), look for repetition after reset, short seeds, time dependence, and simple relations (XOR/add constants, byte rotation) offline in Python. Only send a computed key with OK; a wrong key costs an attempt.
- EV charging (V2G, ISO 15118 over HomePlug Green PHY on the charging cable): V2GInjector is shipped; any injection on a charging session is active, needs an OK and the operator's electrical-safety judgement.
- After active work: return the ECU to the default session (`10 01`) or let it time out, note every DTC that may have been set, and tell the operator.

## Reading results
- Positive UDS response = request SID + 0x40 (`50` for `10`, `62` for `22`, `67` for `27`). Negative: `7F <SID> <NRC>`: `11` service not supported, `12` sub-function not supported, `13` wrong length/format, `22` conditions not correct, `31` request out of range, `33` security access denied, `35` invalid key, `36` exceeded attempts, `37` time delay not expired, `78` response pending (wait), `7E`/`7F` not supported in the active session.
- Nothing on candump: wrong pins or bitrate, missing termination, a gateway that keeps the OBD port silent until a diagnostic request, interface down or `BUS-OFF` (see `ip -details link show`; `restart-ms` or down/up recovers).
- `ERROR-PASSIVE` with listen-only on a single-node bench: no ACKs, expected. Rising bus errors in normal mode: wrong bitrate or a wiring fault: stop transmitting.
- Rolling counters and checksum bytes in frames explain why naive replay is rejected; that is not a vulnerability on its own. Message authentication (SecOC) shows as extra MAC/freshness bytes.
- Periodic frames at fixed rates are status/heartbeat traffic; event frames appear only around an action.

## Evidence
- `captures/`: `can0_<bitrate>_<location-or-ecu>_<UTC>.log` (candump `-L` format) and `.pcap`, replay subsets `replay_<id>_<UTC>.log`. UTC stamp: `date -u +%Y-%m-%dT%H-%MZ`.
- `logs/`: UDS transcripts `uds_<reqid>_<UTC>.log`, caringcaribou/gallia output (point gallia's artifact directory into `logs/`), `ip -details -statistics link show` snapshots.
- Client DBC files and their hashes go in `dumps/`.
- Mission note (`write_note`, `append: true`): vehicle or bench identity (make, model, year, ECU part numbers), connection point and pins, adapter and driver, bitrate, ignition/engine state, each operator action with its UTC time, each active step with the OK reference, DTCs set and cleared.
- Seed/key algorithms or keys, security-access constants, PINs, immobiliser data, credentials from DIDs: `save_secret` with the log file and line as `source`.

## Typical findings
- "Weak UDS security access on <ECU> (static or predictable seed / reversible algorithm)". Confirmed: seed/key pairs and a computed key accepted by the ECU (`67 02`, sent with OK). Candidate: same seed after reset, short seed, or algorithm recovered but not yet accepted. Via the OBD port: `CVSS:3.1/AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H` (6.8); from a compromised ECU on the same bus use `AV:A` and argue it in the description. Remediation: ISO 14229 authentication (`0x29`) or strong per-ECU secrets, attempt counters and delays.
- "Security access without attempt limiting". Confirmed: repeated invalid keys never return `36`/`37` (bench, OK).
- "Sensitive diagnostic services available without security access" (e.g. `0x23` ReadMemoryByAddress, `0x2E`, `0x31` routines in default/extended session). Confirmed for reads by a positive response; for writes only claim what was tested with OK. Read-only exposure: `AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N` (4.6).
- "Gateway forwards OBD-port traffic to internal domain". Confirmed: frames sent on the OBD port (OK) seen on an internal bus capture, or an internal ECU answering. Remediation: gateway filtering and diagnostic-only routing with authentication.
- "Control messages accepted on replay (no message authentication)". Confirmed on a bench or stationary vehicle: replayed frames cause the effect (operator video plus logs). `AV:P/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:H` (6.1). Remediation: SecOC with freshness, gateway segmentation.
- Save with `save_finding` (stored as draft; the operator confirms): `observation` with IDs, bitrate, files and timestamps; `poc` with exact commands; `remediation`.

## Pitfalls
- `canplayer` without an interface mapping replays onto the real bus.
- Forgetting `listen-only off` before active tests (frames silently not sent), or forgetting `listen-only on` before sniffing a live vehicle.
- `BUS-OFF` after a wrong-bitrate attempt: the interface stops until restarted; note any DTCs it may have caused on the vehicle.
- `cansend` cannot do multi-frame ISO-TP: responses longer than 7 bytes need flow control, use `isotpsend`/`isotprecv`.
- Two programs on one slcan serial port fail; SocketCAN interfaces can be read by several programs at once (candump plus SavvyCAN is fine).
- Logs grow fast (several thousand frames per second at 500 kbit/s): time-box captures and filter by ID when possible.
- Long sessions drain the vehicle battery; ECUs entering sleep change the traffic you see.
