# macOS signing, notarization and DMG releases

RF Swift ships a macOS disk image, `rfswift_Darwin_universal.dmg`, built by
`.github/workflows/macos-dmg.yml` on every `v*` tag. The image contains:

- `rfswift-workbench.app`: the Workbench GUI (`go/rfswift-workbench`, a Wails
  app) as a universal (Apple Silicon + Intel) bundle, next to an
  `Applications` link for the classic drag-to-install;
- `rfswift`: the universal CLI/TUI binary, with a double-clickable
  `Install RF Swift CLI.command` that copies it to `/usr/local/bin`;
- the license and a short README.

When the Apple secrets described below are configured on the repository, the
CLI binary, the `.app` and the image are code-signed with the PentHertz
"Developer ID Application" certificate, notarized by Apple, and stapled, so
Gatekeeper opens them with zero warnings. Without the secrets the workflow
still produces an unsigned image (useful for forks and dry runs), it just
skips the signing steps.

The secret names are identical to the ones used by PentHertz/LUKSbox, so if
you already went through this once for LUKSbox you can reuse the exact same
values (or promote them to organization-level secrets and grant RF-Swift
access to them). This document rebuilds the whole process from scratch
anyway, so you never have to remember it again.

Signing needs two independent credentials:

1. a Developer ID Application certificate (+ its private key), packed as a
   password-protected `.p12` file. This is what `codesign` uses.
2. notarization credentials: your Apple ID, an app-specific password, and
   your 10-character Team ID. This is what `notarytool` uses.


## 1. One-time setup on the Apple side

You need an active Apple Developer Program membership (the paid one). All
commands below run on any machine with OpenSSL 3.x; none of them require a
Mac until you want to test the result locally.

### 1.1 Create the Developer ID Application certificate with OpenSSL

Generate a private key and a certificate signing request. Keep `devid.key`
forever and keep it private: it IS your signing identity, the certificate
downloaded from Apple is useless without it.

```sh
openssl genrsa -out devid.key 2048
openssl req -new -key devid.key -out devid.csr \
    -subj "/emailAddress=YOUR_APPLE_ID_EMAIL/CN=PentHertz/C=FR"
```

Upload the CSR to Apple:

1. Go to https://developer.apple.com/account -> Certificates, IDs and
   Profiles -> Certificates -> "+".
2. Pick "Developer ID Application" (NOT "Developer ID Installer": that one
   only signs `.pkg` installers, which we do not ship). Keep the default
   G2 Sub-CA profile.
3. Upload `devid.csr`, then download the result,
   `developerID_application.cer`.

Note: Apple limits Developer ID Application certificates to 5 active ones
per team, and only the Account Holder role can create them.

Assemble the `.p12`. The Apple intermediate CA must be included, otherwise
`codesign` later fails with "unable to build chain to self-signed root":

```sh
# Convert the Apple-issued cert from DER to PEM
openssl x509 -inform DER -in developerID_application.cer -out devid-cert.pem

# Fetch and convert the Developer ID G2 intermediate CA
curl -LO https://www.apple.com/certificateauthority/DeveloperIDG2CA.cer
openssl x509 -inform DER -in DeveloperIDG2CA.cer -out DeveloperIDG2CA.pem

# Pack key + cert + chain into a password-protected PKCS12.
# -legacy is CRITICAL: OpenSSL 3 defaults to PBES2/AES encryption, which
# macOS 'security import' rejects with a misleading "MAC verification
# failed" (identical to the wrong-password error). -legacy forces the
# PBES1/3DES envelope that macOS accepts.
openssl pkcs12 -export -legacy \
    -out devid.p12 \
    -inkey devid.key \
    -in devid-cert.pem \
    -certfile DeveloperIDG2CA.pem \
    -name "Developer ID Application"
# You will be prompted for an export password: this becomes the
# APPLE_DEVELOPER_ID_CERT_PASSWORD secret.

# Verify the bundle + password before uploading anything to GitHub:
openssl pkcs12 -in devid.p12 -nokeys -noout
# (prompts for the password; silence + exit 0 means OK)
```

Alternative if you prefer the Mac GUI: create the CSR from Keychain Access
(Certificate Assistant -> Request a Certificate From a Certificate
Authority), upload it the same way, double-click the downloaded `.cer` so
it pairs with the key in your keychain, then export "Certificates" as
`.p12` from Keychain Access. If that `.p12` is rejected by CI with "MAC
verification failed", re-export it through OpenSSL with `-legacy` as shown
above (recent Keychain Access exports are fine, but the OpenSSL route is
deterministic).

