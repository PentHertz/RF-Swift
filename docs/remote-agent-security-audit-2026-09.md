# Remote agent protocol: security audit (ground truth)

Date: 2026-09-08
Scope: the `rfswift agent` server and its protocol (`go/rfswift/remote`, `go/rfswift/cli/agent*.go`), the certificate and credential-file tooling (`certs init|client|export|import`), and the Workbench client side (`internal/workbench/remote.go`, `connection.go`, `engine.go` RemoteEngine). The MCP bridge (`internal/workbench/mcp.go`, `agent.go`) was reviewed for the companion best-practice guide.
Method: full source review, live probes against a real agent instance started with a bundle and an in-memory vault (a throwaway test harness, not kept in the tree), the project's own suites (`scripts/test-remote.sh all` with 20 s fuzz campaigns), `go vet`, and `govulncheck`.

This document records what was verified, what was found and fixed in this pass, and what remains by design. It supersedes the remote-agent items of [security-ground-truth-2026-08-31.md](security-ground-truth-2026-08-31.md). A review is a point in time, not a proof.

## Executive result

The transport and authentication boundary is sound: TLS 1.3 only, mutual TLS verified during the handshake with the bundle's private CA, expired, foreign or wrong-usage certificates rejected before any HTTP, unknown routes closed without a byte, request sizes and connection timeouts enforced, server pinning on the client. Nothing reachable without a CA-signed client certificate was found.

Two defects were found and fixed:

1. **Credential files were not integrity-protected** (medium). `certs client` and `certs export` files bound only the private key to the transfer passphrase. The CA, the certificate, the endpoint and the fingerprints were plaintext and unauthenticated, and the importer's checks (certificate signed by the embedded CA, key matching the certificate) all pass when an attacker re-signs the file's own public key with their CA and edits the fields. Confirmed against the code: a client file rewritten this way imported with the genuine passphrase and pointed the Workbench at the attacker's endpoint and pin; a server file rewritten this way imported and would have made the agent accept the attacker's clients. Fixed: files carry an HMAC-SHA256 under a scrypt-derived key over every field; import refuses modified files; untagged files from older releases import with an explicit warning.
2. **The version check was inert** (low). The agent never passed its version to the server, so `/v1/info` always said "development", and the Workbench set "up-to-date" unconditionally, so the "Agent version" line of the connection audit could not fail. Fixed: the agent reports `common.Version`; the Workbench compares and warns on mismatch or on an agent too old to report.

Also fixed: no access log existed for authenticated requests (every request is now logged with the client fingerprint prefix), and `certs init` did not tighten the mode of a pre-existing bundle directory.

The Workbench was reviewed as a target of a hostile agent: every agent-supplied string reaches the DOM escaped or through `textContent`, under a CSP that forbids remote connections and frames. Nothing exploitable was found; the only recommendation is to drop `'unsafe-inline'` from the CSP as defence in depth.

What remains is the trust model itself: a client certificate is full command execution as the agent's user, with no roles, no per-client revocation and no rate limiting. The deployment guide on rfswift.io ("Remote agent hardening") is written around that.

## Verified controls

Each line was checked in the source and, where marked live, exercised against a running agent.

