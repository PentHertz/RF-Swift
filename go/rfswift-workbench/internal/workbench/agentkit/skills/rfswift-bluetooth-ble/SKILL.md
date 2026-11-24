---
name: rfswift-bluetooth-ble
description: Bluetooth Classic (BR/EDR) and Bluetooth Low Energy (BLE) assessment from an RF Swift mission. Covers HCI adapters and the BlueZ stack (hciconfig, btmgmt info, bluetoothctl interactive session with scan, devices, info, connect, menu gatt, list-attributes, select-attribute, read, write), gatttool -I, btmon btsnoop capture, bleak, bettercap ble.recon/ble.enum, Mirage, WHAD, Caeruleus, sniffing with Sniffle (TI CC1352/CC26x2), btlejack (micro:bit), Ubertooth (ubertooth-btle), nRF Sniffer, ice9-bluetooth SDR sniffer, pairing analysis (Just Works, Passkey, OOB, LE Secure Connections), LE Legacy pairing key recovery with crackle, unauthenticated GATT read/write, address privacy and tracking, advertising data leaks, Classic sdptool, l2ping, hcitool, evidence (btsnoop, pcap, GATT table) and typical findings.
---
# Bluetooth Classic and BLE with RF Swift

## Scope
Covers: device discovery, advertising analysis, GATT enumeration, pairing and encryption review,
over-the-air sniffing of BLE connections, Classic service discovery.
Does not cover: Wi-Fi (`rfswift-wifi-assessment`), Zigbee/Thread on 802.15.4 even when the same
nRF52/CC26x2 dongle is used (`rfswift-iot-lpwan-mesh`), firmware extraction over SWD/UART
(hardware methodology), the shipped exploit PoCs (WhisperPair, RACE toolkit, BlueDucky, Breaktooth,
BLERP, SweynTooth, BlueSploit): those need a separate, explicit authorization per CVE and target.

## Ground rules
- Targets are the devices the operator named (MAC, name, model). Bystander devices appear in scans:
  do not connect to them, and keep their addresses out of findings.
- Receiving is free: passive scanning, sniffers in listen mode, reading pcaps/btsnoop.
- Normal adapter behaviour (active scan with SCAN_REQ, inquiry) is not treated as TX.
- Connecting, reading GATT and SDP browsing are active interactions with the target: fine once the
  target is in scope. Writes (GATT `write`, `char-write-req/cmd`, `ble.write`, enabling notifications
  by writing a CCCD) change the target: ask for an OK naming handle/UUID and value, and prefer
  writing back a value you read or a documented benign value.
- Pairing/bonding with a target creates keys on it: OK first, and remove the bond afterwards.
- TX (hard gate): btlejack jamming (`-j`) and hijacking (`-t`), Ubertooth interference (`ubertooth-btle -i`/`-I`),
  Mirage `ble_jam`/`ble_hijack`/`ble_mitm`, Sniffle or WHAD in advertiser/central injection roles,
  BLERP MitM, spoofed advertisements, `l2ping -f` (flood). Jamming and hijacking are forbidden unless
  the operator explicitly authorizes them for a shielded lab enclosure. For any TX: operator enabled
  "Allow RF transmission", explicit OK in chat for this action, `transmits: true`, and `purpose`
  with 2.4 GHz band/channels, tx power, duration and target address.
- Changing the local adapter address (`bdaddr`) impersonates a device and can be permanent on some
  chips: OK required.

