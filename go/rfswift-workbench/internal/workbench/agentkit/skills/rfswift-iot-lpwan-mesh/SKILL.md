---
name: rfswift-iot-lpwan-mesh
description: IoT radio mesh and LPWAN assessment from an RF Swift mission. Covers IEEE 802.15.4 (Zigbee, Thread, Matter over Thread), Z-Wave (S0, S2, Z-Wave LR), LoRa and LoRaWAN (OTAA, ABP, join request, DevNonce, frame counters, AppKey, gateways), Meshtastic and other sub-GHz IoT sensors. Sniffing hardware and tools: KillerBee (zbid, zbdump, zbwireshark, zbdsniff, zbstumbler, zbreplay) with ApiMote, RZUSBstick, CC2531; WHAD and Mirage on nRF52840; HackRF/SDR with gr-ieee802-15-4, gr-lora_sdr, gr-lora, gr-zwave_poore, waving-z, meshtastic_sdr, rtl_433. Zigbee default Trust Center link key ZigBeeAlliance09, network key transport during join, install codes, touchlink; Thread/Matter commissioning observation; Z-Wave S0 key exchange weakness; LoRaWAN ABP replay; evidence (pcap, keys via save_secret), TX rule for injection and replay, typical findings.
---
# IoT mesh and LPWAN (802.15.4, Z-Wave, LoRa) with RF Swift

## Scope
Covers: Zigbee and other 802.15.4 networks (2.4 GHz channels 11-26), Thread and Matter-over-Thread
(passive view), Z-Wave (868/908/916 MHz and regional variants, Z-Wave LR), LoRa PHY and LoRaWAN,
Meshtastic, generic sub-GHz IoT sensors decodable by rtl_433.
Does not cover: BLE (Matter commissioning starts over BLE: `rfswift-bluetooth-ble`), Wi-Fi
(`rfswift-wifi-assessment`), gate/garage remotes and raw OOK/FSK reverse engineering (generic SDR
capture methodology), cellular LPWAN (NB-IoT/LTE-M, telecom images).

## Ground rules
- Scope = the client's network identifiers (Zigbee PAN ID / extended PAN ID, Z-Wave Home ID, LoRaWAN
  DevEUI/JoinEUI/DevAddr ranges, gateway EUIs) and named devices. Neighbouring networks show up in
  captures: never act on them; keep their identifiers out of findings.
- Receiving is free: sniffers in listen mode, SDR receive flowgraphs, rtl_433, offline decoding.
- TX (hard gate): `zbstumbler` (sends beacon requests on each channel), `zbreplay`, `zbassocflood`,
  `zbkey`, `zbpanidconflictflood` and any KillerBee/Scapy injection; Mirage/WHAD injection or
  `zigbee_*` attack modules; touchlink scan/reset commands; GNU Radio flowgraphs with an enabled
  sink (gr-ieee802-15-4 examples are transceivers; gr-lora_sdr TX examples); `wave-out` (waving-z);
  `hackrf_transfer -t`; rfcat/YARD Stick One; LoRa/LoRaWAN uplink or downlink replay; joining a rogue
  node or gateway. For each: operator enabled "Allow RF transmission", explicit OK in chat for this
  specific transmission, `transmits: true`, and `purpose` with frequency/channel, power/gain, duration
  and target identifier. These are ISM bands, but duty-cycle and power limits apply (EU868: mostly 1%
  or 0.1% duty cycle), and replays to production networks can open locks or trigger alarms. Prefer a
  shielded enclosure or coax with attenuators for any replay/injection test; over the air only with
  the operator's explicit OK for that location and device.
- Keys (network keys, link keys, AppKey/AppSKey/NwkSKey, S0/S2 keys, Meshtastic channel PSKs) are
  secrets: `save_secret` with exact source, never in notes, findings, `purpose` lines or chat.

