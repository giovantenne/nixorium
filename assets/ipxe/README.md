# iPXE bootstrap binaries

This directory is intentionally not populated by default.

Place these files here before running `./scripts/run-pxe-proxy.sh`:

- `snponly.efi`

They can be obtained from an iPXE package/build and are used by `dnsmasq`
ProxyDHCP to bootstrap UEFI clients before chainloading the NixOS netboot
payload over HTTP.
