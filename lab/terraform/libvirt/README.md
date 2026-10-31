# Libvirt SNO validation harness

This OpenTofu root module creates one disposable SNO guest, disk, and isolated
NAT network. It consumes the normal `platform: none` configuration and a
verified Okub artifact manifest through `okub lab`; it is not an installation
platform. Every resource name carries the explicit run UUID-derived owner tag.

PXE runs use the production `okub artifacts serve --mode pxe` command on the
lab bridge. This module intentionally contains no HTTP, TFTP, or ProxyDHCP
server. Start that server before applying a PXE run and keep it running until
the Agent environment has fetched its published artifacts.

Do not invoke this module with an implicit backend. The CLI stores each run's
state and private diagnostics under `.local/lab/<run-id>` and inspects exact
state/live identities and domain attachments before allowing destroy. Raw serial
output is deliberately not persisted because it cannot be reliably scrubbed.
