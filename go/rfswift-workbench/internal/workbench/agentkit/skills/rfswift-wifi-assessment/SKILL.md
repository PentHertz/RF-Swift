---
name: rfswift-wifi-assessment
description: Wi-Fi (IEEE 802.11) security assessment from an RF Swift mission. Covers adapter and monitor mode handling (iw dev, iw list, airmon-ng check/start, USB Wi-Fi adapter passthrough, host networking side effects), passive recon with airodump-ng and Kismet on 2.4/5/6 GHz, WPA2-PSK 4-way handshake and PMKID capture (hcxdumptool, hcxpcapngtool, hashcat -m 22000, aircrack-ng), WPS (wash, reaver, bully, pixiewps), WPA3-SAE and transition mode checks, PMF (802.11w), WPA2/WPA3-Enterprise EAP method and identity analysis, rogue AP and evil twin tools (eaphammer, hostapd-mana, hostapd-wpe), deauthentication with aireplay-ng, client probe requests and PNL privacy, captive portals, evidence (pcapng, .22000) and typical findings. Use for wlan, monitor mode, handshake, PSK cracking, WPS, SAE, EAP, PEAP, rogue AP.
---
# Wi-Fi assessment with RF Swift

## Scope
Covers: 802.11 discovery, configuration review from the air (encryption, AKM, PMF, WPS, EAP methods),
PSK handshake and PMKID capture with offline cracking, WPS, WPA3 transition checks, Enterprise client
behaviour, client privacy (probe requests), captive portals.
Does not cover: Bluetooth (use `rfswift-bluetooth-ble`), 802.15.4 / Zigbee / Thread at 2.4 GHz
(use `rfswift-iot-lpwan-mesh`), IP-layer pentest after you are on the network (general network
methodology), SDR decoding of 802.11 with GNU Radio (gr-ieee802-11 exists in `sdr_full`, rarely needed).

## Ground rules
- Work only on SSIDs/BSSIDs the operator lists as in scope. Write the scope (SSIDs, BSSIDs, channels,
  site, time window) into the mission note before the first capture. Neighbouring networks are seen
  but never targeted; do not store their client MACs beyond what the survey needs.
- Receiving is free: monitor mode, airodump-ng, Kismet, `wash` in survey mode, reading pcaps.
- A normal adapter's scanning (probe requests from `iw dev wlanX scan`) is not treated as TX.
- TX (hard gate): `aireplay-ng` (deauth, fake auth, injection test `-9`), `mdk3`/`mdk4`, `hcxdumptool`
  in its default active mode, `reaver`/`bully`, `wacker`, `dragondrain`, rogue/evil-twin AP
  (`hostapd`, `hostapd-mana`, `hostapd-wpe`, `airbase-ng`, `eaphammer`, `wifiphisher`, `fluxion`,
  `wifipumpkin3`, `airgeddon` attack menus). For each one you need:
  1. the operator enabled "Allow RF transmission" for the agent;
  2. an explicit OK in chat for this specific action (target BSSID/client, channel, count, duration);
  3. `transmits: true` on `execute_command`, with channel/frequency, tx power (`iw dev X info`
     shows txpower), duration and target in `purpose`.
  Wi-Fi is unlicensed spectrum but still regulated: stay within the regulatory domain (`iw reg get`),
  never raise tx power or change the regdomain to unlock channels.
- Disruptive actions (deauth, WPS brute force that can lock the AP, SAE flooding, Karma/known-beacon
  modes that lure any nearby device) need the OK to name the impact and the time window.
  Karma/MANA "respond to every probe" modes impersonate networks outside scope: refuse unless the
  operator explicitly scopes them and the venue is controlled.
- Association attempts to client networks (testing a recovered PSK, EAP_buster, captive portal tests)
  are active authentication against client infrastructure: ask first.
- Never upload hashes or captures to online cracking services.

