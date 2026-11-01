# Vendored CyberChef

RF Swift Workbench embeds the official CyberChef standalone distribution for
local, offline artifact decoding.

- Version: `v11.5.0`
- Upstream: <https://github.com/gchq/CyberChef>
- Release asset: `CyberChef_8cd426dd4f40f1423912d5fad91b578a86a65112.zip`
- Release SHA-256: `f6478925d3eaa16ec08626a85f5b85eed10d531a5e18700615fed7ecb024cccf`
- Entry point SHA-256: `809174936fd5f895e23f45e6d06448b6dc6f8f115cf7acc969029f3d894903e9`
- License: Apache-2.0; see `LICENSE` and the generated `*.LICENSE.txt` files.

The bundle is served only by Workbench's embedded asset server. Artifact bytes
are transferred into it through the URL fragment, which is not sent to a
server. CyberChef itself includes explicitly network-capable recipes such as
HTTP Request and DNS over HTTPS; those can still contact the endpoint selected
by the operator. Updating the bundle must be explicit: verify the upstream
release and replace the version, hashes, entry-point reference, and license
together.
