---
name: rfswift-rfid-nfc
description: LF (125/134 kHz) and HF (13.56 MHz) RFID and NFC assessment from an RF Swift mission, mainly with the Proxmark3 Iceman client (interactive pm3 session or one-shot pm3 -c). Covers hw version, hw tune, lf search, hf search, hf 14a info, MIFARE Classic key recovery (hf mf autopwn, chk, fchk, nested, hardnested, darkside), MIFARE Ultralight/NTAG (hf mfu info), DESFire, iCLASS, SEOS, EM4100/EM410x, HID Prox, T5577 (lf t55xx detect), clone and write commands, magic cards, simulation and emulation, firmware/client mismatch, libnfc (nfc-list, nfc-scan-device, nfc-mfclassic), mfoc, mfcuk, mfoc-hardnested, ChameleonUltra, PC/SC readers (pcsc_scan), NDEF and phones, relay concepts, evidence (dumps .bin/.json/.eml, keys via save_secret) and typical access-control findings such as cloneable badges and default keys.
---
# RFID and NFC with RF Swift

## Scope
Covers: identifying card technology, reading IDs and memory, key recovery on MIFARE Classic,
security review of Ultralight/NTAG, DESFire, iCLASS, SEOS, LF proximity credentials (EM410x, HID Prox,
Indala, AWID, T5577 clones), reader behaviour (accepts clones/emulation), NFC NDEF tags and phones.
Does not cover: smart card applet security beyond enumeration (GlobalPlatform/APDU fuzzing), payment
cards (EMV) beyond identification, long-range readers at UHF (860-960 MHz), hardware attacks on readers.
Bluetooth-based access control is in `rfswift-bluetooth-ble`.

## Ground rules
- Only cards and readers the client owns and named in scope. A badge carried by an employee is read
  only with that person's knowledge and the operator's OK.
- Reading is free: an RFID reader's field (`lf search`, `hf search`, `nfc-list`) is not treated as TX.
  Sniffing (`hf 14a sniff`, `lf sniff`) is passive.
- Commands that MODIFY a card need an explicit OK naming the card (UID) and the operation, and the
  card must be a blank or a target card owned by the client: `lf em 410x clone`, `lf hid clone`,
  `lf t55xx write`/`wipe`, `hf mf wrbl`, `hf mf restore`, `hf mf cload`, `hf mf csetuid`, `gen3uid`,
  `hf mfu wrbl`/`restore`, `hf mfdes` create/delete/changekey, `nfc-mfclassic w|W|f`,
  `mifare-classic-format`, NDEF writes, `gp` install/delete/lock. Writing block 0, access bits or keys
  can brick a card permanently: dump first, re-check the block layout.
- Simulation/emulation impersonates a credential (`lf em 410x sim`, `lf hid sim`, `hf 14a sim`,
  `hf mf sim`, `hf iclass sim`, ChameleonUltra slots, Flipper emulation): explicit OK, client's reader only.
- Brute-force or dictionary attacks against a reader (e.g. `lf em 410x brute`, `lf hid brute`) can lock
  out or alarm: explicit OK and the client's site security informed.
- Flashing the Proxmark (`pm3-flash-all`, bootrom) changes the operator's hardware: ask first.
- Relay attacks: discuss as a concept and remediation only; do not set one up unless explicitly scoped.
- The 125 kHz/13.56 MHz field is not radio transmission in the sense of the RF transmission rule,
  but simulation still requires the OK above.