## Tools in RF Swift (verify with `command -v` or the path)
| Tool | Docker image | Nix env |
|---|---|---|
| KillerBee (`zbid`, `zbdump`, `zbwireshark`, `zbdsniff`, `zbconvert`, `zbstumbler`, `zbreplay`) | `sdr_light`, `sdr_full` | `sdr_light`, `sdr_full`, `sdr_hacks` |
| WHAD (`whadup`, `wsniff`, `wdump`), Mirage (`zigbee_*` modules) | `bluetooth` | `bluetooth` |
| gr-ieee802-15-4 (`/rftools/sdr/oot/gr-ieee802-15-4/examples`) | `sdr_full` | `sdr_full`: check `python3 -c 'import gnuradio.ieee802_15_4'` |
| gr-lora_sdr, gr-lora (rpp0) (`/rftools/sdr/oot/...`) | `sdr_full` | `sdr_full`: check `python3 -c 'import gnuradio.lora_sdr'` |
| gr-zwave_poore | `sdr_full` | not confirmed: check the import |
| waving-z (`wave-in`; Docker: `/rftools/sdr/waving-z/build/`) | `sdr_full` | `sdr_full` |
| meshtastic_sdr (`/rftools/sdr/meshtastic_sdr`), `meshtastic` Python CLI | `sdr_full` | `sdr_full` |
| `rtl_433`, `hackrf_transfer`, `rtl_sdr`, `urh`, `inspectrum`, `rfcat` | `sdr_light`, `sdr_full` | `sdr_light`, `sdr_full` |
| `wireshark`/`tshark` | `network`, `wifi` (not `sdr_*`) | `bluetooth`, `network`, `wifi` |
| RFQuack (sub-GHz radio firmware), LR2021-SDR (`lriq_tool`) | `sdr_full` / `sdr_hacks` | `sdr_hacks` (LR2021) |
Not shipped by RF Swift: whsniff, ZBOSS/zigpy sniffers (zigpy-znp, bellows), Silicon Labs Zniffer,
zwave-js, OpenThread tools (`ot-ctl`, pyspinel sniffer), Matter `chip-tool`, ChirpStack or another
LoRaWAN network server, LoRaWAN packet decoders. Say so when the task needs them; the operator can
use their own tools on the client's infrastructure.
Start with `recommend_tools {task: "zigbee 802.15.4 sniffing"}`.

## Phase 0: hardware detection
```sh
lsusb; ls /dev/ttyUSB* /dev/ttyACM* 2>/dev/null
zbid                      # KillerBee-supported devices (ApiMote, RZUSBstick, CC2531 with sniffer firmware, ...)
whadup                    # WHAD devices (nRF52840 with ButteRFly firmware)
hackrf_info; rtl_test -t
```
For SDR validation (firmware, drops, gain, PPM) follow `rfswift-sdr-setup` first.
If `zbid` lists nothing, check the dongle firmware (CC2531 needs a sniffer firmware, RZUSBstick needs
the KillerBee firmware). Reflashing changes the operator's hardware: ask first.

## Phase 1: 802.15.4 / Zigbee / Thread (passive)
Channel k (11-26) is at 2405 + 5*(k-11) MHz. Zigbee usually sits on 11, 15, 20 or 25; Thread on the
channel of its operational dataset. Passive channel survey without beacon requests: capture each
channel for a fixed time and compare frame counts.
```sh
mkdir -p captures logs
for ch in $(seq 11 26); do timeout 20 zbdump -i <dev> -c $ch -w captures/154_ch${ch}_2026-11-24T10-05Z_survey.pcap; done
```
`zbdump -h`: older builds use `-f` for the channel. Long single-channel capture as a background job:
`execute_command {command: "zbdump -i <dev> -c 15 -w captures/zigbee_ch15_2425MHz_<UTC>_<panid>.pcap",
purpose: "Passive Zigbee capture ch15 2425 MHz, 2 h, no TX", wait_seconds: 5, max_seconds: 7200}`.
Live view: `zbwireshark -i <dev> -c 15` (GUI). WHAD: `wsniff -i uart0 zigbee -c 15 | wdump captures/<name>.pcap`
(check `wsniff -h`). `zbstumbler` is the active (beacon request) alternative: TX gate.
SDR (HackRF, `sdr_full`): gr-ieee802-15-4 needs 4 Msps around the channel centre. The shipped examples
(`transceiver_OQPSK.grc`) contain both a source and a sink: copy one into `logs/`, delete or disable
every sink, keep the source -> OQPSK PHY -> MAC -> Wireshark Connector -> File Sink (pcap) chain,
and run it with `grcc` + `python3`. A flowgraph with an enabled sink is TX.
Reading Zigbee captures (Wireshark/tshark):
- `zbee_nwk`, `zbee_aps`, `zbee_zdp` filters. Beacons show PAN ID, extended PAN ID, permit-join,
  stack profile.
