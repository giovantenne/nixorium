# iPXE bootstrap binaries

This directory is intentionally not populated by default.

Place these files here before running `./scripts/run-pxe-proxy.sh`:

- `snponly.efi`

It is obtained from nixpkgs iPXE as `snp.efi` and stored here as
`snponly.efi`. It is used by `dnsmasq`
ProxyDHCP to bootstrap UEFI clients before chainloading the NixOS netboot
payload over HTTP.
