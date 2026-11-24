---
name: rfswift-firmware-analysis
description: Offline analysis of firmware images from flash dumps or vendor downloads in RF Swift: identification (file, binwalk signatures, entropy -E, strings, xxd), extraction (binwalk v2 vs Rust binwalk v3/binwalkv3, unblob, sasquatch/unsquashfs for SquashFS, UBI/UBIFS, JFFS2, CramFS, uImage/FIT with dtc, ext4, Android sparse, 7z, dd by offset), root filesystem triage (passwd/shadow, inittab, init scripts, web roots, CGI, setuid, hard-coded credentials, private keys, certificates, API tokens, gitleaks), binary analysis (checksec, pwn checksec, readelf, rabin2/rz-bin, radare2, rizin, Ghidra analyzeHeadless, capa, floss, yara, unicorn, angr), SBOM and component versions (syft, grype, trivy), bootloader, secure boot and OTA update review (signatures, rollback), with evidence and finding patterns. Use for .bin/.img/.trx/.itb dumps, MCU flash images, router/camera/IoT firmware.
---
# Firmware analysis: identify, extract, triage, reverse

## Scope
- Covers: images obtained from `rfswift-hardware-interfaces` dumps (SPI/eMMC/MCU flash, U-Boot `md` dumps) or vendor downloads and OTA captures; identification, extraction, filesystem and binary review, boot/update mechanism review, component inventory.
- Not covered: getting the dump (`rfswift-hardware-interfaces`), testing the live device's network services or radios (network or RF skills, e.g. `rfswift-subghz-ism`, `rfswift-cellular-lab`), ECU diagnostics (`rfswift-automotive-can`).
- Everything here is offline: no RF transmission, no contact with the device. Steps that reach the device (logging in with found credentials, uploading a modified image) leave this skill and need an explicit OK.

## Ground rules
- Authorisation and confidentiality: vendor firmware is often under NDA. Keep it in the mission workspace. Never upload images, binaries or extracted files to online services (sandboxes, online decompilers, hash lookups, LLM pastes) without the operator's OK.
- Extracted content is untrusted data:
  - never execute target binaries or scripts natively, never `source` firmware scripts, never `cd` into an extracted tree and run an interpreter there (planted `*.py` modules): pass paths as arguments and run Python with `-I`;
  - absolute symlinks inside a rootfs (`etc/passwd -> /tmp/passwd`) point into the container when you `cat` them: check with `ls -l`/`readlink` and resolve them against the rootfs yourself.
- Credentials and keys: store each exact observed value with `save_secret {label, kind, value, source}` where `source` is the file path inside the extraction plus line/offset. Notes, findings and reports say only "stored as secret <label>". Keep grep outputs that print values out of saved logs (save file lists with `grep -l` instead).
- CVEs: never write a CVE ID from memory. Record component and version evidence; scanner matches are candidates for the operator to confirm.
- Ask the operator before: offline password cracking (needs another environment and CPU/GPU time), vulnerability-database downloads (grype/trivy need network), any step that would modify or contact the device.

## Tools in RF Swift
- Docker image `reversing` (profile `reversing`: NAT network, desktop, `SYS_PTRACE`):
  - identification/extraction: `file`, `xxd`, `hexyl`, `strings`/`readelf`/`objdump` (binutils), `binwalk` (distribution package: v2 or v3 depending on the base, check), `binwalkv3` (Rust binwalk v3), `unblob` (not on riscv64), `sasquatch`, `qnx6-extractor`, `7z`, `dtc`, `unrar`, `cabextract`, `lz4`, `lzop`, `zstd`, `simg2img`, `debugfs` (e2fsprogs), sleuthkit, `bkcrack` (ZipCrypto);
  - reversing: Ghidra (`ghidraRun`, headless at `/reverse/ghidra_*_PUBLIC/support/analyzeHeadless`), radare2 (x86_64 only), Cutter, ImHex, gdb with pwndbg/`gef` (native arch only), pwntools (`pwn`), ROPgadget, ropper, `checksec`, capa, floss, yara, Detect-It-Easy (`diec`), angr, unicorn, keystone, kaitai-struct;
  - code review/fuzzing: semgrep, cppcheck, joern, AFL++, honggfuzz; SBOM/vulns: trivy.