## Tools in RF Swift (verify with `command -v`)
| Tool | Docker image | Nix env |
|---|---|---|
| Proxmark3 Iceman client: `pm3` wrapper, `proxmark3`, `pm3-flash-*`; `pm5` (Proxmark 5 build) | `rfid` | `rfid` (`proxmark3`, `proxmark5`) |
| libnfc: `nfc-list`, `nfc-scan-device`, `nfc-poll`, `nfc-mfclassic`, `nfc-mfultralight` | `rfid` | `rfid` |
| `mfoc`, `mfcuk`, `mfoc-hardnested`, `libnfc_crypto1_crack` (miLazyCracker) | `rfid` | `rfid` |
| libfreefare tools (`mifare-desfire-info`, `mifare-classic-format` = destructive) | `rfid` | `rfid` (`libfreefare`) |
| `pcscd`, `pcsc_scan`, `gp` (GlobalPlatformPro) | `rfid` | `rfid` (`pcsclite`, `pcsc-tools`, `global-platform-pro`) |
| `nfcpy` (`python3 -m nfc`), mfdread (`/rftools/rfid/mfdread/mfdread.py`) | `rfid` | `rfid` |
| ChameleonUltra CLI (`chameleon_cli`) | `rfid`: listed but may be missing, check | `rfid` (`chameleon-ultra-cli`) |
| `ufbt`, `qFlipper` (Flipper Zero), RFIDler (x86_64) | `rfid` | `rfid` |
| nfc-laboratory (SDR NFC sniffer/decoder) | `sdr_full` (`/rftools/sdr/nfc-lab`) | `rfid`, `sdr_full` |
Not shipped by RF Swift: PCSC/opensc tools beyond pcsc-tools, a dedicated SEOS toolkit, relay tooling.
Start with `recommend_tools {task: "mifare classic proxmark"}`.

## Phase 0: reader detection
```sh
ls /dev/ttyACM* /dev/ttyUSB* 2>/dev/null; lsusb
pm3 --list                                  # detected Proxmark ports
pm3 -p /dev/ttyACM0 -c "hw version"         # client and device firmware versions
pm3 -c "hw tune"                            # LF/HF antenna voltage and OK/marginal/unusable
nfc-scan-device -v; nfc-list                # libnfc readers (ACR122U, PN532)
```
- The container needs the serial device mapped (`/dev/ttyACM0`); Nix missions need the udev rules
  (`rfswift env udev <env>` on the host, operator action).
- `hw version` warns when client and firmware come from different builds. Many commands then fail or
  misbehave. Fix = flash matching firmware (`pm3-flash-all`), only with the operator's OK.
  PM3 Easy/generic boards need a firmware built for the generic platform; RDV4 features (flash,
  smartcard slot) are missing on them.
- Iceman and the old official firmware use different syntax, and Iceman itself moved from positional
  arguments to flags around 2020-2021. Before any command, run `<command> -h` (or `help` in a category,
  e.g. `hf mf help`) and use what the installed client prints.

## How to run the Proxmark client
- One-shot (non-interactive, good for logs): `pm3 -c "hf search"`; several commands:
  `pm3 -c "hw version; hf search"`. Run from the mission workspace so output files land there.
- Interactive: `session_start {name: "pm3", command: "pm3"}`, then
  `session_send {name: "pm3", input: "hf search", wait_seconds: 10, expect: "pm3 -->"}`.
  Long attacks (`hf mf hardnested`, `autopwn`): raise `wait_seconds` and keep reading with
  `session_send {input: "", enter: false}`. Stop with input "\u0003", enter false, or press the
  Proxmark button.
- Check where files are saved: `prefs show` (dump/trace save paths). Move results into `dumps/`.

## Phase 1: identify the technology
Place the card flat on the antenna, then:
```
lf search            # EM410x, HID Prox, Indala, AWID, T55xx, Hitag, ...
hf search            # ISO14443A/B, MIFARE, iCLASS, LEGIC, FeliCa, ...
hf 14a info          # UID, ATQA, SAK, ATS, PRNG type (weak/hard/static), magic card detection
```
SAK 08/18/09 = MIFARE Classic 1K/4K/Mini; 00 = Ultralight/NTAG; 20 + ATS = ISO 14443-4 (DESFire,
SEOS, payment, JavaCard). Then technology commands: `hf mfu info`, `hf mfdes info`, `hf iclass info`,
`hf seos info`, `lf em 410x reader`, `lf hid reader`, `lf t55xx detect`.
libnfc/PC-SC alternative: `nfc-list`, `timeout 15 pcsc_scan` (needs `pcscd` running:
`pcscd --foreground --debug &` if no daemon), `python3 -m nfc` (nfcpy diagnostics).

