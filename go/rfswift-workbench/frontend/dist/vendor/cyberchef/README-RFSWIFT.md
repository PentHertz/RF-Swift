# Vendored CyberChef

RF Swift Workbench embeds the official CyberChef standalone distribution for
local, offline artifact decoding.

- Version: `v11.4.0`
- Upstream: <https://github.com/gchq/CyberChef>
- Release asset: `CyberChef_49d1a5634a67a3b806c6db0fdca7dcecb41a776c.zip`
- Release SHA-256: `423635c4fe05eba1be1e000aae3a82c711c5599c1bc1c0b625a02d344bd51bc4`
- Entry point SHA-256: `7a4b856f6104fdd7c0555c185fb06e82c05f0d1df14a6e1ec5c72c80bee4c0d3`
- License: Apache-2.0; see `LICENSE` and the generated `*.LICENSE.txt` files.

The bundle is served only by Workbench's embedded asset server. Artifact bytes
are transferred into it through the URL fragment, which is not sent to a
server. CyberChef itself includes explicitly network-capable recipes such as
HTTP Request and DNS over HTTPS; those can still contact the endpoint selected
by the operator. Updating the bundle must be explicit: verify the upstream
release and replace the version, hashes, entry-point reference, and license
together.
