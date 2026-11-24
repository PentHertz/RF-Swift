---
name: rfswift-iot-device-assessment
description: End-to-end method for assessing one IoT or radio device in RF Swift - inventory every radio and debug interface from the FCC ID, documentation and teardown, set the RF scope and the mission board, order the work passive first (RF recon, signal analysis, protocol tests, hardware and firmware, cross-interface chains), choose the domain skills and subagents for each interface, plan the hardware and spare units, and deliver findings mapped to EN 18031 / ETSI EN 303 645. Use it at the start of any device engagement (smart lock, alarm, camera, gateway, wearable, keyfob, meter, industrial sensor) before the protocol-specific skills.
---

# Assessing an IoT or radio device

The protocol skills (`rfswift-wifi-assessment`, `rfswift-bluetooth-ble`,
`rfswift-iot-lpwan-mesh`, `rfswift-subghz-ism`, `rfswift-rfid-nfc`,
`rfswift-hardware-interfaces`, `rfswift-firmware-analysis`, ...) each cover one
interface. A device has several, and its weaknesses often sit between them: a key
read from flash decrypts the radio link, a debug console reveals the cloud token.
This skill organises the whole assessment. It assumes `rfswift-mission-workflow`.

## 1. Before touching the device

Collect, and write to the mission note:

- **Authorization and scope**: which units, interfaces, back ends and apps are in
  scope; what may be modified or destroyed; time windows for disruptive tests;
  whether transmission is approved and in which bands.
- **Identity**: vendor, model, hardware and firmware versions, serial numbers of
  the units. The **FCC ID** (or the equivalent ISED / CE technical file when the
  client provides it): the FCC database (fcc.gov OET equipment authorization
  search; fccid.io mirrors it) usually has internal photos, the test report with
  operating frequencies, modulation and output power, and the user manual. The
  operator looks it up; you read what they save.
- **Units**: how many. Flash dumps, desoldering and glitching can brick a unit;
  ask for at least one spare before any of them.
- **Companion app and cloud**: platforms, account test credentials, back-end
  hosts. Web and cloud testing use the RF Swift `network` image tools (ZAP,
  nuclei, the opt-in cloud-posture tools); record what is in scope.

## 2. Interface inventory

Build this table in `board.md` (Facts) and keep it current:

| Interface | Evidence | Frequency / bus | Role | Skill | State |
|---|---|---|---|---|---|
| Wi-Fi | FCC report, beacon seen | 2.4 GHz ch 6 | client of home AP | rfswift-wifi-assessment | to test |
| BLE | advertising seen | 2.4 GHz | provisioning | rfswift-bluetooth-ble | to test |
| Sub-GHz | FCC report: 433.92 MHz OOK | 433.92 MHz | remote | rfswift-subghz-ism | to test |
| UART | 4-pin header J3 (photo) | 3.3 V | console | rfswift-hardware-interfaces | to probe |
| SPI flash | W25Q64 (photo) | SPI | firmware | rfswift-hardware-interfaces, rfswift-firmware-analysis | to read |
| Cloud | app traffic | HTTPS/MQTT | telemetry, commands | network image | in scope? |

Sources, in order of reliability: what you observed (sweeps, captures, probes),
the FCC test report, the internal photos, the datasheets of the chips visible on
the photos, the vendor documentation. Mark each row with its evidence.

Typical radio chips tell you a lot: an ESP32 means Wi-Fi + BLE and a UART
bootloader; an nRF52 means BLE (possibly Thread/Zigbee) and SWD; a CC1101 or
Si4463 means sub-GHz FSK/OOK; an EFR32 or CC2652 means Zigbee/Thread/BLE; a
Quectel or u-blox module means cellular (`rfswift-cellular-lab`) or GNSS.

## 3. RF scope and board