### 1.2 Notarization credentials

- Team ID: https://developer.apple.com/account -> Membership details. A
  10-character string like `ABCDE12345`.
- App-specific password: https://account.apple.com -> Sign-In and Security
  -> App-Specific Passwords -> "+". Name it e.g. `rfswift-notary`. You get
  a value like `abcd-efgh-ijkl-mnop`; it is shown once, store it in your
  password manager. This is NOT your Apple ID password; notarytool refuses
  regular passwords.

An App Store Connect API key also works with notarytool (`--key/--key-id/
--issuer`) but the app-specific password is what both LUKSbox and this
workflow use, so stick with it.


## 2. GitHub secrets

Repository -> Settings -> Secrets and variables -> Actions (or Organization
-> Settings -> Secrets and variables -> Actions, then grant the RF-Swift
repository access). Create:

| Secret                             | Value                                                    |
|------------------------------------|----------------------------------------------------------|
| `APPLE_DEVELOPER_ID_CERT_P12_B64`  | single-line base64 of `devid.p12` (see below)            |
| `APPLE_DEVELOPER_ID_CERT_PASSWORD` | the `.p12` export password                               |
| `MACOS_CI_KEYCHAIN_PASSWORD`       | any random string; protects the throwaway CI keychain    |
| `APPLE_NOTARY_USER`                | the Apple ID email of the developer account              |
| `APPLE_NOTARY_PASSWORD`            | the app-specific password (`abcd-efgh-ijkl-mnop`)        |
| `APPLE_NOTARY_TEAM_ID`             | the 10-character Team ID                                 |

Encode the `.p12` as SINGLE-LINE base64. `base64` on macOS wraps at 76
columns by default and a wrapped value pasted into a GitHub secret is the
number one cause of "MAC verification failed" in CI:

```sh
openssl base64 -A -in devid.p12 | pbcopy     # macOS
openssl base64 -A -in devid.p12 | xclip -sel clip   # Linux
```

Generate the keychain password with e.g. `openssl rand -base64 24`. It
protects nothing durable (the keychain lives only for the CI job), it just
has to exist and be non-trivial.

When pasting secrets, watch for trailing newlines and surrounding quotes:
the CI's OpenSSL pre-verify step fails early and explicitly when the
password does not match, so a hidden trailing `\n` shows up as a clear
error rather than a cryptic keychain failure.


## 3. What the CI does

`.github/workflows/macos-dmg.yml` runs on every `v*` tag (and manually via
workflow_dispatch for a dry run). Step by step:

1. Module audit gate: same `modules-audit.yml` as CI; a signed DMG does
   not ship with a vulnerable dependency tree either.
2. Build the CLI: `go build` for `darwin/amd64` and `darwin/arm64` (CGO
   disabled, `netgo` tag, `-trimpath -ldflags "-s -w"`), merged into one
   universal binary with `lipo`.
3. Build the Workbench: `go install` the pinned Wails CLI, then
   `make darwin_universal` in `go/rfswift-workbench` (the same target
   `release.yml` uses), which yields `build/bin/rfswift-workbench.app`.
   The bundle's `Info.plist` comes from
   `go/rfswift-workbench/build/darwin/Info.plist` (bundle identifier
   `com.penthertz.rfswift-workbench`); the release version is stamped
   into `CFBundleShortVersionString` / `CFBundleVersion` with PlistBuddy
   right after the build, since Wails has no flag for it and `common.go`
   stays the single version source.
4. Import certificate (only if `APPLE_DEVELOPER_ID_CERT_P12_B64` is set):
   decodes the `.p12`, sanity-checks the bytes look like PKCS12,
   pre-verifies password + envelope with OpenSSL (turns the two ambiguous
   `security` failures into precise error messages), imports it into a
   fresh throwaway keychain, and unlocks the key for non-interactive
   `codesign` use via `set-key-partition-list`. The identity string is
   discovered from the keychain at run time, so renewing the certificate
   never requires touching CI.
5. Sign the CLI: `codesign --force --timestamp --options runtime`.
   Hardened runtime + secure timestamp are both hard requirements for
   notarization.