## Tools in RF Swift (verify with `command -v`)
| Tool | Docker image | Nix env |
|---|---|---|
| BlueZ: `bluetoothctl`, `btmon`, `bluetoothd`, `hciconfig`, `hcitool`, `hcidump`, `sdptool`, `l2ping`, maybe `gatttool`, `btmgmt` | every image (corebuild), `bluetooth` adds bluez-tools/hcidump | `bluetooth` |
| `mirage` (wrapper over `/opt/mirage-env`) | `bluetooth` | `bluetooth` |
| Sniffle `python_cli` (`/rftools/bluetooth/Sniffle/python_cli`), firmware hex in `/rftools/bluetooth/firmwares/Sniffle` | `bluetooth` | `bluetooth` (`sniffle`) |
| WHAD (`whadup`, `wsniff`, `wble-central`, `wdump`, `wshark`) | `bluetooth` (pipx) | `bluetooth` |
| `caeruleus`, `bluing`, `bdaddr`, `crackle`, `BTSnifferBREDR` | `bluetooth` | `bluetooth` |
| `bleak` (Python), `pybluez`, `scapy` | `bluetooth` | `bluetooth` |
| `btlejack` | not shipped as CLI (firmware archive only, used by Mirage) | `bluetooth` |
| Ubertooth (`ubertooth-util`, `ubertooth-btle`, `ubertooth-rx`, `ubertooth-scan`) | `wifi`/`network` (pulled in with Kismet) | `bluetooth`, `wifi` |
| nRF Sniffer for BLE (Wireshark extcap) | zip only in `/rftools/bluetooth/firmwares/NRFSniffer` | `bluetooth` (`nordic-nrf-sniffer`) |
| `wireshark`/`tshark` | not in `bluetooth` image (`wifi`, `network` have it) | `bluetooth` |
| `bettercap` (`ble.recon`, `ble.enum`, `ble.write`) | `wifi`, `network` | `wifi`, `network` |
| `ice9-bluetooth` (SDR, HackRF/bladeRF/USRP) | `sdr_full` | `bluetooth`, `sdr_full` |
Start with `recommend_tools {task: "ble gatt sniffing"}`.
Presence: `command -v bluetoothctl btmon hciconfig gatttool btmgmt crackle mirage whadup btlejack ubertooth-btle`.

## Phase 0: adapter and stack
```sh
hciconfig -a                 # hci0/hci1, BD_ADDR, UP/DOWN, features
btmgmt info                  # controller version, current settings: powered le bredr ssp sc privacy
bluetoothctl list; bluetoothctl show
rfkill list bluetooth
ls /dev/ttyACM* /dev/ttyUSB* 2>/dev/null   # sniffer dongles (Sniffle, nRF, micro:bit, ESP32)
```
- Containers: HCI devices come from the host kernel. The lab needs `NET_ADMIN` (and the device
  mappings the operator chose, `/dev/vhci` for some tools). `bluetoothctl` and `bleak` need a
  running `bluetoothd` on a D-Bus the container can reach. If `bluetoothctl` prints
  "Waiting to connect to bluetoothd", either start one in the lab
  (`service dbus start; bluetoothd -n &`, path often `/usr/libexec/bluetooth/bluetoothd`) or ask the
  operator. Two `bluetoothd` (host and lab) fighting over one controller cause random failures: ask
  the operator to stop the host daemon or dedicate a USB dongle to the lab.
- Raw-HCI tools (`hcitool`, `gatttool`, Mirage, bettercap) can conflict with `bluetoothd` on the same
  adapter; use one stack at a time per adapter and note which.
- Nix missions run as the user: HCI operations need `sudo`/`rfsudo`; stop and ask if sudo prompts.

## Phase 1: discovery and advertising (passive first)
Record HCI traffic of everything you do from the start (background job):
```sh
btmon -w captures/ble_hci0_2026-11-24T10-05Z_<target>.btsnoop
```
Passive LE scan (no SCAN_REQ): `hcitool -i hci0 lescan --passive --duplicates` (Ctrl-C to stop,
use `max_seconds`). Normal scan through BlueZ:
```sh
bluetoothctl --timeout 30 scan on      # older BlueZ: interactive "scan on"; "scan le" or menu scan > transport le on newer
bluetoothctl devices
bluetoothctl info AA:BB:CC:DD:EE:FF    # AddressType, Name, UUIDs, ManufacturerData, RSSI, Paired, Bonded
```
Interactive session for longer work: `session_start {name: "btctl", command: "bluetoothctl"}`, then
`session_send {name: "btctl", input: "scan on", wait_seconds: 20}`, `scan off`, `devices`.
bleak one-shot (needs bluetoothd):
```sh
python3 - <<'EOF'
import asyncio
from bleak import BleakScanner
async def main():
    for d, adv in (await BleakScanner.discover(timeout=15, return_adv=True)).values():
        print(d.address, adv.rssi, adv.local_name, adv.service_uuids, adv.manufacturer_data)
asyncio.run(main())
EOF
```
bettercap (wifi/network images): `bettercap -no-colors -eval "ble.recon on; sleep 30; ble.show; ble.recon off; quit"`.
Address analysis: `public` addresses and random static (most significant byte 0xC0-0xFF) do not
change; resolvable private addresses (0x40-0x7F) rotate; non-resolvable (0x00-0x3F) are ephemeral.
Watch the advertising payload for names with serials or owner names, manufacturer data (company ID),
device state, or keys/tokens in service data.