## Tools in RF Swift (verify with `command -v`)
| Tool | Docker image | Nix env |
|---|---|---|
| `iw`, `rfkill`, `usbutils` | every image (corebuild) | `wifi` (`iw`) |
| aircrack-ng suite (`airmon-ng`, `airodump-ng`, `aireplay-ng`, `aircrack-ng`) | `wifi` | `wifi` |
| `hcxdumptool`, `hcxpcapngtool`, `hcxhashtool`, `hcxpmktool` | `wifi` | `wifi` |
| `hashcat`, `john` | `wifi` (hashcat comes with airgeddon's deps) | `wifi`, `network` |
| `kismet`, `kismetdb_to_pcap`, `kismetdb_dump_devices` | `wifi` (from `network` base) | `wifi`, `network` |
| `tshark`/`wireshark`, `tcpdump`, `macchanger` | `wifi` | `wifi` (`wireshark`, `macchanger`), `network` (`tcpdump`) |
| `wash`, `reaver`, `bully`, `pixiewps` | `wifi` | `wifi` (`reaverwps-t6x`) |
| `mdk4`, `mdk3` | `wifi` | `wifi` (`mdk4`) |
| `eaphammer` (`/rftools/wifi/eaphammer/eaphammer`), `hostapd-mana`, `hostapd-wpe`, `wpa_sycophant`, `EAP_buster`, `asleap` | `wifi` | `wifi` |
| `dragonslayer`, `dragondrain`/`dragontime`, `wacker`, `dragonforce` (WPA3 research PoCs) | `wifi` | `wifi` |
| `bettercap`, `airgeddon`, `wifite`, `sparrow-wifi`, `horst` (Nix only), `cowpatty` | `wifi` | `wifi` |
First call `recommend_tools {task: "wifi monitor mode handshake"}`, then run commands with `execute_command`.
Presence check: `command -v airodump-ng hcxdumptool hcxpcapngtool hashcat kismet wash reaver tshark`.

## Phase 0: adapter, capabilities, environment
```sh
iw dev                                  # interfaces, phy index, type, channel
iw list | sed -n '/Wiphy phy1/,/Wiphy phy2/p' | grep -A12 'Supported interface modes'
iw phy phy1 info | grep -E 'MHz|Band'   # bands; "(disabled)" or "(no IR)" = passive only
iw reg get
airmon-ng                               # PHY, driver, chipset per interface
lsusb                                   # confirm the USB adapter model
```
- You need "monitor" in Supported interface modes. Injection support is only proven by a TX test
  (`aireplay-ng --test`), which needs the TX gate; for passive work you do not need it.
- Good USB chipsets: MediaTek mt76 (MT7612U, MT7921AU for 6 GHz), Atheros AR9271 (2.4 GHz only),
  Realtek RTL8812AU with out-of-tree driver (monitor works, quirks). Internal laptop cards often
  miss frames or refuse monitor mode.
- Containers: the Wi-Fi interface lives in a network namespace, not in `/dev`. With host networking
  the container sees and changes the host's real interfaces: any `ip link`/`iw` change affects the
  host. With bridge/nat networking the interface is invisible unless the operator moves the phy
  into the container (`iw phy phy1 set netns <container pid>` on the host). The lab also needs
  `NET_ADMIN` and `NET_RAW` (`capsh --print` if available, or just try `ip link set`).
- Nix missions run natively as the operator's user: root commands need `sudo`/`rfsudo`. If sudo asks
  for a password, stop and ask the operator.
- Use a dedicated USB adapter, never the interface carrying the operator's uplink.

## Phase 1: monitor mode (prefer iw)
```sh
ip link set wlan1 down
iw dev wlan1 set type monitor
ip link set wlan1 up
iw dev wlan1 set channel 6 HT20         # or: iw dev wlan1 set freq 5180 80 5210
iw dev wlan1 info                       # type monitor, channel, txpower
```
Restore afterwards: `ip link set wlan1 down; iw dev wlan1 set type managed; ip link set wlan1 up`.
`airmon-ng check` only lists interfering processes (safe). `airmon-ng check kill` kills
NetworkManager and wpa_supplicant: in a Nix mission or a container sharing the host PID namespace
that is the host's Wi-Fi stack and can cut the operator's connectivity. Do not run it without an
explicit OK; instead ask the operator to unmanage the adapter on the host
(`nmcli device set wlan1 managed no`). `airmon-ng start wlan1 [channel]` creates `wlan1mon`
on most mac80211 drivers; record the new name. Stop with `airmon-ng stop wlan1mon`.

## Phase 2: passive recon (receive only)
Survey all bands, then lock on in-scope channels. airodump-ng is a curses program: run it as a
background job with a hard limit and read the files, not the screen.
```sh
mkdir -p captures logs
airodump-ng wlan1 --band abg --manufacturer --wps --uptime \
  -w captures/wifi_survey_abg_2026-11-24T10-05Z --output-format pcap,csv --write-interval 10
```
`execute_command {command: "...", purpose: "Passive 2.4/5 GHz survey on wlan1, 10 min, no TX",
wait_seconds: 5, max_seconds: 600}`, then `job_output`. airodump appends `-01.cap`/`-01.csv`.
Targeted capture on one AP: `airodump-ng wlan1 -c 36 --bssid AA:BB:CC:DD:EE:FF -w captures/<name> --output-format pcap,csv`.
- `--band a` needs a 5 GHz adapter. 6 GHz: most airodump builds cannot select it by band letter;
  try `-C 5955-7115` (MHz list) or use Kismet/hcxdumptool; check `airodump-ng --help`.
- Kismet (sets monitor mode itself, logs a sqlite `.kismet` file, web UI on localhost:2501):
  `kismet -c wlan1 -p "$PWD/logs" -t site-survey` as a background job; afterwards
  `kismetdb_dump_devices --in logs/<file>.kismet --out logs/<name>_devices.json` and
  `kismetdb_to_pcap --in logs/<file>.kismet --out captures/<name>.pcapng` (check `--help` of each).
- `wash -i wlan1` (survey mode is the default and passive; `-s` scan mode sends probes) or offline
  `wash -f captures/<file>.cap` lists WPS state, version and "Lck" (locked).

Configuration from beacons (offline, on the capture):
```sh
tshark -r captures/X.cap -Y 'wlan.fc.type_subtype == 0x0008' -T fields \
  -e wlan.bssid -e wlan.ssid -e wlan_radio.channel -e wlan.rsn.akms.type \
  -e wlan.rsn.pcs.type -e wlan.rsn.capabilities.mfpc -e wlan.rsn.capabilities.mfpr | sort -u
```
AKM: 1 = 802.1X, 2 = PSK, 5 = 802.1X-SHA256, 6 = PSK-SHA256, 8 = SAE, 9 = FT-SAE, 12 = Suite-B-192,
18 = OWE. PSK + SAE on one BSS = WPA3 transition mode. `mfpr=1` = PMF required, `mfpc=1, mfpr=0` =
optional. No RSN element with privacy bit = WEP or open; pairwise cipher 2 = TKIP.
Client privacy (probe requests, PNL):
```sh
tshark -r captures/X.cap -Y 'wlan.fc.type_subtype == 0x0004 && wlan.ssid != ""' \
  -T fields -e wlan.sa -e wlan.ssid | sort | uniq -c | sort -rn
```
Second hex digit of a MAC in {2,6,A,E} = locally administered (randomized). Report on in-scope
devices only (corporate SSID names probed by staff devices), not on bystanders.
Enterprise, passive:
```sh
tshark -r captures/X.cap -Y eap -T fields -e wlan.bssid -e wlan.sa -e eap.code -e eap.type -e eap.identity
```
EAP type 13 = TLS, 21 = TTLS, 25 = PEAP, 17 = LEAP, 43 = FAST, 52 = pwd. The outer identity is in
clear; if it is a real `user@domain` instead of `anonymous@domain`, note it (identities can also be
listed with `hcxpcapngtool -I logs/<name>_identities.txt`). The RADIUS server certificate is visible in
TLS 1.2 PEAP/TTLS handshakes: `tshark -r X.cap -Y tls.handshake.certificate -O tls | grep -iE 'issuer|subject'`.

## Phase 3: WPA2-PSK handshake and PMKID
Passive first: keep airodump-ng locked on the target channel and BSSID and wait for real clients
to (re)connect. A complete capture needs EAPOL M1+M2 (or M2+M3) of one exchange.
Active options (TX gate, OK per BSSID):
- Targeted deauth of one client to force a reconnect (see Phase 6).
- hcxdumptool: it associates to APs and disassociates clients by default to collect PMKIDs and
  handshakes from everything it hears. Flags changed a lot between 6.2.x, 6.3.x and 7.x: run
  `hcxdumptool --help` first and restrict it to scope. In 6.3.x the scope is a BPF filter:
  ```sh
  hcxdumptool -m wlan1                      # 6.3.x: set monitor mode and exit
  tcpdump -i wlan1 'wlan addr3 AA:BB:CC:DD:EE:FF or wlan addr2 AA:BB:CC:DD:EE:FF' -ddd > logs/scope.bpf
  hcxdumptool -i wlan1 -c 6a --bpf=logs/scope.bpf -w captures/wifi_ch6_2026-11-24T10-30Z_AABBCC.pcapng --tot=10
  ```
  (`-c` needs a band suffix: a = 2.4 GHz, b = 5 GHz, c = 6 GHz; `--tot` is minutes.) A receive-only
  mode exists in some versions (`--silent` in older 6.x, `--disable_*` switches in 6.3); only treat
  a run as passive if `--help` of the installed version confirms it. Otherwise it is TX.
Convert and inspect:
```sh
hcxpcapngtool -o captures/<name>.22000 -E logs/<name>_essids.txt captures/<name>.pcapng   # also reads .cap
hcxhashtool -i captures/<name>.22000 --info=stdout
```
Line format: `WPA*01*` = PMKID, `WPA*02*` = EAPOL message pair; the last field encodes which messages
were combined and whether the AP authorized the client.
Offline cracking (background job, long-running):
```sh
hashcat -m 22000 -a 0 captures/<name>.22000 <wordlist> -r /usr/share/hashcat/rules/best64.rule --status --status-timer 60
hashcat -m 22000 captures/<name>.22000 --show
aircrack-ng -w <wordlist> -b AA:BB:CC:DD:EE:FF captures/<name>-01.cap      # alternative, CPU
```
Wordlists: ask the operator; SecLists is under `/opt/fuzzing/SecLists` in the `wifi` image. Do not
write cracked output into the workspace (`-o`); read it with `--show` and store it with
`save_secret {label, kind: "wifi", value, source: "captures/<name>.22000, hashcat -m 22000"}`.
The `.22000` file itself stays an evidence file in `captures/`, not a secret, and is not pasted in notes.
Without an OpenCL device hashcat stops ("No devices found"): `hashcat -I`, use `-D 1` for CPU, or hand
the hash file to the operator's GPU rig.

## Phase 4: WPS
`wash` shows WPS 1.0/2.0 and lock state. Pixie Dust (offline nonce weakness) is a few frames:
`reaver -i wlan1 -b <BSSID> -c <ch> -K -vv` or `bully wlan1 -b <BSSID> -c <ch> -d -v 3`
(TX, OK required). Online PIN brute force (`reaver` without `-K`) takes hours, triggers lockouts and
can reboot cheap APs: only with explicit OK and a time window. Check `reaver -h`: the Pixie flag form
differs between builds. A recovered PIN and PSK go to `save_secret`.

## Phase 5: WPA3-SAE, Enterprise, captive portals
- SAE itself is not crackable offline from a capture. Risks: transition mode (same passphrase on a
  WPA2 AKM, so a WPA2 handshake from any client cracks it), clients that accept a WPA2-only
  impersonating AP (downgrade; proving it needs a rogue AP, TX + impersonation), old
  hostapd/wpa_supplicant versions affected by Dragonblood side channels (dragonslayer/dragontime are
  research tools; dragondrain is a DoS). `wacker` is an online SAE brute force (TX, OK required).
- Enterprise rogue AP (eaphammer, hostapd-wpe, hostapd-mana + wpa_sycophant relay) tests whether
  clients validate the RADIUS certificate. It impersonates the client's SSID and harvests employee
  credentials: needs written authorization naming the SSIDs/BSSIDs, location, time window, and the
  TX gate. Example once approved: `cd /rftools/wifi/eaphammer && ./eaphammer --cert-wizard`, then
  `./eaphammer -i wlan1 --channel 6 --auth wpa-eap --essid <in-scope SSID> --creds`
  (check `./eaphammer -h`). MSCHAPv2 challenge/response cracks with `hashcat -m 5500` or `asleap`;
  only a cracked exact value goes to `save_secret`.
- `EAP_buster <SSID> <identity> <interface>` enumerates accepted EAP methods by authenticating with
  wpa_supplicant: active, uses an identity, may lock accounts. Ask first.
- Captive portal / guest (after OK to associate): DNS resolution before login, client isolation
  (ping/ARP to another test client you control), MAC-cloning bypass with `macchanger` (impersonates
  an authenticated client: separate OK), portal served over plain HTTP.

## Phase 6: deauthentication (TX, disruptive)
Only after an OK that names BSSID, client MAC, count and time. Lock the channel first.
```sh
iw dev wlan1 set channel 6
aireplay-ng --deauth 3 -a AA:BB:CC:DD:EE:FF -c 11:22:33:44:55:66 wlan1
```
`purpose`: "Deauth 3 frames, ch6 2437 MHz, txpower 20 dBm, BSSID AA:BB..., client 11:22..., ~1 s".
Never `--deauth 0` (continuous) or broadcast deauth without naming every affected client. With PMF
required (`mfpr=1`) deauth is ignored; say so instead of repeating it.

## Reading results
- airodump "WPA handshake: <BSSID>" appears for partial or failed exchanges; trust hcxpcapngtool's
  summary of usable message pairs instead.
- A pair where the AP never sent M3 may come from a client typing a wrong passphrase: hashcat then
  "recovers" the typo. Prefer PMKID or authorized pairs, and confirm a PSK with an approved
  association or with `hcxpmktool` (check `--help`) against an authorized pair.
- Hidden SSIDs show as length tags in beacons; the name appears in probe/association frames.
- Low PWR (below -85 dBm) means frames are missed: move closer instead of retrying TX.

## Evidence
- Captures: `captures/wifi_<band-or-ch>_<UTC>_<bssid-short>.pcapng|.cap`, csv next to them, Kismet db in `logs/`.
- Hash files: `captures/<capture-name>.22000`. Secrets (PSK, WPS PIN, cracked EAP password) only
  through `save_secret`; refer to them by label in notes and findings.
- Mission note (`write_note`, append): scope, adapter/chipset/driver, regdomain, exact commands, file
  names, UTC times, channel plan, every TX action with its OK and parameters, host state changes
  (monitor mode, killed processes) and their restoration.

## Typical findings
Titles: "<SSID>: <weakness> allows <impact>". Save with `save_finding` (status draft); include
observation, poc (commands + files), remediation.
- Weak WPA2-PSK recovered offline. Confirmed: cracked from an authorized pair or PMKID and verified.
  Candidate: hash captured, not cracked. `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N` (8.1).
  Fix: 20+ random characters, per-device keys (iPSK/PPSK), or 802.1X, or WPA3-SAE only.
- WPS Pixie Dust/PIN discloses the PSK: same vector (8.1). Fix: disable WPS.
- Enterprise clients accept an untrusted RADIUS certificate. Confirmed only with an authorized rogue
  AP capture. `CVSS:3.1/AV:A/AC:H/PR:N/UI:N/S:C/C:H/I:H/A:N` (8.0). Fix: enforce server cert and name
  validation via MDM/GPO, EAP-TLS.
- PMF not required: `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:L` (4.3); confirmed by config
  (beacon RSN capabilities) plus an approved deauth test. Fix: PMF required (mandatory for WPA3).
- WPA3 transition mode without transition-disable: low/informational unless a downgrade was demonstrated.
- Outer EAP identity or probe requests leak usernames/corporate SSIDs:
  `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N` (4.3). Fix: anonymous outer identity, remove stale profiles.
- Guest network without client isolation or with portal bypass:
  `CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:L/I:L/A:N` (5.4).
- WEP/TKIP/open corporate network: report with observed cipher; severity from what the network carries.

## Pitfalls
- Monitor mode on a host-networked container or a Nix mission changes the host; restore it and say so.
- NetworkManager on the host silently reverts monitor mode: ask the operator to unmanage the adapter.
- Channel hopping misses handshakes; lock on the AP channel for capture.
- DFS and "no IR" channels: receive only, never force TX there.
- USB 2 ports and long cables drop frames on 5 GHz with 80 MHz; use a short cable/USB 3 port.
- After a container restart the `wlan1mon` name may disappear; re-run `iw dev`.
- Cracking jobs: set `max_seconds`, report progress from `--status` lines, cancel with `cancel_job`.
