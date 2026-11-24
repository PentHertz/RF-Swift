---
name: rfswift-red-en18031-assessment
description: Technical test plan for radio equipment against the EU Radio Equipment Directive cybersecurity requirements (Article 3(3)(d)(e)(f), harmonised standards EN 18031-1, -2 and -3) in RF Swift - deciding which parts apply, inventorying assets and interfaces, testing each security mechanism (access control, authentication, secure update, secure storage, secure communication, resilience, cryptographic keys, general capabilities, cryptography, and the privacy mechanisms of part 2) with RF Swift tools, and recording a per-mechanism evidence matrix tied to findings. Use it for RED / EN 18031 pre-assessments, CE marking support, or when a client asks which EN 18031 requirements a device fails.
---

# EN 18031 (Radio Equipment Directive) technical assessment

Since 1 August 2025, radio equipment placed on the EU market must meet the
cybersecurity essential requirements of the Radio Equipment Directive, Article
3(3)(d) (network protection), (e) (personal data and privacy) and (f)
(protection from fraud), through the harmonised standards EN 18031-1, -2 and -3
or a notified body. This skill organises the **technical testing** part with RF
Swift. It does not replace the standard: the standard also assesses the
manufacturer's documentation and design decisions, and a conformity decision
belongs to the manufacturer and, where involved, a notified body. Work from the
edition of the standard the client uses, and from the citation in the Official
Journal, which lists restrictions.

Assumes `rfswift-mission-workflow` and `rfswift-iot-device-assessment` (interface
inventory, RF scope, board).

## 1. Which parts apply

Decide with the operator and write it on the board:

| Part | Applies when the equipment... | Assets it protects |
|---|---|---|
| EN 18031-1 (3(3)(d)) | can communicate over the internet, directly or through other equipment | security assets and network assets |
| EN 18031-2 (3(3)(e)) | is internet-connected and processes personal, traffic or location data, or is a toy, childcare equipment or a wearable | security assets and privacy assets |
| EN 18031-3 (3(3)(f)) | is internet-connected and lets the holder transfer money, monetary value or virtual currency | security assets and financial assets |

A device can fall under several parts; the same mechanism is then assessed for
each part's assets.

## 2. Assets and interfaces

List, with the manufacturer's documentation when the client provides it:

- **Security assets**: credentials, keys, certificates, security configuration,
  the firmware and its update mechanism.
- **Network assets**: the networks the device joins or provides and their
  configuration (Wi-Fi, Zigbee/Thread, cellular, Ethernet), and anything that
  could harm the network or its other users.
- **Privacy assets** (part 2): personal, traffic and location data the device
  holds or sends.
- **Financial assets** (part 3): payment credentials and the operations that move
  value.
- **Interfaces**: every radio, wired and debug interface of the inventory, plus
  the app and cloud interfaces, marked "network interface" or not and "accessible
  to whom" (anyone in radio range, a paired user, physical access only).

## 3. Mechanism test plan

For each mechanism, the RF Swift tests that produce evidence, and the weakness
classes to set on findings (`weakness`, see `map_compliance`). Test only within
the RF scope and with the operator's approval for anything active.

### [ACM] Access control
- Which functions each interface exposes without authentication: GATT services
  and characteristics, Zigbee/Z-Wave commands, network services (port scan of the
  device on its network), local web UI, debug consoles.
- Debug interfaces: UART console, JTAG/SWD, bootloader prompt
  (`rfswift-hardware-interfaces`).
- Classes: `missing-authentication`, `debug-interface`, `excessive-exposure`.

### [AUM] Authentication
- Default, weak or shared passwords and PINs; whether the user must change them;
  brute-force limits (attempt counting, delays) on every login path, including
  BLE pairing PINs and local web UIs.
- Authentication of radio commands: replay and freshness (`rfswift-subghz-ism`,
  `rfswift-iot-lpwan-mesh`), cloneable credentials (`rfswift-rfid-nfc`).
- Classes: `default-credentials`, `missing-authentication`, `replay`,
  `cloneable-credential`, `weak-wifi-security`.