## Phase 2: technology-specific checks
LF proximity (EM410x, HID Prox, Indala, AWID, ioProx): fixed IDs, no authentication. Reading the ID
is enough to clone. Record format and value (HID: format such as H10301, facility code, card number).
T5577 cards: `lf t55xx detect` (block 0 config), `lf t55xx dump`; a password-protected T5577 needs
`-p <pwd>`; `lf t55xx chk` tries common passwords.
MIFARE Classic (Crypto1, broken):
```
hf mf autopwn                         # default keys, then nested/hardnested/darkside as needed; saves keys + dump
hf mf fchk --1k -f mfc_default_keys   # fast dictionary check (also hf mf chk)
hf mf nested --1k --blk 0 -a -k FFFFFFFFFFFF          # one known key -> others (weak PRNG)
hf mf hardnested --blk 0 -a -k <known key> --tblk 4 --ta   # hardened PRNG (EV1, Plus SL1); long
hf mf darkside                        # no known key, weak PRNG only
hf mf dump --1k -f dumps/<name>       # with recovered key file
```
Recent cards with static encrypted nonces (some FM11RF08S clones) need dedicated recovery in newer
Iceman builds: check `hf mf help` for `isen` and related scripts. libnfc path: `mfoc -O dumps/<name>.mfd`
(nested, needs one known key; `-k <key>` adds keys), `mfoc-hardnested`, `mfcuk` (darkside, slow),
`nfc-mfclassic r a u dumps/<name>.mfd dumps/<name>_keys.mfd` (read with A keys, any UID).
Inspect a dump: `python3 /rftools/rfid/mfdread/mfdread.py dumps/<name>.mfd` (sectors, access bits).
Look for: default or known keys, the same keys on every card (site-wide key), value blocks for
credit/stored value, MAD/NDEF sectors, identical UID-derived data.
Ultralight / NTAG: `hf mfu info` (type, lock bits, password protection, originality signature).
Ultralight EV1/NTAG21x passwords are 32-bit and sent in clear: a `hf 14a sniff` of a reader session
followed by `trace list -t 14a` shows PWD_AUTH. Some products derive the password from the UID:
`hf mfu pwdgen -r`. Ultralight C uses 3DES authentication; check whether the factory default key works.
DESFire EV1/EV2/EV3: `hf mfdes info` (version, key settings, free directory listing), list applications
(`hf mfdes lsapp`/`getaids` depending on build). Strong when keys are diversified and not default. The
common weakness is the reader: many systems read only the UID. Random-UID setting hides tracking.
iCLASS: `hf iclass info` shows legacy/standard vs Elite vs SE. Legacy standard-security credentials use a
widely known master key (index 0 in the client's default key list): `hf iclass dump --ki 0`. Elite
keys can be recovered from a reader with loclass (simulation, needs OK). iCLASS SE/SEOS: identify
only; check whether the reader still accepts legacy iCLASS or 125 kHz credentials (downgrade).
ChameleonUltra (if `chameleon_cli` is present): reader mode to read, slots to emulate (emulation needs
the OK above); see `help` in its CLI. Flipper Zero: same rules for its emulation features.
NFC NDEF/phones: `nfc-list`, nfcpy, `hf mfu ndefread` / `hf mf ndefread` (Iceman names; check help).
Phone-based credentials (HCE) are out of scope unless the client lists the app.

## Phase 3: clone, emulate, prove (modifying, OK required)
Only with a client-owned blank or test card and the reader the client designates.
```
lf em 410x clone --id <10 hex>                      # writes a T5577 by default
lf hid clone -w H10301 --fc <fc> --cn <cn>
hf mf csetuid -u <uid>   |   hf mf cload -f dumps/<name>.eml    # Gen1a magic; Gen2/Gen3 differ (check help)
hf mf restore --1k -f dumps/<name>-dump.bin -k dumps/<name>-key.bin
lf em 410x sim --id <id>   |   hf 14a sim -t 1 -u <uid>   (emulation)
```
Proof protocol: (1) present the original card: access granted; (2) present the blank before writing:
denied; (3) present the clone/emulation: granted; (4) operator checks the access control log shows the
original credential ID. Record each step with UTC time. Wipe or hand over clones as the operator says
and note it.

## Reading results
- "No known/supported 13.56 MHz tag found" can mean antenna detuning (`hw tune`), card too far, metal
  surface, or two cards in the field. Test one card at a time.
- Magic card detection in `hf 14a info` matters before writing: Gen1a needs `cload`/`csetuid`,
  Gen2 (CUID) accepts normal block 0 writes, Gen3/Gen4 have their own commands.
- A static nonce or "hard" PRNG explains why nested/darkside fail; switch method instead of retrying.
- LF readers often accept a clone even when data is identical but modulation differs slightly; failure
  of one clone is not proof that the system resists cloning: check T5577 config with `lf t55xx detect`.

## Evidence
- Dumps under `dumps/`: `dumps/hf-mf-<UID>-dump.bin|.json` (older builds also `.eml`), key files
  `dumps/hf-mf-<UID>-key.bin`, traces `dumps/<UTC>_<card>_14a.trace`, libnfc `.mfd`.
  File names carry UID, technology and UTC date; the Workbench classifies `.eml`, `.mfd`, `.dump`,
  `.json` as Proxmark artifacts.
- Recovered keys and passwords: `save_secret {label: "MFC key A sector 1 card <UID>", kind: "other",
  value: <exact key>, source: "dumps/hf-mf-<UID>-key.bin, hf mf autopwn"}`. A badge number
  (EM ID, HID FC/CN) is itself a credential: store it as a secret and refer to it by label.
- Mission note (append): reader hardware and firmware/client versions, exact commands, card
  technology and UID per card, which keys were default vs recovered (labels only), every write or
  simulation with its OK, proof steps with times.

## Typical findings
Titles: "<site/system>: <credential technology> <weakness> allows <impact>".
- Cloneable LF proximity badge grants door access. Confirmed: proof protocol above passed.
  Candidate: ID read only. `CVSS:3.1/AV:P/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:N` (7.3). Fix: migrate to
  DESFire EV2/EV3 or SEOS with diversified keys, disable 125 kHz on readers.
- MIFARE Classic with default or recoverable keys protecting access data or stored value:
  `CVSS:3.1/AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N` (6.1); S:C (7.3) when it opens doors. Confirmed: keys
  recovered and data read (and, with OK, a modified/cloned card accepted).
- Same sector keys across all cards or sites (key reuse): raises every other finding's impact;
  confirmed with two or more cards. Fix: key diversification per card.
- Reader authenticates only the UID (DESFire/NTAG/ISO14443 UID) or accepts emulation: 7.3 as above.
- iCLASS legacy standard keys: cloneable credential, 7.3.
- NTAG/Ultralight EV1 password sniffable or UID-derived: `CVSS:3.1/AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N`
  (4.6), more if the tag gates access.
- Multi-technology reader accepts a downgraded legacy credential: report with the reader config.
Remediation patterns: modern secure credentials with mutual authentication and diversified keys,
reader configuration to reject legacy technologies, secure channel between reader and controller,
anti-passback and logging, distance-bounding/proximity check against relay.

## Pitfalls
- Two Proxmark clients on one port fail with "port busy": close the session before one-shot commands.
- pcscd grabs ACR122U-style readers: libnfc then reports no device; stop pcscd for libnfc work.
  The host kernel `pn533_usb` module can also claim the reader: host action, ask the operator.
- Phones near the antenna act as readers/cards and confuse results.
- Writing access bits wrong locks sectors permanently: verify the trailer block before `wrbl`.
- `hf mf hardnested` needs RAM and minutes; run it in a session, not a 60 s command.
- Do not trust a "clone written" message: read the clone back and compare with the original dump.