- Joins: Association Request/Response, then APS Transport Key from the Trust Center. With the
  well-known default Trust Center link key "ZigBeeAlliance09" (legacy/centralized security), the
  network key in that frame is recoverable by anyone who captured the join. `zbdsniff captures/*.pcap`
  extracts transported keys; in Wireshark add keys under ZigBee protocol preferences
  (pre-configured keys). Store any recovered network key with `save_secret`; do not paste it.
- Zigbee 3.0 install codes derive a per-device link key: joins with install codes do not leak the
  network key passively. Unencrypted APS/NWK frames (security level 0) are a finding by themselves.
- Touchlink (ZLL) commissioning works at short range with a widely published master key: devices
  that still accept touchlink can be factory-reset or moved to another network. Proving it is TX
  (inter-PAN scan/reset): only on the client's test device, with OK, at close range.
Thread / Matter over Thread: MAC frames are encrypted with the network key from the start. Passively
you see PAN ID, extended PAN ID (in MLE discovery), extended/short addresses, frame counters,
traffic volume and timing. Commissioning uses DTLS with EC-JPAKE (PSKc/PSKd), which resists offline
guessing. With the client's operational dataset (from their border router, operator-provided),
Wireshark can decrypt: store the key as a secret. Matter commissioning: BLE advertising with service
UUID 0xFFF6 (discriminator, vendor/product IDs) and setup codes on labels; the Matter SDK test
passcode 20202021 / discriminator 3840 or test device attestation certificates on a production
device are findings. No `chip-tool` in RF Swift: commissioning tests need the client's controller.

## Phase 2: Z-Wave
Frequencies: EU 868.40/868.42 MHz (9.6/40 kbit/s), 869.85 MHz (100 kbit/s); US 908.40/908.42 and
916.00 MHz; Z-Wave LR (US) 912 and 920 MHz; other regions differ: ask the operator for the region.
Receive with waving-z (RTL-SDR pipeline; check `wave-in --help` for input formats):
```sh
rtl_sdr -f 868420000 -s 2048000 -g 25 - | /rftools/sdr/waving-z/build/wave-in -u | tee logs/zwave_868.42M_<UTC>.log
```
or record IQ first and decode offline (see Evidence); gr-zwave_poore offers GNU Radio blocks
(`sdr_full`, check its examples, remove sinks). Look at Home ID, source/destination node IDs,
command classes, and whether frames are Security S0 (0x98) / S2 (0x9F) encapsulated or plain.
Security: S0 encrypts the network key exchange during inclusion with a fixed all-zero temporary
key, so a sniffed S0 inclusion reveals the network key. S2 uses ECDH with the device DSK
(authenticated or unauthenticated class); a controller that falls back to S0 or no security for an
S2-capable device is a downgrade finding. Z-Wave LR requires S2. Unencapsulated commands (no S0/S2)
to locks, alarms or valves are replayable; replay is TX.

## Phase 3: LoRa / LoRaWAN
Parameters: EU868 default channels 868.1/868.3/868.5 MHz, 125 kHz, SF7-SF12, RX2 869.525 MHz;
US915 uplinks 902.3-914.9 MHz (125 kHz) and 903.0-914.2 MHz (500 kHz), downlinks 923.3-927.5 MHz.
Locate activity with a waterfall (`gqrx`, SDR++ or `hackrf_sweep`), then decode with gr-lora_sdr:
use its RX example flowgraph (check `/rftools/sdr/oot/gr-lora_sdr/examples`, confirm it has no sink),
set centre frequency, bandwidth, spreading factor, coding rate and sample rate (a multiple of the
bandwidth), and save decoded payload bytes plus the IQ. The rpp0 gr-lora module is an alternative.
LoRaWAN frame review (PHYPayload: MHDR, MACPayload, MIC):
- Join Request (MType 0): JoinEUI, DevEUI and DevNonce in clear. DevEUIs identify device models and
  allow tracking; repeated DevNonce values or a counter that resets suggest weak nonce handling
  (LoRaWAN 1.0.4/1.1 require monotonic DevNonce).
- Data frames: DevAddr, FCtrl, FCnt, FPort in clear; FRMPayload encrypted (AppSKey, or NwkSKey on
  FPort 0); MIC under NwkSKey. ABP devices whose FCnt restarts at 0 after reboot and are still
  accepted mean the network server has relaxed frame counter checks: replay candidate. Confirming
  acceptance of a replay is TX and needs an OK, a test device and the network server logs.