## Phase 2: GATT enumeration (active, in-scope target)
bluetoothctl session:
```
connect AA:BB:CC:DD:EE:FF
menu gatt
list-attributes
select-attribute /org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF/service0010/char0011   # or a UUID
attribute-info
read
back
disconnect
```
`write "0x01 0x00"` and `notify on` only after an OK (see Ground rules). gatttool, if present:
```sh
gatttool -i hci0 -b AA:BB:CC:DD:EE:FF -t random -I     # then: connect, primary, characteristics, char-desc
# char-read-hnd 0x0012, char-read-uuid 2a00 ; writes: char-write-req 0x0013 0100 (OK first)
gatttool -b AA:BB:CC:DD:EE:FF -t random --characteristics   # non-interactive
```
Use `-t public` for public addresses. Retry at `--sec-level=medium` only to observe whether
encryption is required (it may trigger pairing: OK first). Other enumerators: `bettercap` `ble.enum
AA:BB:CC:DD:EE:FF`, `caeruleus` (JSON output, see `caeruleus --help`; its write/fuzz modes change the
target), Mirage (`mirage --help`, modules such as `ble_info`, `ble_scan`, `ble_discover`, `ble_master`).
For each characteristic record: handle, UUID, properties, whether a read/write succeeded without
pairing, and the value (redact secrets; store them with `save_secret`).

## Phase 3: pairing analysis
Pair only with an OK (`bluetoothctl`: `agent KeyboardDisplay`, `default-agent`, `pair <MAC>`;
afterwards `remove <MAC>`). Read the SMP exchange from the btsnoop
(`btmon -r captures/<file>.btsnoop | less`, or Wireshark filter `btsmp`):
- Pairing Request/Response: IO capability, OOB flag, AuthReq bits (Bonding, MITM, SC, Keypress),
  Max Encryption Key Size (below 16 is a weakness), key distribution.
- SC=0 means LE Legacy pairing. Just Works (TK = 0) or a 6-digit passkey in Legacy pairing can be
  recovered from a sniffed pairing: `crackle -i captures/<sniff>.pcap -o captures/<sniff>_decrypted.pcap`.
  crackle needs the whole pairing in a link-layer capture (sniffer, not btsnoop); with a known LTK
  `crackle -i in.pcap -o out.pcap -l <LTK>` decrypts later sessions (put the LTK in `save_secret`).
- SC=1 (LE Secure Connections) resists passive recovery; Just Works still gives no MITM protection.
- Classic: legacy PIN pairing (fixed `0000`/`1234`) or SSP Just Works on devices with input/display.

## Phase 4: over-the-air sniffing (receive only unless stated)
Needed to see other devices' connections, pairings and traffic. Verify hardware and firmware first;
flashing sniffer firmware changes the operator's dongle: ask.
- Sniffle (CC26x2/CC1352 boards, incl. Sonoff CC2652P dongles with Sniffle firmware):
  ```sh
  cd /rftools/bluetooth/Sniffle/python_cli
  python3 sniff_receiver.py -s /dev/ttyUSB0 -m AA:BB:CC:DD:EE:FF -o /workspace/captures/ble_sniffle_<UTC>_<target>.pcap
  ```
  (`-e` extended advertising, `-l` coded PHY, `-a` advertising only; check `--help`; use the
  workspace path of your mission.) Its scanner/initiator/advertiser roles transmit.
- btlejack (Nix `bluetooth`, micro:bit with btlejack firmware): `btlejack -s` (detect connections),
  `btlejack -c AA:BB:CC:DD:EE:FF -x ll_phdr -o captures/<name>.pcap` (follow new connections); check
  `btlejack -h`. `-j` (jam) and `-t` (hijack) are TX and forbidden outside an authorized shielded lab.
- Ubertooth: `ubertooth-util -v` (firmware must match host tools), then
  `ubertooth-btle -n -q captures/<name>.pcap` (advertisements), `ubertooth-btle -f -t AA:BB:CC:DD:EE:FF -q captures/<name>.pcap`
  (follow target connections), `ubertooth-btle -p` (promiscuous, existing connections).
  Classic: `ubertooth-rx` (LAP discovery), `ubertooth-scan` (inquiry plus LAP/UAP).