6. Sign the app, inside-out: every nested Mach-O first (a Wails app
   normally has none; the loop guards against a future helper or dylib),
   then the bundle itself, which signs `Contents/MacOS/rfswift-workbench`
   and seals `Contents/Resources`. Never `codesign --deep`. No
   entitlements file: the Workbench is not sandboxed, and WKWebView, the
   Keychain, PTYs and the docker/podman/nix subprocesses all work under
   the hardened runtime without any. Verified with
   `codesign --verify --deep --strict`.
7. Notarize + staple the app (only if the notary secrets are also set):
   the bundle is zipped with `ditto`, submitted with
   `xcrun notarytool submit --wait`, then `xcrun stapler staple` writes
   the ticket into the bundle. This happens BEFORE the DMG is built, so
   the app still validates offline once dragged out of the image into
   `/Applications`. `spctl -a -t exec` confirms the Gatekeeper verdict.
8. Stage + build the DMG: app (copied with `ditto` so the seal survives),
   `Applications` link, CLI, `Install RF Swift CLI.command`, README,
   license -> `hdiutil create -format UDZO`.
9. Sign the DMG: `codesign --force --timestamp` (no hardened runtime on a
   container file). Required, or notarytool answers "The signature does
   not include a secure timestamp".
10. Notarize + staple the DMG: `xcrun notarytool submit --wait`, then
    `xcrun stapler staple` so Gatekeeper validates offline, then
    `spctl -a -t open` as the final this-is-what-the-user-sees check.
    Expected verdict: `source=Notarized Developer ID`.
11. Provenance + upload: a Sigstore build attestation is generated for the
    DMG, and on tag builds the DMG is uploaded to the GitHub release
    created by `release.yml` (the workflow waits for goreleaser to create
    it first).

Forks and PRs never see the secrets, so their runs produce unsigned
images: that is the intended trade-off, signing material never reaches
untrusted contexts.

Two notes on the other macOS artifacts:

- `release.yml` still uploads `rfswift-workbench_Darwin_universal.zip`
  (unsigned, built by its own `workbench-macos` job) plus the goreleaser
  `rfswift_Darwin_*.tar.gz` archives. They stay as they are: the DMG is
  the signed distribution channel for macOS, and the workbench checksums
  file `release.yml` generates would no longer match if the DMG workflow
  replaced that zip.
- The DMG is uploaded after goreleaser has generated `checksums.txt`, so
  it is not listed there. Its sha256 is printed in the CI log, and the
  artifact is covered by the Sigstore attestation:

  ```sh
  gh attestation verify rfswift_Darwin_universal.dmg --repo PentHertz/RF-Swift
  ```


## 4. Signing and notarizing by hand

For a local release or for debugging CI, on a Mac with the `.p12`:

```sh
# One-time: import the identity into your login keychain
security import devid.p12 -P 'THE_P12_PASSWORD' -T /usr/bin/codesign

# One-time: store the notary credentials under a profile name
xcrun notarytool store-credentials rfswift-notary \
    --apple-id YOUR_APPLE_ID_EMAIL \
    --team-id ABCDE12345 \
    --password abcd-efgh-ijkl-mnop

# Build both deliverables
( cd go/rfswift && make darwin )                 # bin/rfswift_darwin_{amd64,arm64}
lipo -create -output rfswift go/rfswift/bin/rfswift_darwin_amd64 \
    go/rfswift/bin/rfswift_darwin_arm64
( cd go/rfswift-workbench && make darwin_universal )
APP=go/rfswift-workbench/build/bin/rfswift-workbench.app

# Sign the CLI (hardened runtime + timestamp)
codesign --force --timestamp --options runtime \
    --sign "Developer ID Application" rfswift
codesign --verify --strict --verbose=2 rfswift

# Sign the app (a Wails bundle has a single Mach-O, so signing the
# bundle is enough; sign nested helpers/dylibs first if you add any)
codesign --force --timestamp --options runtime \
    --sign "Developer ID Application" "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"

# Notarize + staple the app
ditto -c -k --sequesterRsrc --keepParent "$APP" workbench.zip
xcrun notarytool submit workbench.zip --keychain-profile rfswift-notary --wait
xcrun stapler staple "$APP"
spctl -a -t exec -vv "$APP"

# Build + sign the DMG
mkdir dmg-root && cp rfswift dmg-root/ && ditto "$APP" dmg-root/rfswift-workbench.app \
    && ln -s /Applications dmg-root/Applications
hdiutil create -volname "RF Swift" -srcfolder dmg-root -ov -format UDZO \
    rfswift_Darwin_universal.dmg
codesign --force --timestamp --sign "Developer ID Application" \
    rfswift_Darwin_universal.dmg

# Notarize + staple the DMG
xcrun notarytool submit rfswift_Darwin_universal.dmg \
    --keychain-profile rfswift-notary --wait
xcrun stapler staple rfswift_Darwin_universal.dmg
xcrun stapler validate rfswift_Darwin_universal.dmg
spctl -a -t open --context context:primary-signature -v \
    rfswift_Darwin_universal.dmg
```


