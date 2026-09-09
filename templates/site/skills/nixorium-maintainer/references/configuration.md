# Deployment configuration

## Supported customization

The deployment passes these values to `nixorium.lib.mkLab`:

- `labConfig`: typed site settings
- `publicKeys`: cache, SSH, and Veyon public-key paths
- `assets`: logo, backgrounds, MIME defaults, and VS Code settings
- `sharedModules`: every installed host
- `controllerModules`: controller only
- `clientModules`: student PCs only
- `hostModules`: modules keyed by a generated host name
- `netbootModules`: the PXE environment

Unknown settings, asset names, public-key names, host names, and Veyon pilot
hosts are rejected. Keep every referenced file inside the deployment
repository.

## Network settings

`networkBase` is a full IPv4 network address, such as `10.0.0.0`, and
`networkPrefixLength` is its CIDR prefix. Client and controller host numbers
are offsets within that network. The controller number must be greater than
the client count, and every generated address must fit before the broadcast
address.

`masterDhcpIp` is used only during PXE installation. Update it and rebuild
netboot artifacts whenever the institutional DHCP lease changes.

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

Do not install clients or deploy until `ready` is true. The status detects
the DHCP placeholder, missing public keys, and unchanged public password
hashes.

Private files stay outside Git:

- `secret-key`
- `admin-ssh`
- `veyon-private-key.pem`

Only their public counterparts belong under `keys/`.
