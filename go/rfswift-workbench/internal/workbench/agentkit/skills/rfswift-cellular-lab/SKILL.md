---
name: rfswift-cellular-lab
description: 2G/3G/4G/5G security testing with RF Swift telecom images and Nix envs: passive survey (kal/kalibrate, gr-gsm grgsm_scanner, grgsm_livemon, grgsm_decode, GSMTAP in Wireshark, srsRAN cell_search, LTESniffer), UE/modem diag capture (QCSuper, SCAT for Qualcomm/Samsung), private lab networks (srsRAN 4G srsenb/srsepc, OCUDU and srsRAN Project gnb, OpenAirInterface rfsim, Open5GS EPC/5GC, UERANSIM, YateBTS, OpenBTS, Osmocom osmo-bts/osmo-bsc/osmo-msc), test SIMs (pySim, sysmo-usim-tool, PC/SC), shielded-enclosure TX rules, EEA0/EIA0/NEA0/NIA0 null algorithms, identity requests, SUCI, IMSI catcher and 2G downgrade research, paging, baseband testing, S1AP/NGAP/NAS pcaps, ARFCN/EARFCN/NR-ARFCN conversion. Use for eNB/gNB/BTS labs, IoT cellular modules, test phones.
---
# Cellular lab: passive survey, private 2G-5G networks, UE and SIM testing

## Scope
- Covers: receive-only survey of cells in an authorised setting, diagnostic capture from test phones and modules, private lab networks (2G, 3G, 4G, 5G NSA/SA) built with RF Swift to test UEs, IoT modules and SIMs, configuration review of the client's own (private/campus) networks.
- SDR detection, drivers, UHD images and clock checks: `rfswift-sdr-setup`; wideband occupancy surveys: `rfswift-spectrum-survey`.
- Not covered: SS7/Diameter/GTP signalling attacks (SigPloit, jSS7, bromelia, py5sig and 5Greplay are shipped but out of this skill), module firmware (`rfswift-firmware-analysis`), UART/USB access to a module's debug ports (`rfswift-hardware-interfaces`), telematics CAN side (`rfswift-automotive-can`).

## Ground rules (hard)
- Base stations and UEs transmit in licensed spectrum. Radiating programs here: `srsenb`, `srsue`, `gnb` (OCUDU, srsRAN Project bladeRF), `nr-softmodem`/`nr-uesoftmodem` (OAI, unless `--rfsim`), YateBTS (`yate` with ybts/`mbts`), OpenBTS (`transceiver`), `osmo-trx-*` with `osmo-bts-trx`, OpenBTS-UMTS, srsRAN examples such as `pdsch_enodeb`, any GNU Radio flowgraph with a sink.
- They run only into a shielded enclosure (Faraday cage or RF shield box) or over coax with attenuators between SDR and UE, never over the air, with the lowest TX gain that works. The test UEs sit inside the enclosure.
- Each TX run needs all of:
  - the operator has enabled "Allow RF transmission" for the agent in the Workbench;
  - an explicit OK in chat for that specific run;
  - `transmits: true` on `execute_command`, a `max_seconds` limit, and a `purpose` with band, ARFCN/EARFCN/NR-ARFCN and centre frequencies in MHz, TX gain or power, duration, enclosure/cabling and target UE. Example: "TX LTE B7 EARFCN 3350 (DL 2680.0 / UL 2560.0 MHz), B210 tx_gain 40, 20 min, shield box SB-1 with 60 dB coax, test UE 'pixel-lab-1', PLMN 001/01".
  - Changing frequency, gain, PLMN or enclosure is a new run: new OK.
