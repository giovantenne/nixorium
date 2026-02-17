# AGENTS.md

This repository manages a 31-PC NixOS school computer lab using Nix Flakes,
Disko, and Colmena. The master controller (`pc99`, IP `10.22.9.99`) deploys to
30 student workstations over a LAN-only network (no internet on clients).

## Project Structure

```
flake.nix                  # Entry point: generates pc01-pc30 + pc99 configs + netboot + Colmena
flake.lock                 # Pinned inputs (nixpkgs nixos-25.11, disko)
disko-uefi.nix             # Declarative disk partitioning (UEFI boot)
setup.sh                   # Installer script for PXE-booted client PCs
public-key                 # Binary cache public key (for reference)
veyon-public-key.pem       # Veyon RSA public key (deployed to all PCs)
pkgs/
  veyon.nix                # Veyon package derivation (not in nixpkgs)
modules/
  common.nix               # Shared system config (GNOME, packages, shells, services)
  hardware.nix             # Generic hardware detection (replaces per-host hardware-configuration.nix)
  networking.nix           # Hostname + static IP with shared iface name
  users.nix                # User accounts (admin + docente + informatica student, veyon-master group)
  cache.nix                # Binary cache client (points to pc99's Harmonia)
  filesystems.nix          # Btrfs subvolume mount declarations
  home-reset.nix           # Student home directory templating + boot-time reset
  veyon.nix                # Veyon service, public key, firewall, base config
scripts/
  install-pc99.sh          # Live USB bootstrap installer for pc99 with disk selection
  run-harmonia.sh          # Launches Harmonia binary cache server
  run-pxe-proxy.sh         # ProxyDHCP + TFTP + HTTP netboot server (external DHCP compatible)
  create-home-template.sh  # Builds clean home directory template
  home-reset.sh            # Boot-time snapshot rotation + home reset
  gnome-user-setup.sh      # GNOME favorites and welcome setup
assets/
  backgrounds/             # Ristretto wallpapers (random at each home-reset)
    1-ristretto.jpg
    2-ristretto.jpg
    3-ristretto.jpg
  ipxe/
    README.md              # Notes about required iPXE bootstrap binaries
  mimeapps.list            # Default browser = Chromium
```

## Build / Deploy Commands

```sh
# Evaluate a single host config (syntax/type check without building)
nix eval .#nixosConfigurations.pc01.config.system.build.toplevel --no-write-lock-file

# Build a single host (full build, outputs to ./result)
nix build .#nixosConfigurations.pc01.config.system.build.toplevel

# Build all client closures
nix build .#nixosConfigurations.pc{01..30}.config.system.build.toplevel

# Rebuild and activate on the local machine (pc99)
sudo nixos-rebuild switch --flake .#pc99 --no-write-lock-file

# Deploy to all lab PCs via Colmena
colmena apply --on @lab

# Deploy to a single PC
colmena apply --on pc05

# Build netboot artifacts
nix build .#nixosConfigurations.netboot.config.system.build.kernel --out-link result-kernel
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --out-link result-initrd
nix build .#nixosConfigurations.netboot.config.system.build.netbootIpxeScript --out-link result-ipxe
```

There are **no tests, linters, or formatters** configured in this repository.
To validate changes, build the affected host configuration (`nix build`).

## Architecture Notes

- Hosts pc01-pc30 are generated programmatically via `builtins.genList` + `mkHost`/`mkColmenaHost` in `flake.nix`, with pc99 defined separately as the controller.
- Hostname + static IP are centralized in `flake.nix` (derived from `networkBase` + host number) and applied in `modules/networking.nix`. Each PC gets both a DHCP address and a static address on the same interface.
- Custom settings flow from `flake.nix` via `specialArgs` (`labSettings`, `hostName`, `hostIp`) to modules that need them.
- No custom NixOS options are declared (`options = { ... }`). This repo only sets existing nixpkgs options.
- VirtualBox guest additions are enabled by default via `mkDefault` in `common.nix` (harmless on bare metal).
- Hardware detection uses `modules/hardware.nix` with `not-detected.nix` for automatic driver loading. No per-host hardware-configuration.nix files are needed.
- UEFI boot is required on all machines. Disk partitioning uses an EFI System Partition (`/boot`) plus Btrfs subvolumes.
- Netboot uses `dnsmasq` in ProxyDHCP mode (`scripts/run-pxe-proxy.sh`) so institutional DHCP remains authoritative for leases.
- `labOverlay` in `flake.nix` provides custom packages (e.g. `pkgs.veyon` via `callPackage ./pkgs/veyon.nix {}`). Applied via `nixpkgs.overlays` in each host's module list and in `colmena.meta.nixpkgs`.
- Veyon classroom management is configured in `modules/veyon.nix`: runs `veyon-service` on all PCs, deploys the public key, generates a `Veyon.conf` with all 30 client PCs pre-mapped, and opens port 11100. The private key is not managed by Nix (see Security).
- The `veyon-master` group (declared in `modules/veyon.nix`) controls access to the Veyon private key. Users `admin` and `docente` are members (configured in `modules/users.nix`).