Ask the operator to set the mission's **RF scope** in the Workbench (Agent panel,
RF scope) from the inventory: the bands you may transmit in (only those the
authorization covers, at the device's own frequencies), the in-scope addresses
(the device's BSSID or BLE address, the test AP), and the addresses to exclude
(neighbouring networks and devices). Read it back with `read_scope`.

Create `board.md` (`rfswift-mission-board`): the goal in one line, the inventory
as facts, one open direction per interface.

## 4. Order of work

Passive before active, non-destructive before destructive, and keep evidence of
every step.

1. **RF reconnaissance** (`rfswift-rf-recon` subagent; `rfswift-sdr-setup`,
   `rfswift-spectrum-survey`, `rfswift-iq-capture`): confirm every radio of the
   inventory transmits where expected, capture each one while operating the
   device (pairing, normal use, update, reset), and look for emitters the
   documentation does not mention.
2. **Signal and protocol analysis** (`rfswift-signal-analyst`;
   `rfswift-signal-analysis`): demodulate and decode the captures; identify
   standard stacks versus proprietary protocols; compare repeated actions for
   counters, rolling codes and nonces.
3. **Protocol tests per interface** (`rfswift-wireless-tester` with the domain
   skill): pairing and authentication, encryption, replay and freshness,
   unauthenticated commands, information leaks. Transmissions only within the
   scope and with the operator's approval.
4. **Hardware and firmware** (`rfswift-hardware-hacker`;
   `rfswift-hardware-interfaces`, `rfswift-firmware-analysis`): debug consoles,
   JTAG/SWD state, flash read-out (backups first), firmware extraction and
   analysis, secrets and update mechanism. Keep a spare unit for anything
   destructive.
5. **Cross-interface chains**: test what one interface revealed against the
   others. A network key or PSK from firmware: does it decrypt the captured
   Zigbee or Wi-Fi traffic? A BLE long-term key or a cloud token from flash: is it
   unique to this unit (compare two units) or shared by every device? A debug
   console: does it expose the radio configuration or the pairing secrets?
6. **App and cloud**, when in scope: provisioning flow, API authorization,
   firmware download location and integrity.

Update the board at the end of each step: facts with evidence, directions closed
or opened.

## 5. Hardware checklist

Ask the operator early; most delays come from missing adapters.

| Interface | Hardware |
|---|---|
| Survey and capture | HackRF, RTL-SDR, USRP/bladeRF for wide or precise captures; antennas for each band |
| Sub-GHz transmit tests | HackRF or YARD Stick One (rfcat) |
| Wi-Fi | adapter with monitor mode and injection for the bands in scope |
| BLE | host adapter; sniffer (nRF52840 dongle with Sniffle/nRF Sniffer, or micro:bit with btlejack) |
| Zigbee/Thread | CC2652/CC2531/nRF52840 sniffer, or SDR with the GNU Radio modules |
| RFID/NFC | Proxmark3; blank and magic cards of the right type for clone tests |
| UART/JTAG/SWD | USB-UART at the right voltage, logic analyser, J-Link/ST-Link/CMSIS-DAP/FT2232H |
| Flash | SOIC clip and CH341A/FT2232H programmer (check voltage: 1.8 V chips need an adapter) |
| Cellular | shielded enclosure or coax with attenuators, test SIMs |

## 6. Safety and limits

- Never transmit outside the RF scope, never jam, and never transmit in licensed
  bands except into a shielded enclosure or attenuated coax.
- Back up before every write (two identical flash reads, matching sha256).
- Stop and ask when a unit behaves unexpectedly (resets in a loop, heats, stops
  answering) or when the next step could brick it.
- Keep personal data seen in captures (names, locations, other people's devices)
  out of notes and findings; note only what proves the issue.

## 7. Deliverables

- One finding per weakness, with proof and control case
  (`rfswift-findings-qualification`), and a chain section where findings combine.
- `weakness` set on every finding (`map_compliance`), so the report's compliance
  table names the EN 18031 mechanisms, ETSI EN 303 645 provisions, OWASP IoT
  categories and IEC 62443-4-2 requirements. For a Radio Equipment Directive
  engagement, follow `rfswift-red-en18031-assessment` as well.
- The interface inventory with what was tested, what was not and why (missing
  hardware, out of scope, no approval). "Not tested" is a valid entry; "secure" is
  not a conclusion you can draw from not finding something.
- Evidence files in the workspace (`captures/`, `dumps/`, `logs/`) with their
  hashes, referenced from the findings.

## Typical findings on consumer and industrial devices

- Sub-GHz command with a fixed code or no freshness (replay).
- BLE provisioning with Just Works pairing or unauthenticated GATT writes; static
  address enabling tracking.
- Zigbee network key sent in the clear at join with the default Trust Center link
  key; one network key or install code shared by all devices.
- Wi-Fi credentials or cloud tokens readable from unprotected flash; the same
  secret on every unit.
- UART root console without authentication; JTAG/SWD left enabled; U-Boot
  interruptible.
- Firmware updates not signed or not verified; downgrade possible.
- Outdated components with known vulnerabilities (version from the firmware, CVE
  matching by the operator).