- Nix environment `reversing`: ghidra, radare2, rizin, cutter, binwalk, unblob, sasquatch, `7zz`, imhex, checksec, gdb, gef, pwndbg, pwntools, ropgadget, capa, flare-floss, detect-it-easy, yara, unicorn, keystone, angr, aflplusplus, semgrep, cppcheck, joern, trivy, grype, syft, gitleaks, foremost, testdisk, sleuthkit, ddrescue, kaitai-struct-compiler, rehex, hexedit.
- Not shipped by RF Swift: qemu-user-static / qemu-system (so no running extracted binaries or full-system emulation), FirmAE, EMBA (its install step is disabled in the image build), Qiling (disabled too), u-boot-tools (`dumpimage`/`mkimage`), gdb-multiarch, standalone `jefferson` and `ubireader_*` (unblob handles JFFS2/UBI itself; in Docker they may exist inside unblob's pipx venv: `ls ~/.local/share/pipx/venvs/unblob/bin/`). OFRAK is opt-in only (licence) and must not be enabled in a published image. Password crackers (hashcat, john) live in the Docker `wifi` image and the Nix `network`/`wifi` environments.
- Check, and ask `recommend_tools {task: "firmware extraction"}` if something is missing:
```sh
for t in file binwalk binwalkv3 unblob sasquatch unsquashfs 7z 7zz dtc strings readelf checksec pwn r2 rabin2 rizin rz-bin ghidraRun analyzeHeadless ghidra-analyzeHeadless capa floss yara diec syft grype trivy gitleaks; do printf '%-22s %s\n' "$t" "$(command -v $t || echo -)"; done
binwalk --help 2>&1 | head -5        # v3: clap-style "Usage: binwalk [OPTIONS] [FILE_NAME]"; v2: "Binwalk v2.x" banner
unblob --show-external-dependencies   # missing extractors
df -h .                               # extraction can need 10x the image size
```

## Workflow
### Phase 0: intake
- Put the original in `dumps/` (or reference the hardware dump), never modify it, work on copies in `dumps/work/`.
```sh
mkdir -p dumps/work dumps/extract logs
sha256sum dumps/fw_v1.2.3.bin | tee -a dumps/SHA256SUMS
```
- Note provenance: dumped from which chip and how (or vendor URL, download date, published version and checksum), and which images belong together (bootloader, kernel, rootfs, MCU blobs).

### Phase 1: identification
```sh
file dumps/fw_v1.2.3.bin
binwalk dumps/fw_v1.2.3.bin | tee logs/binwalk_sig_fw_v1.2.3_2026-11-24T10-05Z.txt
strings -n 8 dumps/fw_v1.2.3.bin | grep -iE 'u-boot 20|linux version|busybox v|squashfs|openwrt|copyright|firmware' | head -50
xxd dumps/fw_v1.2.3.bin | head -32
```
- Entropy: `binwalk -E dumps/fw_v1.2.3.bin` (v2 plots with matplotlib, add `--nplot` headless and read the printed rising/falling edges; v3 writes a PNG next to the input or in the output directory). Text fallback, one value per 64 KiB block:
```sh
python3 -I -c 'import sys,math,collections
d=open(sys.argv[1],"rb").read(); B=int(sys.argv[2],0)
for o in range(0,len(d),B):
    b=d[o:o+B]; n=len(b)
    print(f"{o:#010x} {abs(sum(v/n*math.log2(v/n) for v in collections.Counter(b).values()))/8:.3f}")' dumps/fw_v1.2.3.bin 0x10000 > logs/entropy_fw_v1.2.3_2026-11-24T10-05Z.txt
```
- Magic bytes worth knowing: uImage `27 05 19 56` (64-byte header), FIT/DTB `d0 0d fe ed`, SquashFS `hsqs`/`sqsh`, UBI `UBI#`, JFFS2 `85 19`/`19 85`, CramFS `45 3d cd 28`, gzip `1f 8b`, xz `fd 37 7a 58 5a`, LZMA `5d 00 00`, TRX `HDR0`, Android boot `ANDROID!`, ZIP `PK`. ESP32: partition table at 0x8000, app images start with `e9`. Raw Cortex-M flash: first word is the initial stack pointer (0x2000xxxx), second the reset vector (0x08xxxxxx, odd = Thumb).
- Entropy reading: about 1.0 (above ~0.95 per block) across the whole image with no recognisable headers means encrypted (or one compressed blob); compressed partitions are high too but start with a known header; low bands are code, tables or 0xFF padding. Whole-image encryption: look for the decryption routine in the bootloader, an older unencrypted release, or the vendor update tool; ZipCrypto archives can be attacked with `bkcrack` given known plaintext.

### Phase 2: extraction
- First pass with unblob (recursive, many formats; long-running: background job):
```sh
unblob -e dumps/extract/fw_v1.2.3_unblob --report logs/unblob_fw_v1.2.3_2026-11-24T10-05Z.json dumps/fw_v1.2.3.bin
```
  Flags (`-e/--extract-dir`, `-d/--depth`, `--report`) change between releases: check `unblob --help`.
- binwalk v3 (`binwalkv3`, or `binwalk` when it is v3): `binwalkv3 -e -M -C dumps/extract/fw_v1.2.3_bw3 dumps/fw_v1.2.3.bin` (`-e` extract, `-M` recursive, `-C` output directory, `-c` carve only, `-l` JSON log). It calls external extractors (7z, sasquatch, jefferson, ubireader...) and silently skips the ones missing.
- binwalk v2: `binwalk -e -M -C dumps/extract/fw_v1.2.3_bw2 --run-as=root dumps/fw_v1.2.3.bin`. As root, v2.3+ refuses to run extractors without `--run-as=root`; results land in `_<name>.extracted/` under `-C`.
- Manual carving by offset (offsets from the signature scan; GNU dd):
```sh
dd if=dumps/fw_v1.2.3.bin of=dumps/work/fw_v1.2.3_0x1a0000_squashfs.bin bs=64K iflag=skip_bytes,count_bytes skip=$((0x1A0000)) count=$((0x800000))
sasquatch -d dumps/extract/rootfs_v1.2.3 dumps/work/fw_v1.2.3_0x1a0000_squashfs.bin   # vendor-modified SquashFS too
```
- Per format:
  - SquashFS: `sasquatch` (or `unsquashfs` if present; `-s` prints the superblock: compression, block size, date).
  - UBI/UBIFS and JFFS2: unblob; erase-block size and endianness matter, retry with explicit parameters only if the tool supports them.
  - uImage: payload starts after the 64-byte header; binwalk shows load/entry address and compression.
  - FIT (`.itb`) and device trees: `dtc -I dtb -O dts -o dumps/work/fit_v1.2.3.dts dumps/work/<image>.itb`, then look at `images`, `configurations`, `hash` and `signature` nodes.
  - ext2/3/4: unblob, `7z x`, or `debugfs -R 'ls -l /' <img>`; Android sparse: `simg2img in.img out.raw`; archives: `7z x -odumps/extract/<name> <file>` (`7zz` in Nix).
  - MCU raw blobs: nothing to extract; go to Phase 4 with the base address (e.g. 0x08000000 for STM32 internal flash).
- Locate rootfs trees: `find dumps/extract -maxdepth 6 -type d -name etc -prune -print | head`, then `R=<path to the directory holding bin/ etc/ lib/>`.

### Phase 3: filesystem triage
```sh
ls -la "$R" "$R/etc"; ls -l "$R/etc/passwd" "$R/etc/shadow"            # symlinks?
find "$R" -type l -lname '/*' -printf '%p -> %l\n' | head -50             # absolute links: resolve inside $R
cat "$R/etc/passwd"; cat "$R/etc/shadow" 2>/dev/null
cat "$R/etc/inittab" 2>/dev/null; ls "$R/etc/init.d" "$R/etc/rc.d" 2>/dev/null
find "$R" -type f \( -perm -4000 -o -perm -2000 \) -ls
grep -rIlE 'BEGIN (RSA |EC |DSA |OPENSSH |ENCRYPTED )?PRIVATE KEY' "$R" | tee logs/triage_privkeys_2026-11-24T10-05Z.txt
find "$R" -type f \( -name '*.pem' -o -name '*.key' -o -name '*.crt' -o -name '*.der' -o -name '*.p12' -o -name '*.pfx' -o -name authorized_keys \) | tee logs/triage_keyfiles_2026-11-24T10-05Z.txt
grep -rIilE '(passw(or)?d|passwd|secret|api[_-]?key|token|private[_-]?key)[[:space:]]*[:=]' "$R/etc" "$R/www" "$R/usr" "$R/opt" 2>/dev/null | tee logs/triage_credfiles_2026-11-24T10-05Z.txt
```
- Accounts: shadow/passwd hash formats `$1$` md5crypt, `$5$` sha256crypt, `$6$` sha512crypt, 13-character DES; an empty hash field means no password; `*`/`!` means locked. Check which services use them (telnetd, dropbear/sshd, web login, console `login`).
- Console and services: `inittab` lines such as `::respawn:/bin/sh` or `ttyS0::askfirst:-/bin/sh` give a shell without login; init scripts starting `telnetd`, debug daemons, `dropbear` with password login, services bound to all interfaces; OpenWrt `etc/config/*`; default configuration/NVRAM files (`*.default`, `default.cfg`, `nvram*`).
- Web: `www`, `htdocs`, `web`, `cgi-bin`, server configs (lighttpd, uhttpd, boa, goahead, mini_httpd, nginx), Lua/PHP/shell CGI. In CGI binaries, request parameters flowing into `system`, `popen`, `exec*`, `sprintf`+`system` are command-injection candidates (static only).
- Secrets scan (Nix): `gitleaks dir "$R" --report-path logs/gitleaks_rootfs_v1.2.3_2026-11-24T10-05Z.json` (gitleaks 8.19+; older releases use `gitleaks detect --no-git --source <dir>`; check `--help`; the report contains values, keep it in the workspace only). Review each hit by hand before calling it a secret.
- Keys and certificates without printing key material:
```sh
openssl x509 -in "$R/etc/ssl/device.crt" -noout -subject -issuer -dates -fingerprint -sha256
openssl pkey -in "$R/etc/ssl/device.key" -pubout | sha256sum         # public-key fingerprint to compare across devices/versions
```
- Component versions: `strings "$R/bin/busybox" | grep -m1 'BusyBox v'`, `strings -n 6 "$R"/lib/libssl.so* "$R"/usr/lib/libssl.so* 2>/dev/null | grep -m1 -E '^OpenSSL [0-9]'`, `strings "$R/usr/sbin/dropbear" | grep -m1 -i 'dropbear_'`, `strings <kernel> | grep -m1 'Linux version'`, `cat "$R/etc/openwrt_release" "$R/etc/os-release"`, package databases (`usr/lib/opkg/status`, `var/lib/dpkg/status`).
- SBOM (Nix): `syft dir:"$R" -o spdx-json=logs/sbom_v1.2.3_2026-11-24T10-05Z.spdx.json`. Matching against CVEs (`grype sbom:<file>`, `trivy rootfs "$R"`) downloads a vulnerability database: ask first, save the output under `logs/`, treat matches as candidates.

### Phase 4: binary analysis
- Architecture and format: `file "$R/usr/sbin/httpd"`; `readelf -h -d -l <bin>` (Docker) or `rabin2 -I <bin>` / `rz-bin -I <bin>`.
- Hardening: `checksec --file=<bin>` (bash checksec 2.x; the Go rewrite 3.x uses `checksec file <bin>`, check `checksec --help`) or `pwn checksec <bin>` (any ELF architecture). List all ELF files first: `find "$R" -type f -exec file {} + | grep ELF > logs/elf_list_v1.2.3.txt`.
- Imports and strings: `rabin2 -i -z <bin>` / `rz-bin -i -z <bin>`, `strings -n 6 <bin>`, `floss <bin>` (best on x86), `capa <bin>`, `yara -r <rules.yar> "$R"` with rules the operator provides.
- Batch disassembly (interactive r2/rizin/gdb go in a session):
```sh
r2 -q -c 'aaa; afl' "$R/usr/sbin/httpd" > logs/r2_httpd_functions.txt
rizin -q -c 'aaa; afl' "$R/usr/sbin/httpd" > logs/rz_httpd_functions.txt
r2 -a arm -b 16 -m 0x08000000 -q -c 'aaa; afl' dumps/stm32f103_flash_<UTC>.bin   # raw Thumb blob at its base address
```
- Ghidra headless (long-running: background job, then `job_output`; the operator opens the project with `ghidraRun`):
```sh
AH=$(command -v analyzeHeadless || command -v ghidra-analyzeHeadless || ls /reverse/ghidra_*_PUBLIC/support/analyzeHeadless 2>/dev/null | head -1)
mkdir -p dumps/ghidra
"$AH" dumps/ghidra fw_v1.2.3 -import "$R/usr/sbin/httpd" -overwrite > logs/ghidra_httpd_2026-11-24T10-05Z.log 2>&1
"$AH" dumps/ghidra mcu_fw -import dumps/stm32f103_flash_<UTC>.bin -processor ARM:LE:32:Cortex -loader BinaryLoader -loader-baseAddr 0x08000000 > logs/ghidra_mcu.log 2>&1
```
- Emulation: RF Swift ships no QEMU/FirmAE/EMBA, so stay static. For a small routine (password or key derivation, a decryption loop), emulate just that code with unicorn or angr in Python, inputs from the image. Fuzzing extracted parsers (AFL++, honggfuzz) needs a harness: propose it to the operator first.
- GPL source drops from the vendor: `semgrep` with local rules (registry rulesets need network), `cppcheck`, `joern`.

### Phase 5: boot chain and update mechanism
- Bootloader: `strings <uboot.bin> | grep -E 'U-Boot 20[0-9]{2}|bootcmd=|bootdelay=|bootargs='`; verified boot evidence: FIT images carrying `signature` nodes and a U-Boot control DTB with `/signature/key-*` marked `required`. Unsigned FIT or plain uImage means the bootloader does not authenticate the kernel. Note where the environment lives and whether it is writable.
- Update path: `grep -rlE 'sysupgrade|fwupdate|firmware|upgrade|ota' "$R/usr" "$R/www" "$R/etc" "$R/sbin" | head -50`, then read the handler:
  - integrity only (CRC, MD5, SHA without a key) or real signature checks (`openssl dgst -verify`, `mbedtls_pk_verify`, embedded public keys) and where the public key comes from (writable partition?);
  - transport: `http://` URLs, `curl -k`, `wget --no-check-certificate`, pinned or unpinned TLS;
  - rollback: version comparison, anti-rollback counter, A/B slots that accept older images;
  - encryption with a static key present in the image (then anyone can decrypt and re-encrypt).
- MCU/ESP32: image signature blocks and header fields are visible offline; whether secure boot or flash encryption is enforced is a device property (eFuses, option bytes): check it with `rfswift-hardware-interfaces`.
- "Device accepts a modified image" is only confirmed by flashing one: explicit OK, verified backup, and the operator accepting the brick risk. Until then report it as a candidate from static evidence.

## Reading results and false positives
- binwalk reports many random signatures inside compressed data (absurd LZMA properties, lone JPEG or certificate hits); trust a hit when its header fields are consistent and extraction succeeds.
- Public material is not a secret: CA bundles in `etc/ssl/certs`, library test keys, sample configs, documentation examples. A host key generated at first boot (`dropbearkey`/`ssh-keygen` in init scripts) means the shipped file may be a placeholder.
- Locked (`*`, `!`) accounts and hashes for services that do not exist lower the impact; an empty root hash with a reachable login service raises it.
- Version strings in comments, help texts or unused copies differ from what is linked; confirm with the library actually loaded (`readelf -d`, `rabin2 -l`).
- High entropy with periodic structure or a valid header is compression, not encryption.

## Evidence
- `dumps/`: original images plus `dumps/SHA256SUMS`; carved parts `dumps/work/<image>_<offset>_<type>.bin`; extraction trees `dumps/extract/<image>_<tool>/`; Ghidra projects `dumps/ghidra/`.
- `logs/`: `binwalk_sig_<image>_<UTC>.txt`, `entropy_<image>_<UTC>.txt`, `unblob_<image>_<UTC>.json`, `triage_*_<UTC>.txt` (file lists), `sbom_<image>_<UTC>.spdx.json`, scanner outputs, `r2_*`/`ghidra_*` logs.
- Mission note (`write_note`, `append: true`): provenance and hashes, tool versions (`binwalk --version` or `--help` header, `unblob --version`), each extraction command, rootfs path, notable files with their paths, open questions for the operator.
- Every credential, key, token or PSK: `save_secret` (kinds: password, token, api-key, private-key, wifi, pin, other) with `source: "dumps/extract/fw_v1.2.3_unblob/.../etc/shadow, line 1"`.

## Typical findings
- "Hard-coded credentials in firmware (<account/service>)". Confirmed: file path and line, the service that uses them, and either an offline crack with the operator's OK or a verified login on the device with OK. Candidate: hash present, not cracked or service not reachable. Network service: `CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H` (9.8, critical); console only: `AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H` (6.8). Remediation: per-device credentials set at provisioning or first boot, no shared accounts.
- "Private key shared across devices embedded in firmware (TLS/SSH/VPN/cloud client)". Confirmed: same public-key fingerprint in two firmware versions or devices, or matching the key the device presents (operator check). Never put key material in the finding. `AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:N` (7.4, high). Remediation: per-device keys in secure storage, rotation.
- "Firmware update not authenticated". Candidate from code (CRC/MD5 only, signature check skippable, key in writable storage); confirmed only by an accepted modified image (OK). LAN admin-only upload: `AV:A/AC:L/PR:H/UI:N/S:U/C:H/I:H/A:H` (6.8); unauthenticated OTA: up to `AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H` (9.8). Remediation: asymmetric signatures verified by a trusted boot stage, anti-rollback.
- "Outdated third-party components". Evidence: component, version, path, and the scanner output file; the operator confirms CVE applicability and reachability. Severity follows the confirmed CVE, not the scanner score.
- "Unauthenticated console shell configured in inittab" (supporting evidence for a hardware finding), "Debug services enabled at boot" (telnetd, debug CGI: confirm on the device in a network test), "Command-injection candidate in CGI <path>" (static candidate until confirmed dynamically with OK), "Binaries without NX/PIE/RELRO/canary" (low, supporting).
- Save with `save_finding` (draft status; the operator confirms). Fill `observation` with paths, offsets, commands and hashes, `poc` with reproducible steps, `references` with file names of the evidence.

## Pitfalls
- Absolute symlinks and `..` paths in extracted trees: resolve inside the rootfs, never follow them on the container filesystem; copy with `cp -P`.
- Extraction bombs and deep recursion: limit depth (`unblob -d`, binwalk without `-M`), watch disk space.
- Different output layouts: binwalk v2 `_<name>.extracted/`, binwalk v3 `<dir>/<name>.extracted/`, unblob `<name>_extract/` under `-e`.
- Missing helper extractors make binwalk "succeed" with empty directories: compare with `unblob --show-external-dependencies` and the signature list.
- Raw blobs analysed at base 0 or in ARM instead of Thumb mode give garbage; derive the base address from the vector table or the bootloader.
- Big-endian MIPS/PowerPC targets: set the architecture explicitly in r2/rizin/Ghidra when loading raw code.
- Ghidra on large binaries runs for a long time and uses a lot of RAM: background job, one binary at a time.