## Nix Code Style

### Formatting
- **2-space indentation**, no tabs
- No block comments (`/* ... */`); use single-line `#` comments with a space after `#`
- Place comments on the line above the code they describe
- No trailing commas in lists or attribute sets (Nix does not use them)

### Module Signatures
Use only the arguments the module actually needs:
```nix
{ ... }:            # When no module args are used
{ config, pkgs, lib, ... }:   # When config/pkgs/lib are needed
{ labSettings, ... }:          # When consuming specialArgs
```

### Attribute Sets
- Dot-path notation for one-liners: `boot.loader.systemd-boot.enable = true;`
- Nested set notation for grouped settings:
  ```nix
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    pulse.enable = true;
  };
  ```
- Mixing both styles within a file is acceptable and expected.

### Lists
- Short lists on one line: `[ "nix-command" "flakes" ]`
- Long lists with one item per line:
  ```nix
  environment.systemPackages = with pkgs; [
    wget
    curl
    bat
  ];
  ```

### `with` Usage
- Use `with pkgs;` **only** for `environment.systemPackages` (the long package list)
- Everywhere else, use explicit `pkgs.packageName` references
- Always use store-qualified paths for executables: `"${pkgs.bash}/bin/bash"`

### `inherit` Usage
- One binding per `inherit` statement, each on its own line:
  ```nix
  inherit name;
  inherit system;
  ```

### `let...in` Blocks
- Place between the function signature and the attribute set body
- Only use when there are repeated values or complex expressions to extract

### String Handling
- Multi-line strings: `''...''` (Nix indented strings)
- Interpolation: `${...}` inside strings
- Explicit `toString` for int-to-string conversion: `toString n`

### Naming Conventions
| Scope            | Convention       | Examples                                    |
|------------------|------------------|---------------------------------------------|
| Nix variables    | camelCase        | `masterIp`, `mkHost`, `labSettings`         |
| File names       | kebab-case       | `home-reset.nix`, `run-harmonia.sh`         |
| Shell variables  | UPPER_SNAKE_CASE | `PC_NUMBER`, `TEMPLATE_DIR`                 |
| NixOS options    | Standard dotted  | `services.openssh.enable`                   |
| Helper functions | `mk` prefix      | `mkHost`, `mkColmenaHost`                   |

### Imports
- Flake uses root-relative: `./modules/networking.nix`, `./modules/cache.nix`
- Script references from modules: `../scripts/create-home-template.sh`

## Shell Script Style

All shell scripts must follow these conventions:

```bash
#!/usr/bin/env bash
set -euo pipefail
```

- Shebang: always `#!/usr/bin/env bash` (not `/bin/bash`)
- Safety: always `set -euo pipefail` as the first non-comment line
- Variables: `UPPER_SNAKE_CASE`, always double-quoted (`"$VAR"`, `"${VAR}"`)
- Positional args: assign to named variables immediately (`TEMPLATE_DIR="$1"`)
- Errors: print to stderr with `>&2` (`echo "Error: ..." >&2`)
- Cleanup: use `trap '...' EXIT` for temporary file cleanup
- Validation: check argument count and format before proceeding

## Security

- **Never commit** `secret-key` or `id_ed25519` (both in `.gitignore`)
- **Never commit** `veyon-private-key.pem` (in `.gitignore`); deploy manually to `/etc/veyon/keys/private/teacher/key` with mode `0640` and group `veyon-master`
- Passwords in `users.nix` are hashed (SHA-512 crypt); never store plaintext
- SSH password auth is disabled; key-based only
- `users.mutableUsers = false` enforces declarative user management

## Host Configuration

Hostname + static IP are generated in `flake.nix`. The shared interface name
is configured via `labSettings.ifaceName` and applied in `modules/networking.nix`.
