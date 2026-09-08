# Changelog

All notable changes to Nixorium are documented in this file.

The project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- Replaced Ghostty with Foot as the lab terminal: same Ristretto color
  theme, JetBrainsMono Nerd Font, GNOME favorites and `Super+Return`
  keybinding, and identical screensaver behavior (fullscreen TTE effects
  with black palette overrides).
- Foot configuration and screensaver overrides moved to the foot 1.28
  `[colors-dark]` syntax (`cursor` foreground/background pair, validated
  against foot 1.28.0).
- GNOME favorites migration for admin and teacher: cached
  `com.mitchellh.ghostty.desktop` entries are replaced with `foot.desktop`
  at session start.
- Foot CSD titlebar styled to match GNOME Adwaita dark (GNOME/mutter has
  no server-side decorations), aligned with Nautilus and other GTK4 apps.
- Documented the complete per-host customization workflow in both READMEs and
  in the Nixorium maintainer skill.

### Added

- An interactive controller-bootstrap version selector offering `master`, the
  latest GitHub prerelease, and published stable releases while preserving
  `--release` and `NIXORIUM_RELEASE` for unattended installations.

## [2.0.0-beta.3] - 2026-09-08

### Added

- A Raw GitHub `install.sh` entrypoint that selects a tagged release, prepares
  the private deployment Git repository, and installs the controller from it.

### Changed

- Rewrote the deployment upgrade guide with an explicit release example,
  current Nix Flake commands, complete validation, and the Git merge workflow.
- Completed the project-wide Nixorium rebrand across repositories, Flake
  inputs, installer identifiers, command names, desktop identifiers, and the
  Agent Skill.
- Adopted `nixorium.org` as the canonical website and installer entrypoint,
  with Raw GitHub documented as the bootstrap fallback.

### Breaking

- Private deployments must rename their upstream input to `nixorium` and use
  `github:giovantenne/nixorium/<release>` when moving to this release.
- Installer environment variables now use the `NIXORIUM_` prefix.

## [2.0.0-beta.2] - 2026-09-07

### Added

- A reusable `lib.mkLab` Flake API with typed lab configuration validation.
- A `site` Flake template for private per-lab deployment repositories.
- Extension points for shared, controller, client, host-specific and netboot modules.
- Configurable logo, backgrounds, MIME defaults, VS Code settings and public key paths.
- A standalone netboot installer bundle containing the effective downstream configuration.
- A portable repository-maintainer Agent Skill, preinstalled by the site template for Codex, OpenCode, Claude Code and Pi.

### Changed

- Operational Harmonia and PXE helpers are exposed as Flake apps for downstream repositories.
- Lab-specific configuration can now update the upstream through a pinned Flake input instead of Git merges.
- The upstream screensaver logo is generic and site-specific printer drivers are delegated to deployment modules.

## [2.0.0-beta.1] - 2026-09-04

### Added

- Per-user rootless Docker daemons with declarative subordinate UID/GID ranges.
- A writable `~/.local/npm` global prefix, available in every login session for tools such as Codex and Claude Code.
- An opt-in `veyonNativeHosts` canary for Veyon 4.11's PipeWire/XDG portal Wayland backend.
- Setuid wrappers for Veyon's authentication and Wayland input helpers.

### Changed

- Updated nixpkgs from NixOS 25.11 to 26.05 and refreshed all flake inputs.
- Updated Veyon from the local 4.10.0 derivation to its official 4.11.0 flake.
- Updated the GNOME Remote Desktop reconnect patch for GNOME 50's connection throttler.
- Switched development packages to the current stable Node.js and PHP aliases.
- Migrated the controller sleep policy to NixOS 26.05's structured systemd settings.
- Disabled automatic ZFS root-pool imports because the shared Disko layout uses Btrfs.
- Excluded Docker data, the npm cache, and global npm tools from student home snapshots.

### Security

- Removed every normal user from the root-equivalent `docker` group.

### Breaking

- Docker now uses a per-user rootless socket and storage. Existing rootful images and containers are not migrated.

## [1.0.0] - 2026-09-04

### Added

- Declarative generation of controller and student workstations with Nix Flakes.
- UEFI installation and Btrfs partitioning through Disko.
- Offline client installation through ProxyDHCP, TFTP, HTTP netboot, and a local Harmonia cache.
- Multi-host deployment through Colmena.
- Parameterized lab settings and a public `labMeta` flake output for operational scripts.
- GNOME Wayland student desktop with development tools and classroom defaults.
- Boot-time student home reset with five recoverable Btrfs snapshots.
- Veyon classroom management with pre-generated lab topology and GNOME Remote Desktop integration.

### Security

- Key-only SSH access and immutable declarative users.
- Separate public and private material for SSH, Harmonia, and Veyon.

[Unreleased]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.3...HEAD
[2.0.0-beta.3]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.2...v2.0.0-beta.3
[2.0.0-beta.2]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.1...v2.0.0-beta.2
[2.0.0-beta.1]: https://github.com/giovantenne/nixorium/compare/v1.0.0...v2.0.0-beta.1
[1.0.0]: https://github.com/giovantenne/nixorium/releases/tag/v1.0.0