### [SUM] Secure update
- How updates arrive (OTA over Wi-Fi/BLE/Zigbee, app, USB) and whether their
  authenticity and integrity are verified: capture an update, check signatures in
  the firmware (`rfswift-firmware-analysis`), test acceptance of a modified or
  older image only on a spare unit with approval.
- Classes: `insecure-update`, `boot-integrity`.

### [SSM] Secure storage
- Secrets at rest: flash read-out (`rfswift-hardware-interfaces`), read-out
  protection state, secrets in the extracted file system, plaintext credentials.
- Classes: `readable-storage`, `hardcoded-keys`.

### [SCM] Secure communication
- Every link that carries security, network, privacy or financial assets: Wi-Fi
  security mode and PMF, BLE pairing mode (LE Secure Connections versus legacy or
  Just Works), Zigbee/Thread key handling at join, sub-GHz encryption and
  freshness, cellular ciphering, and TLS to the cloud (network image tools).
- Classes: `cleartext-communication`, `replay`, `weak-cryptography`,
  `unprotected-management-frames`, `privacy-tracking`.

### [RLM] Resilience
- Whether the device recovers from malformed or excessive traffic and from loss
  of connectivity. Protocol-level robustness tests (malformed frames, protocol
  fuzzing) only with explicit approval and spare units; never jamming.
- Classes: `denial-of-service`, `input-validation`, `unprotected-management-frames`.

### [NMM] Network monitoring and [TCM] Traffic control
- Mostly relevant to network equipment (routers, gateways, access points): whether
  it can detect and limit attacks or abnormal traffic it relays. Check
  applicability first; test with the network image tools on a lab network.

### [CCK] Confidential cryptographic keys
- Whether keys are unique per device (compare two units), generated with enough
  entropy, protected against extraction, and not defaults (Zigbee
  `ZigBeeAlliance09` link key, MIFARE default keys, shared LoRaWAN AppKeys).
- Classes: `hardcoded-keys`, `readable-storage`.

### [GEC] General equipment capabilities
- Up-to-date software without publicly known exploitable vulnerabilities:
  component versions from the firmware (SBOM with syft or the strings and package
  databases of the image); the operator matches CVEs. Exposed interfaces kept to
  what is needed. Input validation of exposed parsers.
- Classes: `outdated-components`, `excessive-exposure`, `input-validation`.

### [CRY] Cryptography
- Algorithms and modes in use on every link and in storage: no broken primitives
  (Crypto1, KeeLoq, WEP, DES, MD5 for security), adequate key lengths.
- Classes: `weak-cryptography`.

### Part 2 privacy mechanisms: [LGM] logging, [DLM] deletion, [UNM] user notification
- Mostly the app, the cloud and the documentation. From RF Swift: whether radio
  identifiers allow tracking (static addresses, probe requests, advertising data),
  whether personal data leaves the device unprotected, whether a factory reset
  really deletes data (check flash before and after on a spare unit).
- Classes: `privacy-tracking`, `cleartext-communication`.

## 4. Evidence matrix

Keep a matrix in `board.md`, one row per mechanism and part:

| Mechanism | Part | Applies | Tested how | Result | Findings |
|---|---|---|---|---|---|
| [AUM] | 1 | yes | default creds on web UI and BLE PIN brute-force limit | weakness | F-3, F-7 |
| [SUM] | 1 | yes | update captured, signature checked in firmware | no weakness observed | - |
| [NMM] | 1 | no | not network equipment | n/a | - |

Results are one of: `weakness` (with finding ids), `no weakness observed` (with
what was tested), `not tested` (with why), `n/a` (with why). "No weakness
observed" is not "compliant": it says what was tested and nothing more.

## 5. Findings

- One finding per weakness with proof and control case
  (`rfswift-findings-qualification`), its `weakness` classes set, so the report's
  compliance table lists the EN 18031 mechanisms it relates to.
- Describe impact in the standard's terms when it helps the reader: which asset is
  exposed (security, network, privacy, financial) and through which interface.
- Never state that the equipment is or is not compliant; state which mechanisms
  showed weaknesses and what was not assessed. The mapping tables in RF Swift work
  at mechanism level and are a starting point for the report.