- Keys: OTAA AppKey (and NwkKey in 1.1) shared across a fleet, printed on labels, or readable from
  firmware/UART is a frequent root cause; gateways with the legacy Semtech UDP packet forwarder send
  traffic unauthenticated in clear over IP (UDP 1700). Network servers (ChirpStack and others) are
  not shipped; review them with the client's access.
Meshtastic: default primary channel (LongFast) uses a publicly known default key, so traffic is
readable by anyone; EU868 default slot 869.525 MHz, US 906.875 MHz. meshtastic_sdr flowgraphs receive
it. `meshtastic --port /dev/ttyACM0 --info` on the client's own node prints the channel URL including
the PSK: treat that output as a secret (do not leave it in notes).
Other sub-GHz IoT sensors: `rtl_433 -f 868.3M -s 1024k -F json:logs/rtl433_868.3M_<UTC>.json -M time:utc -M level`
(background job). Decoded IDs show which sensors are unauthenticated and spoofable.

## Reading results
- Few frames on a channel does not mean no network: Zigbee end devices sleep; capture for hours or
  ask the operator to trigger a join, a switch toggle or a rejoin (that is the operator acting, not TX).
- FCS/CRC errors in SDR decoding usually mean wrong centre frequency, sample rate or gain; fix before
  concluding anything.
- A key printed by `zbdsniff` is confirmed only if Wireshark decrypts later NWK frames with it.
- LoRa decodes with CRC error or impossible MType values are noise.

## Evidence
- pcaps: `captures/zigbee_ch15_2425MHz_<UTC>_<panid>.pcap`, `captures/zwave_868.42M_<UTC>_<homeid>.log`.
- IQ: `captures/<protocol>_<freq>_<rate>_<UTC>_<label>.cs8` (HackRF:
  `hackrf_transfer -r <file>.cs8 -f 868100000 -s 2000000 -l 24 -g 20 -n <samples>`) with a SigMF
  `.sigmf-meta` next to it: `core:datatype` (`ci8` for HackRF, `cu8` for RTL-SDR), `core:sample_rate`,
  `captures[0].core:frequency`, `core:datetime`, `core:hw`, gain settings in a description.
- Mission note (append): region and band plan, hardware and firmware, exact commands, channel/SF
  settings, UTC times, identifiers in scope, every TX action with its OK; keys only as secret labels.

## Typical findings
Titles: "<network/device>: <weakness> allows <impact>".
- Zigbee network key exposed through default Trust Center link key during join. Confirmed: key
  extracted and decrypts traffic. `CVSS:3.1/AV:A/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:N` (6.8); AC:L (8.1)
  if a rejoin can be forced. Fix: install codes, Zigbee 3.0 with unique link keys, network key rotation.
- Z-Wave S0 inclusion exposes the network key / S2 downgrade to S0: same vector (6.8). Fix: S2 only.
- Unencrypted or unauthenticated control commands accepted (Zigbee security level 0, Z-Wave without
  S0/S2, 433/868 MHz sensors): `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N` (6.5). Confirmed only
  with an approved replay in a controlled setup.
- Touchlink accepted: `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:H` (8.1) when proven on a test device.
- LoRaWAN ABP with frame counter reset accepted (replay): 6.5 as above, confirmed with NS logs.
- Shared or exposed LoRaWAN AppKey: severity by fleet impact (physical extraction: AV:P).
- Meshtastic or other mesh on default channel key: `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N` (6.5).
- Matter device with test credentials (test passcode, test DAC): report as configuration finding.
- DevEUI/identifier exposure enabling tracking: `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N` (4.3).

## Pitfalls
- 2.4 GHz Wi-Fi drowns 802.15.4 channels 11-22; Zigbee channels 15, 20, 25, 26 sit between Wi-Fi 1/6/11.
- HackRF minimum sample rate is 2 Msps; with RTL-SDR stay below 2.4 Msps to avoid drops.
- Strong nearby transmitters saturate the SDR front end: lower LNA/VGA gain before trusting decodes.
- KillerBee and WHAD may claim the same serial device; run one at a time.
- Keys typed on a command line end up in job logs: prefer Wireshark key preference files, refer
  to secret labels in notes.