| Control | Evidence |
|---|---|
| TLS 1.3 only (server `MinVersion`/`MaxVersion` 1.3; client the same) | live: a TLS 1.2 client gets `protocol version not supported`; `openssl s_client -tls1_2` gets alert 70 |
| Mutual TLS required: `RequireAndVerifyClientCert` against the bundle CA only | live: no certificate -> `certificate required` during the handshake, no HTTP bytes |
| Foreign CA, expired, and wrong extended key usage (a server certificate presented as a client) refused | live: all three refused during the handshake (`certificate required`, `expired certificate`, `bad certificate` with `incompatible key usage`) |
| HTTP/2 disabled (`TLSNextProto` empty) so the handler can hijack and close | live: ALPN `h2` offered, nothing negotiated |
| Unknown routes and wrong methods close the connection without a status line, header or body | live: `/health`, `GET /v1/control` return zero bytes to an authenticated client |
| `/v1/info` only after authentication; response carries no server banner beyond protocol, version, name, exposure and engine list | live: headers are `Content-Type`, `Date`, `Content-Length` |
| Request limits: 64 KiB command body, 128 arguments of at most 8 KiB with no NUL, 2 MiB control body, 16 KiB headers | live: 2 MiB+ body -> 400, 129 args -> 400, NUL -> 400, 20 KiB header -> 431 |
| Timeouts: 5 s to send the handshake or the request header, 15 s to read a request, 30 s idle | live: an authenticated connection sending nothing and a raw TCP connection sending no ClientHello are both closed after 5.0 s |
| Response limits on the client: 16 MiB command output and 16 MiB artifacts with JSON and base64 expansion accounted for | `protocol_limits_test.go`; regression tests at the limits pass |
| Server certificate pinning by SHA-256 on the client, plus CA chain and hostname verification, plus validity dates | live: a wrong pin fails with `agent certificate pin changed`; a right pin with a hostname that is not in the certificate fails with the x509 name error |
| Endpoints must be a bare `https://` origin (no credentials, path, query, fragment); `http://` refused, default port 8443 | `protocol_security_test.go` |
| Keys: ECDSA P-256; CA 10 years; leaves 1 year with `serverAuth` or `clientAuth` only; 128-bit random serials; neutral subjects | live: certificate dump |
| Keys at rest: PKCS#8 PBES2 (PBKDF2-HMAC-SHA256, AES-256-CBC) under a 256-bit random password held only in the OS vault; files `0600`; passphrase-protected keys use scrypt (N=2^17, r=8, p=1) and AES-256-GCM; minimum passphrase 12 characters | live: `openssl asn1parse` of both key kinds; file modes |
| Vault fails closed: Secret Service on Linux (D-Bus, login collection, unlock attempted), Keychain, Credential Manager; no file fallback | `zalando/go-keyring` source; `OSSecretStore` returns an error and the agent does not start |
| No plaintext or legacy key format accepted | `decryptPrivateKeyPEM` requires one `ENCRYPTED PRIVATE KEY` block |
| The agent prints "listening" only after the key is decrypted and the socket bound | `TestServeDoesNotReportListeningBeforeCredentialsLoad` |
| Artifact reads are confined to the mission workspace with symlink resolution on both root and target; listings skip symlinks and hidden directories and stop at 5,000 files; reads stop at 16 MiB | `safeAgentArtifactPath`, `agentArtifacts` |
| Terminal output buffered per session is capped at 4 MiB and marked truncated; UTF-8 sequences are not split | `agentTerminalRead` |
| Creation and pull jobs use 64-bit random ids; finished jobs expire after 10 minutes | `agent_builds.go` |
| Nested `agent` invocation through `/v1/command` refused | `runAgentCommand` |
| Client refuses cleartext and never falls back to local when the agent is selected | `normalizeEndpoint`, `RemotePendingEngine` |
| Dependencies | `govulncheck`: no vulnerable symbol reachable; `golang.org/x/crypto` v0.55.0 has three advisories in unreached code (GO-2026-6354 and GO-2026-6355 fixed in v0.56.0, GO-2026-5932 unfixed): bump at the next dependency round |

## Findings