- Without MCP the same rule holds: you do not start a transmitter without the operator's explicit OK for that run.
- Before the first TX run of a session, the operator checks containment: with the transmitter on at the planned gain, a receiver or phone outside the enclosure must not detect the lab cell. If it does, stop.
- Identities: test PLMN only (MCC 001 / MNC 01, or what the client's licence allows); never broadcast a real operator's PLMN, never attract or hold third-party phones, never block emergency calls. Only client-owned test UEs and programmable test SIMs; personal SIMs out of the devices.
- IMSI-catcher, downgrade, null-cipher and reject-message research: only in the shielded lab with own UEs.
- Passive reception of public networks: decoding broadcast information (MIB/SIB, cell identity, ARFCNs) for a survey is usually acceptable within scope, but decoding or recording other subscribers' signalling, identities (IMSI/TMSI in paging, identity responses) or traffic can be unlawful interception. Do it only on the lab cell or the client's own network with written scope; never store third-party identifiers; tell the operator if you captured any by accident.
- SIM writes (pySim-prog, pySim-shell `update_*`/`edit_*`, sysmo-usim-tool writes) need an OK. Use the ADM key from the card vendor's data sheet only: wrong ADM attempts consume a retry counter and can permanently block card administration.
- Regulation is the operator's responsibility (national regulator rules; some countries require an authorisation even for shielded tests). Record the authorisation reference in the note.

## When to stop and ask the operator
- Before every TX run; when the enclosure, cabling or attenuation is unknown; when containment was not checked.
- When a UE outside the test set appears in logs (unknown IMSI/IMEI): stop the cell and report.
- When an SDR, phone or SIM reader is not detected, or a run needs `sudo`, host kernel modules (`sctp`), or network access (UHD images, vulnerability databases).

## Tools in RF Swift
- Docker images (all telecom images build on `telecom_utils`, itself on the `sdrsa_devices` driver layer: UHD, bladeRF, LimeSuite, HackRF, SoapySDR):
  - `telecom_utils`: pycrate, CryptoMobile, pySim (`/telecom/SIM/pysim`: `pySim-shell.py`, `pySim-read.py`, `pySim-prog.py`), sysmo-usim-tool (`/telecom/SIM/sysmo-usim-tool`), pcscd and `pcsc_scan`, SCAT (`scat`), QCSuper (`qcsuper`), Modmobmap (`/telecom/Modmobmap`), SigPloit, jSS7, bromelia, py5sig; `kal` (kalibrate-hydrasdr, HydraSDR RFOne only).
  - `telecom_2Gto3G`: YateBTS (bladeRF), OpenBTS (UHD, x86_64), OpenBTS-UMTS (x86_64), Osmocom core `osmo-msc`, `osmo-hlr`, `osmo-mgw`, `osmo-sgsn`, `osmo-ggsn`, `osmo-stp` with sample configs in `/telecom/2G/osmocom`. The Docker install script does not build `osmo-bts`, `osmo-bsc` or `osmo-trx`: check `command -v osmo-bsc osmo-bts-trx`; the Nix `telecom` env has them.
  - `telecom_4G_5GNSA` (profile `telecom`): srsRAN 4G (`srsenb`, `srsepc`, `srsue`; example programs such as `cell_search` stay in the build tree: `find /telecom/4G -type f -name cell_search`), Open5GS (`/telecom/5G/open5gs`, all-in-one `Open5Gs_deployall`, TUN setup `/sdrtools/run/Open5GS.sh`).
  - `telecom_4Gto5G`: srsRAN 4G, OCUDU (`gnb`), Open5GS.
  - `telecom_5G` (profile `telecom-5g`): OCUDU `gnb`, Open5GS, UERANSIM (`/telecom/5G/UERANSIM/build/nr-gnb`, `nr-ue`, `nr-cli`), 5Greplay.
  - `telecom_5G_bladerf`: srsRAN Project bladeRF fork (`gnb`), Open5GS, Wireshark/tshark.
  - `telecom_5G_oai`: OAI `nr-softmodem`, `nr-uesoftmodem` (USRP), Open5GS.
  - `telecom_ltesniffer`: `LTESniffer` (USRP).
  - `telecom_5G_train`: the 4G/5G NSA image plus Open5GS `nohttp2` and `0caps` builds (`Open5Gs_nullciph_deployall`), the null-cipher UERANSIM build, Wireshark/tshark, GNU Radio.
  - `sdr_full`: gr-gsm (`grgsm_scanner`, `grgsm_livemon`, `grgsm_livemon_headless`, `grgsm_capture`, `grgsm_decode`); the only RF Swift environment with gr-gsm.
  - Every image has `tcpdump`; Wireshark/tshark only in `telecom_5G_train`, `telecom_5G_bladerf` and `network`.
- Nix environments:
  - `telecom`: srsran, ocudu, open5gs plus `open5gs_nohttp2`/`open5gs_0caps`, ueransim plus `ueransim_nullciph`, yate, yatebts, openbts, openbts-umts;
  - Osmocom in `telecom`: osmo-bsc, osmo-bts, osmo-trx, osmo-msc, osmo-hlr, osmo-sgsn, osmo-ggsn, osmo-pcu, osmo-mgw, osmo-sip-connector;
  - utilities in `telecom`: kalibrate-rtl (`kal`, RTL-SDR), pysim, sysmo-usim-tool, pycrate, cryptomobile, scat, qcsuper, modmobmap, wireshark with telecom dissectors, GNU Radio;
  - `telecom_ltesniffer`, `telecom_5g_bladerf`, `telecom_5g_oai` mirror their Docker images. No gr-gsm in Nix.
- Profiles `telecom` and `telecom-5g`: host network, realtime, `NET_ADMIN`/`NET_RAW`/`NET_BIND_SERVICE`, `/dev/net/tun`, USB bus.
- Not shipped by RF Swift: FALCON, other LTE control-channel sniffers, any 5G NR passive sniffer, gr-lte, IMSI-catcher scripts, OsmoNITB (legacy; use the split Osmocom stack), upstream srsRAN Project (replaced by OCUDU and the bladeRF fork), PyHSS (its install function currently installs bromelia only; check `command -v pyhss`), baseband/modem flashing tools.
- Checks:
```sh
for t in kal grgsm_scanner grgsm_livemon_headless LTESniffer srsenb srsepc srsue gnb nr-softmodem nr-gnb nr-ue open5gs-amfd Open5Gs_deployall yate osmo-msc osmo-bsc osmo-bts-trx qcsuper scat pcsc_scan tcpdump tshark; do printf '%-24s %s\n' "$t" "$(command -v $t || echo -)"; done
uhd_find_devices; bladeRF-cli -p; LimeUtil --find; hackrf_info; SoapySDRUtil --find   # whichever SDR is attached
adb devices; ls -l /dev/ttyUSB* /dev/ttyACM* 2>/dev/null; lsusb                         # test phones and modules
```

## Frequency helpers
- GSM900: DL = 935.0 + 0.2 x ARFCN (1-124); E-GSM 975-1023: DL = 935.0 + 0.2 x (ARFCN - 1024); UL = DL - 45. DCS1800 (512-885): DL = 1805.2 + 0.2 x (ARFCN - 512); UL = DL - 95.
- LTE: DL = F_low + 0.1 x (EARFCN - N_offs): B1 2110/0, B3 1805/1200, B7 2620/2750, B8 925/3450, B20 791/6150, B28 758/9210. UL EARFCN = DL EARFCN + 18000 for FDD bands (B7 3350 -> 21350 = 2560.0 MHz).
- NR: NR-ARFCN below 600000: F = 0.005 x N MHz; 600000-2016666: F = 3000 + 0.015 x (N - 600000) MHz.

## Workflow
### Phase 0: lab plan (write it to the note before any RF)
- Authorisation reference, location, enclosure model and measured isolation, cabling and total attenuation, SDR and clock reference (GPSDO or 10 MHz; LTE/NR need well under 1 ppm), PLMN, band and channels, maximum TX gain, list of test UEs (labels) and test SIMs (labels; keys stored as secrets), what each test is meant to show.

### Phase 1: passive survey (receive-only; mind the scope caveat above)
- GSM band scan and clock calibration (`kal` is kalibrate-hydrasdr in Docker images, kalibrate-rtl in Nix `telecom`):
```sh
kal -s GSM900 -g 40                  # bands: GSM850, GSM-R, GSM900, EGSM, DCS, PCS
kal -c 62 -g 40                      # clock offset in ppm against a strong channel, reuse it as -p below
```
- gr-gsm (`sdr_full` image): `grgsm_scanner -b GSM900 --args="rtl=0" -g 40 -p <ppm>` lists ARFCN, frequency, CID, LAC, MCC/MNC and power. Live decode of one carrier to GSMTAP (UDP 4729 on loopback), two background jobs with `max_seconds`:
```sh
tcpdump -i lo -w captures/gsmtap_GSM900_arfcn62_947.4M_2026-11-24T10-05Z.pcap udp port 4729
grgsm_livemon_headless -f 947.4M -g 40 -p <ppm> --args="rtl=0"
```
  IQ for offline work: `grgsm_capture` (output argument form differs between versions, check `--help`) to a `.cfile` (complex float32), then `grgsm_decode -c <file>.cfile -s <rate> -f <freq> -m BCCH -t 0`. Many countries have switched 2G/3G off: an empty scan can be normal.
- LTE cell search (srsRAN 4G example, any SDR srsRAN supports): `<path>/cell_search -b 3 -a "<device args>" -g 50` (prints PCI, PRBs, ports, power). LTESniffer (USRP): run it from a fresh directory because it writes pcaps into the working directory, e.g. `mkdir -p captures/ltesniffer_2026-11-24T10-05Z && cd captures/ltesniffer_2026-11-24T10-05Z && LTESniffer -A 2 -W 4 -f 1840e6 -C -m 0` (downlink; uplink mode adds `-u <UL freq> -m 1` and needs tight synchronisation; check `LTESniffer -h` and its README). Its output contains other subscribers' identifiers: lab cell or written scope only.
- 5G NR: no passive sniffer is shipped; observe NR through the UE side (next bullet) or the lab gNB/core logs.
- UE-side diagnostics on a client-owned test phone or module (receive-only from the agent's point of view; the UE talks to whatever network it is on):
  - QCSuper (Qualcomm): `qcsuper --adb --pcap-dump captures/qcsuper_pixel-lab-1_2026-11-24T10-05Z.pcap` (rooted Android) or `--usb-modem <diag tty>` for modules; options change between versions, check `qcsuper --help`.
  - SCAT (Qualcomm `-t qc`, Samsung `-t sec`): `scat -t qc -s /dev/ttyUSB0` (serial diag) or `scat -t qc -u -a <bus:addr> -i <interface>` (USB); it streams GSMTAP to 127.0.0.1:4729, capture with tcpdump as above; newer versions can write pcap directly (`scat --help`).
  - AT commands on module ports (`picocom -b 115200 /dev/ttyUSB2` in a session): read-only ones (`ATI`, `AT+COPS?`, `AT+CEREG?`, `AT+CSQ`, vendor serving-cell queries) are fine; configuration changes (`AT+CFUN`, RAT/band locks, vendor `AT+QCFG`-style settings) need an OK and must be reverted.

### Phase 2: lab network without RF (no TX gate, still lab-only)
- Start here: core and NAS behaviour can be tested with a simulated radio.
- 5G SA with Open5GS + UERANSIM (Docker `telecom_5G`; paths for Nix differ, use `command -v`):
```sh
/sdrtools/run/Open5GS.sh                                     # ogstun 10.45.0.1/16 + NAT (needs NET_ADMIN)
mongod --dbpath /data/db --fork --logpath logs/mongod_2026-11-24T10-05Z.log    # subscriber DB, if mongod is present
/telecom/5G/open5gs/misc/db/open5gs-dbctl add 001010000000001 <K> <OPc>        # values from the test SIM sheet (store as secrets)
Open5Gs_deployall                                            # all NFs from the build tree: background job
tcpdump -i any -w captures/core_ngap_gtpu_2026-11-24T10-05Z.pcap 'sctp or udp port 2152 or tcp port 7777'
/telecom/5G/UERANSIM/build/nr-gnb -c /telecom/5G/UERANSIM/config/open5gs-gnb.yaml       # background job
/telecom/5G/UERANSIM/build/nr-ue -c /telecom/5G/UERANSIM/config/open5gs-ue.yaml         # creates uesimtun0
```
  NF configs are YAML files in the Open5GS build tree (`ls /telecom/5G/open5gs/build/configs/ /telecom/5G/open5gs/build/configs/open5gs/`); NGAP is SCTP 38412, S1AP SCTP 36412, GTP-U UDP 2152, SBI HTTP/2 (often 7777).
- 4G with srsRAN over ZeroMQ instead of an SDR (if the RF plugin exists: `ls /usr/local/lib/libsrsran_rf_zmq*`): `srsenb ... --rf.device_name=zmq --rf.device_args="fail_on_disconnect=true,tx_port=tcp://*:2000,rx_port=tcp://localhost:2001,id=enb,base_srate=23.04e6"` with `srsue` using the mirrored ports; the UE runs in its own network namespace (`--gw.netns=ue1`). Example configs: `srsran_install_configs.sh user` copies them to `~/.config/srsran/`.
- OAI: `--rfsim` replaces the radio with a TCP link between `nr-softmodem` and `nr-uesoftmodem` (sample configs under `/telecom/5G/openairinterface5g/targets/PROJECTS/GENERIC-NR-5GC/CONF`); flags change between OAI tags, check `--help`.
- Null-algorithm and capability tests on the core side: the `0caps` Open5GS build and the null-cipher UERANSIM build (Docker `telecom_5G_train`, Nix `telecom`).

### Phase 3: shielded RF lab (TX gate applies to every start)
- Prepare configs first, gains low, PLMN 001/01, then ask for the OK with the full `purpose`.
- 4G (srsRAN): EPC then eNB, everything logged into the workspace through command-line overrides:
```sh
srsepc ~/.config/srsran/epc.conf --pcap.enable=true --pcap.filename=/workspace/captures/epc_s1ap_nas_2026-11-24T10-05Z.pcap --log.filename=/workspace/logs/epc_2026-11-24T10-05Z.log
srsenb ~/.config/srsran/enb.conf --rf.dl_earfcn=3350 --rf.tx_gain=40 --pcap.enable=true --pcap.filename=/workspace/captures/enb_mac_2026-11-24T10-05Z.pcap --log.filename=/workspace/logs/enb_2026-11-24T10-05Z.log
```
  `srsepc` alone does not radiate; `srsenb` does (`transmits: true`, `max_seconds`). Override names follow the `[section] key` layout of the .conf files; check `srsenb --help`. Subscribers: `user_db.csv` (keys: secrets).
- 5G (OCUDU/srsRAN Project `gnb -c gnb.yaml`, OAI `nr-softmodem -O <conf>`) against Open5GS: same procedure; the gNB is the transmitter.
- 2G: YateBTS (configs in `/usr/local/etc/yate/`, `ybts.conf`: band, C0 ARFCN, MCC/MNC, power attenuation), OpenBTS, or in Nix the Osmocom chain `osmo-trx-*` + `osmo-bts-trx` + `osmo-bsc` + `osmo-msc` + `osmo-hlr` + `osmo-mgw` (+ `osmo-stp`). Cipher policy in `osmo-msc.cfg` (`network` node: `encryption a5 ...`, `authentication required`). The transceiver/BTS process is the transmitter.
- Stop transmitters with `cancel_job` (or `max_seconds`) as soon as the test is done; confirm with `list_jobs` that nothing radiates.

### Phase 4: tests on UEs, modules and SIMs
- Configuration matrix to exercise (core side, one change per run):
  - Open5GS `amf.yaml`/`mme.yaml` `security: integrity_order` / `ciphering_order` (NIA*/NEA* for 5G, EIA*/EEA* for 4G); srsEPC `[mme] integrity_algo`, `encryption_algo`; srsENB `[expert] eea_pref_list`, `eia_pref_list`; gNB `cu_cp` security preferences (names vary by release, check the sample config).
  - Identity procedures before authentication (IMSI/SUCI, IMEI/IMEISV), authentication reject, attach/registration reject causes, TAU reject, RRC release with redirection to 2G/3G.
  - Paging: identities used (S-TMSI/5G-S-TMSI vs IMSI), GUTI reallocation after procedures.
- What to observe for each run: UE capability IEs (supported EEA/EIA, NEA/NIA), Security Mode Command and the UE's Complete or Reject (cause #23 UE security capabilities mismatch, #24 security mode rejected), whether the UE continues to registration accept and data, which identity it sends in Identity Response, RAT changes after rejects, and how long the UE stays off the network.
- SIM review with a PC/SC reader: `pcscd` must run (`pcscd -f` as a background job if no service), `pcsc_scan` lists readers, `cd /telecom/SIM/pysim && ./pySim-read.py -p 0`. `./pySim-shell.py -p 0` is interactive (session); reading files such as `EF.SUCI_Calc_Info` under `ADF.USIM/DF.5GS` shows the SUCI protection scheme. Programming test SIMs (`pySim-prog.py`, `pySim-shell` updates) is a write: OK, vendor ADM key, values recorded as secrets.

## Reading results
- No attach: PLMN or K/OPc mismatch (check core logs for MAC failure / SQN sync failure), UE not allowing a test network (try manual network selection), band not supported by the UE, wrong EARFCN, clock error too large (cell found but RACH fails), SCTP missing on the host (`modprobe sctp` is a host action), core not listening.
- A UE advertising EIA0/NIA0 is normal (needed for emergency calls). The finding is a UE completing a normal registration and carrying data with null integrity.
- 4G Identity Request for IMSI before authentication is standard behaviour; disclosure is a 5G finding when the SUCI uses the null scheme (protection scheme 0) outside test SIMs, or an IMEI sent before security on any generation.
- In GSM the network chooses ciphering and does not authenticate itself: a UE camping on a lab 2G cell with A5/0 is expected; what matters is whether the device allows 2G at all, warns, or can be pushed there by rejects/redirects.
- Rejections without integrity protection that keep a UE off service until reboot are a denial-of-service result: measure the duration.

## Evidence
- `captures/`: GSMTAP pcaps `gsmtap_<band>_arfcn<N>_<MHz>M_<UTC>.pcap`; core pcaps `core_s1ap_nas_<UTC>.pcap`, `core_ngap_gtpu_<UTC>.pcap`; RAN pcaps `enb_mac_<UTC>.pcap`; UE diag `qcsuper_<ue>_<UTC>.pcap`, `scat_<ue>_<UTC>.pcap`; LTESniffer run directories.
- IQ recordings: `captures/947.4M_1Msps_<UTC>_arfcn62.cf32` (gr-gsm `.cfile` is cf32) with a SigMF sidecar `.sigmf-meta`:
```json
{"global":{"core:datatype":"cf32_le","core:sample_rate":1000000,"core:version":"1.0.0","core:hw":"RTL-SDR v4","core:description":"lab BTS ARFCN 62 BCCH, shield box SB-1"},
 "captures":[{"core:sample_start":0,"core:frequency":947400000,"core:datetime":"2026-11-24T10:05:00Z"}],"annotations":[]}
```
- `logs/`: eNB/gNB/BTS and core logs, `logs/configs_<UTC>/` copies of every config used for a run (without subscriber keys), `mongod`/NF logs, AT and pySim session logs.
- Mission note (`write_note`, `append: true`): per TX run the OK reference, frequencies, gain, start/stop UTC, enclosure and attenuation, containment check result; per test the configuration change, UE and SIM labels, frame numbers in the pcaps that prove the behaviour.
- K, OPc, Ki, ADM keys, PINs/PUKs: `save_secret` with the source (SIM sheet, config file and line). IMSIs of test SIMs may appear in notes as labels plus the last digits.

## Typical findings
- "UE accepts null integrity protection (EIA0/NIA0) for normal service". Confirmed: core configured to offer only EIA0/NIA0 (config snapshot), NAS Security Mode Command with it, UE Security Mode Complete, registration accept and user data (pcap frame numbers from core and UE diag). `CVSS:3.1/AV:A/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:N` (6.8, medium). Remediation: UE/baseband must reject null integrity outside emergency sessions.
- "SUPI disclosed in clear: SUCI null protection scheme" (5G) or "IMEI sent before NAS security". Confirmed: Registration Request/Identity Response with the clear identity (frame numbers) and the SIM's SUCI configuration read with pySim. `AV:A/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N` (4.3). Remediation: provision home-network public keys (Profile A/B), send IMEI only after security.
- "Device falls back to unauthenticated 2G" / "downgrade by unprotected reject or redirect accepted". Confirmed in the shield: LTE/NR reject or redirect message, followed by the UE camping and registering on the lab 2G cell (UE diag + BTS/MSC logs). `AV:A/AC:H/PR:N/UI:N/S:U/C:H/I:N/A:N` (5.3). Remediation: disable 2G where not needed (module configuration, OS setting), handle unprotected rejects per 3GPP timers.
- "Unprotected reject messages cause prolonged denial of service". Confirmed with measured outage duration. `AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H` (6.5).
- "Private network core offers null ciphering/integrity" (client's network configuration review). Confirmed: config file plus a Security Mode Command selecting NEA0/EEA0 or NIA0/EIA0 in a capture. `AV:A/AC:H/PR:N/UI:N/S:U/C:H/I:N/A:N` (5.3). Remediation: order NEA2/NIA2 (or 1/3) first and remove the null algorithms except where emergency service requires them.
- Save with `save_finding` (stored as draft; the operator confirms): `observation` with configs, pcap names and frame numbers; `poc` with the lab setup and steps; include the regulatory context (shielded lab, test PLMN) in `description`.

## Pitfalls
- Host networking: `lo` is the host's loopback, so a GSMTAP capture also sees other host processes using UDP 4729, and core ports (36412, 38412, 2152, 7777) clash with other missions or host services.
- Clock: uncalibrated SDRs find cells but UEs never attach; use a GPSDO/10 MHz reference for LTE/NR and `kal` ppm values for gr-gsm.
- USRP B2x0 needs USB 3 for LTE/NR sample rates; missing FPGA images produce "Could not find path for image" (the operator runs `uhd_images_downloader`, network).
- CPU: realtime profile helps, but frequency scaling and other load cause underruns (`U`/`L` in UHD output) and dropped attaches.
- MongoDB not running means Open5GS starts but every attach fails authentication.
- Shield boxes leak through cables and USB; put attenuators on every RF line and keep TX gain low even inside.
- Commercial phones cache the last registered network and may need airplane-mode toggles or manual selection to see the lab PLMN.