## 5. Workbench bundle details

- Bundle template: `go/rfswift-workbench/build/darwin/Info.plist`. Wails
  renders it with the values from `wails.json` (`{{.Info.ProductName}}`,
  `{{.OutputFilename}}`, ...). Change the identifier there if the app ever
  moves to another team; nothing in the workflow hardcodes it.
- Version: `wails.json` carries no `productVersion`, so a local
  `make darwin_*` yields Wails' default `1.0.0`; CI stamps the tag version
  after the build. Add `"productVersion"` under `info` in `wails.json` if
  you want local builds versioned too.
- Entitlements: none today. Add a `--entitlements` plist to the two app
  `codesign` calls only for a capability the GUI demonstrably needs, and
  keep it minimal: `com.apple.security.cs.allow-jit` is NOT needed for
  WKWebView (JIT runs in Apple's own WebContent process),
  `com.apple.security.cs.disable-library-validation` only if the app ever
  loads plugins signed by another team.
- The app inside the DMG is the same bundle `release.yml` zips, plus the
  signature and the stapled ticket. Both are built from the same commit
  with the same Wails version, pinned in the Makefile, `release.yml` and
  `macos-dmg.yml` (`WAILS_VERSION`); bump all three together.


## 6. Troubleshooting

- "MAC verification failed" from `security import`: three causes, in
  order of likelihood. (a) The base64 secret was line-wrapped or
  truncated: re-encode with `openssl base64 -A`. (b) The `.p12` was
  exported by OpenSSL 3 without `-legacy` (PBES2/AES envelope): OpenSSL
  accepts it, `security` does not; re-export with `-legacy`. (c) The
  password secret does not match the `.p12` (hidden newline, or the file
  was re-exported with a new password). The CI's OpenSSL pre-verify step
  distinguishes (b) from (c) for you.
- "The signature does not include a secure timestamp" at notarization:
  a `codesign` call ran without `--timestamp`, or the DMG itself was
  never signed.
- Notarization returns "Invalid": get the real reason with
  `xcrun notarytool log <submission-id> --keychain-profile rfswift-notary
  log.json`. Typical findings: a binary without hardened runtime, an
  unsigned Mach-O buried in the payload (for the app, a helper or dylib
  that the inside-out loop should have caught: check the `file -b`
  output), or a bundle without `CFBundleIdentifier`.
- `codesign --verify --deep --strict` fails on the app with "resource
  fork, Finder information, or similar detritus not allowed": an xattr
  landed on a file inside the bundle; `xattr -cr "$APP"` before signing.
- `codesign` fails with `errSecInternalComponent` in CI: the private key
  is not unlocked for codesign; the `set-key-partition-list` call is
  missing or ran with the wrong keychain password.
- "unable to build chain to self-signed root": the Apple intermediate CA
  was not packed into the `.p12`; rebuild it with
  `-certfile DeveloperIDG2CA.pem`.
- `notarytool` authentication errors: `APPLE_NOTARY_PASSWORD` must be an
  app-specific password, not the account password, and the Apple ID must
  belong to the team in `APPLE_NOTARY_TEAM_ID`.
- The Workbench opens but Keychain prompts every launch: the app was
  re-signed with another identity or bundle identifier; the vault items
  are bound to the previous one. Keep `com.penthertz.rfswift-workbench`
  and the same team across releases.
- Certificate renewal: Developer ID Application certificates are valid
  for 5 years. To renew, repeat section 1.1 (a new CSR from the SAME
  `devid.key` is fine), rebuild the `.p12`, replace
  `APPLE_DEVELOPER_ID_CERT_P12_B64` (+ password if changed). Nothing in
  the workflow references the certificate by name or expiry, so no YAML
  changes are needed. Already-notarized releases keep working after
  expiry: the notarization ticket, not the certificate window, is what
  Gatekeeper checks.
