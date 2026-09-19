# Deployment configuration

## Supported customization

The deployment passes these values to `nixorium.lib.mkLab`:

- `labConfig`: typed site settings loaded from the machine-owned
  `lab-settings.json`
- `publicKeys`: cache, SSH, and Veyon public-key paths
- `assets`: logo, backgrounds, MIME defaults, and VS Code settings
- `labSoftware`: package declarations loaded from `lab-software.json`
- `softwareCatalog`: local suggestions loaded from `software-catalog.nix`
- `sharedModules`: every installed host
- `controllerModules`: controller only
- `clientModules`: student PCs only
- `hostModules`: modules keyed by a generated host name
- `netbootModules`: the PXE environment

Unknown settings, asset names, public-key names, host names, and Veyon pilot
hosts are rejected. Keep every referenced file inside the deployment
repository.

The generated template owns the workstation profile in its package declaration,
catalog, focused `modules/`, `assets/`, and optional `scripts/`. Profile modules
receive `hostSoftwarePackages`, the effective managed package IDs for the host
being evaluated. Keep browser/editor favorites, shortcuts, services, and home
content conditional on the related ID so changing a package scope does not
leave stale policy.

`lab-settings.json` is deterministic, versioned JSON owned by the management
application. Validate it through both schema layers after any edit:

```sh
nix run .#nixorium -- config validate
```

For an administrator using the current TUI, prefer its installation flow:
laboratory settings are followed by validation/save, keys, controller activation,
client preparation, and a reviewed PXE start. This is not a save-only editor.
For the first-run CLI settings/key workflow:

```sh
nix run .#nixorium -- setup
```

It proposes detected network values, retains prior entries when navigating
back, accepts normal passwords without echo, hashes them locally, validates the
complete candidate through Nix, and presents a redacted review before writing.
Bare `setup` then reconciles the three key pairs; `setup configure` runs only
the settings stage.

For a machine-generated complete candidate, use the review/apply protocol:

```sh
nix run .#nixorium -- config plan --file candidate.json --json
nix run .#nixorium -- config apply --file candidate.json \
  --expect 'sha256:fingerprint-from-plan'
```

The plan evaluates the candidate through the deployment Flake and redacts
password hashes. Apply changes only `lab-settings.json`, uses an atomic write,
and rejects stale fingerprints or concurrent edits. Never place plaintext
passwords in a candidate file.

Do not make the application rewrite arbitrary Nix. Existing deployments that
still import `lab-config.nix` remain supported but read-only until an explicit
equivalence-checked migration is available.

## Network settings

`networkBase` is a full IPv4 network address, such as `10.0.0.0`, and
`networkPrefixLength` is its CIDR prefix. Client and controller host numbers
are offsets within that network. The controller number must be greater than
the client count, and every generated address must fit before the broadcast
address.

`masterDhcpIp` is the initial address/hint used only during PXE installation.
Preparation prefers it while assigned, otherwise captures the only usable
non-static, non-link-local IPv4 address on the configured interface. Update the
hint only if multiple candidate addresses make runtime selection ambiguous.

## Per-host customization

Create a deployment module and register it under the exact generated host name.
For example:

```nix
hostModules.pc05 = [ ./modules/pc05.nix ];
```

A name such as `pc5` is invalid when the generated host is `pc05`. Build
only that host before deploying it.

## Readiness and keys

Run:

```sh
nix eval .#deploymentStatus --json --no-write-lock-file
```

Do not install clients or deploy to them until `ready` is true. The status detects
the DHCP placeholder, missing public keys, and unchanged public password
hashes.

Controller-only mode instead uses `deploymentStatus.controller` when supported;
it permits local activation without clients or laboratory keys. Never turn this
exception into permission for client/PXE operations. Derive interface names
from evaluated metadata: per-host and controller/client overrides take priority
over the shared `ifaceName` fallback.

Private files stay outside Git:

- `secret-key`
- `admin-ssh`
- `veyon-private-key.pem`

Only their public counterparts belong under `keys/`.

Create or reconcile all three pairs with:

```sh
nix run .#nixorium -- setup keys
```

The operation creates only missing material, restricts private modes, verifies
public/private correspondence, and refuses to overwrite public-only or
mismatched pairs. Commit only the resulting files under `keys/`.

Install verified controller-side copies through the narrow privileged action:

```sh
nix run .#nixorium -- setup install-secrets
```

The source is the fixed `services.nixorium.deploymentPath` (default
`/home/admin/nixorium-deployment`). The systemd service rejects symlink
sources/destinations and refuses to replace different existing material.

Commit the reviewed settings and public keys, then apply this controller with:

```sh
nix run .#nixorium -- setup apply
```

The worktree must be clean. Use the confirmation requested by the CLI after
reviewing the networking/service warning. The fixed service builds the Git
view of the deployment as `admin`, which excludes the three ignored private
files from the Nix store, then activates only that exact closure as root.
Inspect failures with `journalctl -u nixorium-apply-controller.service` and
retry after correcting the reported preflight, build, or activation error.
Completion requires a root-owned receipt matching both the current Git
revision and active closure; `/run/current-system` alone is insufficient. After
upgrading from a version without receipts, run one reviewed `setup apply` to
create that proof even if the closure already matches.

For routine controller changes, use `nixorium controller plan` and its reviewed
apply command; see [operations](operations.md). Keep first-run compatibility
distinct from the ordinary rebuild workflow.