- nRF Sniffer: Wireshark extcap; `tshark -D` lists the extcap interface, then
  `tshark -i <iface> -w captures/<name>.pcapng`.
- WHAD with ButteRFly on nRF52840: `whadup` lists devices; `wsniff -i uart0 ble | wdump captures/<name>.pcap`
  (check `wsniff -h`).
- ice9-bluetooth (SDR, all channels; validate the SDR with `rfswift-sdr-setup` first): `ice9-bluetooth -l` to list interfaces, then see `-h` for
  center/channel options and `-w <pcap>`. High sample rates: check USB throughput.
- ESP32 BR/EDR sniffer `BTSnifferBREDR` connects to the target itself (active); OK first; see `--help`.
Long captures: background job with `max_seconds`; check size with `ls -l captures/`.

## Phase 5: Classic BR/EDR
```sh
hcitool -i hci0 scan                    # inquiry, discoverable devices only
hcitool -i hci0 info AA:BB:CC:DD:EE:FF  # pages the device (active)
sdptool browse AA:BB:CC:DD:EE:FF        # services, RFCOMM channels, profiles
l2ping -i hci0 -c 3 AA:BB:CC:DD:EE:FF   # reachability (active, in-scope only; never -f)
```
Non-discoverable devices can still be found by LAP sniffing (`ubertooth-rx`). Look for services
that accept unauthenticated connections (OBEX, serial port profiles) and legacy pairing.

## Reading results
- "Connection failed: le-connection-abort-by-local" often means wrong address type or the device
  only accepts its bonded central; retry with the right `-t`, do not hammer it.
- A characteristic marked read but returning "Insufficient Authentication/Encryption" is protected:
  good. A value returned without any SMP exchange in btsnoop is an unauthenticated read.
- `ATT Write Response` without error = the write was accepted; confirm the effect on the device.
- Sniffers miss connections that started before they followed; restart the target connection.
- Same name on many addresses usually means rotating RPAs, not many devices.

## Evidence
- `captures/ble_hci0_<UTC>_<target>.btsnoop` (our own HCI traffic), `captures/ble_<sniffer>_<UTC>_<target>.pcap`
  (over-the-air), decrypted outputs next to them.
- GATT table dump in the note: service UUID, char UUID, handle, properties, auth required (y/n),
  value or "secret <label>".
- Mission note (append): adapter, stack mode (bluetoothd/raw HCI), firmware versions of sniffers,
  commands, UTC times, every write/pair/TX with its OK.
- Keys (LTK, passkeys, PINs), tokens read from GATT: `save_secret` with the capture file and frame as source.

## Typical findings
Titles: "<device>: <weakness> allows <impact>".
- Unauthenticated GATT read exposes sensitive data. Confirmed: btsnoop shows the read without
  pairing plus the value class. `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N` (6.5), C:L if minor.
- Unauthenticated GATT write controls a function (unlock, config, DFU mode). Confirmed: approved
  write and observed effect. `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:H` (8.1) or I:H only (6.5).
- LE Legacy pairing allows passive key recovery (crackle decrypted traffic). Confirmed: decrypted
  frames. `CVSS:3.1/AV:A/AC:H/PR:N/UI:N/S:U/C:H/I:N/A:N` (5.3).
- Just Works pairing on a device with display/input (no MITM protection):
  `CVSS:3.1/AV:A/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:N` (6.8), candidate until a MITM is shown in a lab.
- Static/public address and stable advertising allow tracking: same address over at least two
  sessions hours apart or across a power cycle. `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N` (4.3).
- Advertising leaks serial/owner/state: same vector (4.3).
- Reduced encryption key size accepted, legacy PIN pairing with fixed PIN on Classic.
Remediation patterns: require LE Secure Connections with MITM (numeric comparison/passkey) for
sensitive characteristics, set attribute permissions to authenticated/encrypted, use RPAs (privacy),
remove secrets from advertising, 16-byte key size, signed firmware updates.

## Pitfalls
- USB Bluetooth dongles power-save; `hciconfig hci0 reset` or replug (ask the operator) on timeouts.
- Wi-Fi in 2.4 GHz next to the sniffer degrades capture; move away from APs.
- Sniffle/Ubertooth host tools and firmware versions must match.
- bluetoothctl caches devices and GATT: `remove <MAC>` before re-enumerating a changed device.
- btsnoop shows only our controller's traffic; it is not an over-the-air capture of others.
- Never leave a bond on the client's device; record removal in the note.