| # | Severity | Finding | Status |
|---|---|---|---|
| 1 | Medium | Credential file metadata (CA, certificate, endpoint, fingerprints) not bound to the passphrase; substitution accepted at import with the genuine passphrase | Fixed: integrity tag (HMAC-SHA256, scrypt-derived key) sealed at issue, verified at import; `TestImportRefusesCredentialFilesModifiedInTransit`; legacy files import with a warning (`TestImportAcceptsLegacyCredentialFileWithWarning`) |
| 2 | Low | Agent version never reported; Workbench version check always passed | Fixed: `Version: common.Version` in the agent, `agentVersionState` comparison and audit text in the Workbench |
| 3 | Low | No access log of authenticated requests (the ground-truth document of 2026-08 already listed audit logging as P1) | Fixed: `logAccess` in every handler, client fingerprint prefix, source address, method or control method |
| 4 | Low | `certs init` into an existing directory kept that directory's mode | Fixed: `chmod 0700` after `MkdirAll` |
| 5 | Design | One CA-signed certificate equals every right of the agent's user: no roles, no per-command policy, no per-client allow or deny list, no revocation, no rate limit | Open. Documented in the hardening guide. Recommended next step: an optional `clients.json` next to `bundle.json` listing allowed client fingerprints, checked in `VerifyPeerCertificate`, so one credential can be removed without rotating the bundle |
| 6 | Design | Client keys are generated on the agent host and travel inside the credential file; a CSR flow where the key never leaves the client is not implemented | Open. Mitigated by the passphrase and the integrity tag; documented |
| 7 | Design | The CA key lives in the bundle directory of the machine where `certs init` ran; when that is the exposed agent host, a compromise of the host yields the CA | Open, operational: generate the bundle on an administration machine and move only the server side with `certs export` / `certs import` (the imported directory holds no CA key and cannot issue clients). Documented |
| 8 | Low | Leaf certificates expire after one year with no renewal command; the Workbench warns 30 days ahead for the server certificate only, an expiring client certificate fails with a bare TLS error | Open. Recommended: a `certs renew` command and a client-side expiry warning read from `client.pem` |
| 9 | Low | The server certificate carries a single name (`--host`); a client reaching the agent through a tunnel under another name fails hostname verification even with the right pin | Open. Documented workaround (dial the certificate's name, resolve it locally). Recommended: accept several `--host` values |
| 10 | Info | Terminal sessions have predictable ids (`remote-<unix nanoseconds>`), are shared between every authenticated client, are unlimited in number and never time out while the shell lives | Open, trusted-client population by design. Recommended: per-client ownership of sessions and an idle timeout |
| 11 | Info | No `WriteTimeout` on the server: a client that stops reading a large response holds a handler goroutine until it disconnects | Open, authenticated clients only |
| 12 | Info | `/v1/info` reports exposure from the bind address only (`loopback` or `lan`); an agent behind a VPN interface is shown as `lan` and audited as a warning | Open, conservative |
| 13 | Info | The `exposure`, `engines` and version fields of `/v1/info` are disclosed to authenticated clients; nothing is disclosed before authentication | Accepted |

## The Workbench as a target of a hostile agent

A compromised or impostor agent answers the Workbench's control calls with strings that end up in the WebView, which holds the Wails bindings to the local machine. What was checked:

| Control | Evidence |
|---|---|
| Agent-supplied strings are escaped before DOM insertion: mission ids and images (target cards), engine labels, states, sockets and details (engine doctor), USB device names, descriptions, serials and warnings, container summaries (mounts, devices, ports, seccomp, ulimits), audit records (ids, components, versions, evidence), tool search results, artifact paths and dates, Nix build stages | `esc()` on every interpolation in `renderEngineRows`/`engineBlockHTML`, `usbRowHTML`, `summaryRows`/`networkRows`, the audit issues list, the installer results and `renderLiveArtifacts`; the frontend sink-guard test forbids the raw mission id sink |
| Free text from the agent goes through `textContent`, never HTML: build log lines and the "now" line, pull status, toasts, confirmation and question dialogs, connection status, the version picker | `appendNixBuildLog`, `renderNixBuild`, `toast`, `workbenchDialog`, `dangerConfirm` |
| Terminal output is fed to xterm.js, and colour-query replies are filtered from the input path | `terminal-keys.js`, changelog |
| Content Security Policy: `default-src 'self'`, `connect-src 'self'`, no frames, objects or form actions, images and media only local, data or blob | `index.html` meta tag |
| Secrets never cross the JavaScript bridge: key passwords go from the Go side to the vault; the transfer passphrase is typed once and handed to Go | `remote.go`, `secrets.go` |
| A remote session never falls back to local operation, and the heartbeat marks it unreachable within 4 s | `RemotePendingEngine`, `PingRemoteAgent` |

Findings on this side:

| # | Severity | Finding | Status |
|---|---|---|---|
| 14 | Low | The CSP allows `script-src 'unsafe-inline'` (and inline styles). Escaping is applied consistently, but a future slip would not be caught by the policy; moving the inline bootstrap to a file and dropping `'unsafe-inline'` would make DOM injection from agent data inert | Open, defence in depth |
| 15 | Info | Saved agent profiles (endpoint, pinned fingerprint, credential directory, no keys) live in the WebView's local storage; a process that can edit that storage as the same user could change a pin, which is the same trust boundary as the user's files | Accepted |
| 16 | Info | The GUI's "Best practices" card and the Agent & MCP dialog text were written before the hardening and MCP guides | Fixed: both now match the guides |

## Trust model, stated plainly

- Possession of a client certificate signed by the agent's CA is remote command execution as the agent's user: `rfswift` with any arguments, containers with any mounts, capabilities and devices the engine allows, shells in those containers, the mission workspace, image pruning, Nix garbage collection.
- On Linux Docker that user is in the `docker` group and therefore root-equivalent; rootless Podman confines a privileged container to a user namespace.
- Everything above happens inside the mutually authenticated TLS session. The network between the Workbench and the agent sees a TLS 1.3 handshake and encrypted records; an unauthenticated peer learns only that a TLS server exists.
- The vault is the root of trust for keys at rest; a person who can unlock the agent user's vault can decrypt its keys.

## Hardening baseline (summary; the full guide is on rfswift.io)

1. Loopback bind and a WireGuard or SSH tunnel; never a public port; a VPN-interface bind with a firewall at most.
2. A dedicated, unprivileged agent account; rootless Podman preferred over Docker for exposed agents.
3. CA generated on an administration machine, only the server side installed on the agent host; one client file per person and machine; passphrase and file over separate channels; fingerprints compared at import.
4. Vault unlocked in the agent's session (Secret Service on Linux servers); no plaintext keys.
5. A hardened systemd user unit; stderr shipped and reviewed; alerts on handshake errors from unknown addresses.
6. Yearly rotation planned; lost credential means a new bundle and re-issued clients.

## Verification commands

```text
cd go/rfswift
go vet ./remote ./cli
go test ./remote ./cli -count=1
RFSWIFT_FUZZ_TIME=20s ../../scripts/test-remote.sh all
go run golang.org/x/vuln/cmd/govulncheck@latest ./remote/... ./cli/...
cd ../rfswift-workbench && go vet ./internal/workbench && go test ./internal/workbench ./tests/integration
```

Live probes (TLS versions, certificate classes, routes, limits, timeouts, pins, credential-file substitution) were run from a temporary Go test inside the `remote` package against `Serve` with an in-memory secret store, and removed afterwards; `TestImportRefusesCredentialFilesModifiedInTransit` keeps the substitution case as a permanent regression test.
