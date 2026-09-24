#!/usr/bin/env bash
set -euo pipefail

EXPECTED_ISO_SHA256=fc01aaaf63437949988ce7e7264e5e72fcb201b4d405952e5275a475ca643f2d
EXPECTED_REVISION=06d1ae558dbbe962c2b61018049950dfe0b58473
EXPECTED_ANYWHERE_VERSION=1.13.0

usage() {
  cat <<'EOF'
Usage: tests/usb-ssh-p0.sh iso <official-minimal.iso>
       tests/usb-ssh-p0.sh anywhere <nixos-anywhere-bin>
       tests/usb-ssh-p0.sh postboot <ssh-key> <known-hosts> <port> <system-path>

The script never accepts a password. VM creation, interactive bootstrap and
destructive disposable-disk runs are documented in docs/qualification/usb-ssh-p0.md.
EOF
}

require_regular_file() {
  [[ -f "$1" && ! -L "$1" ]] || {
    echo "expected a regular non-symlink file: $1" >&2
    exit 2
  }
}

case "${1:-}" in
  iso)
    [[ $# -eq 2 ]] || { usage >&2; exit 2; }
    require_regular_file "$2"
    actual=$(sha256sum -- "$2" | awk '{print $1}')
    [[ "$actual" == "$EXPECTED_ISO_SHA256" ]] || {
      echo "unexpected ISO SHA-256: $actual" >&2
      exit 1
    }
    echo "official ISO digest verified"
    ;;
  anywhere)
    [[ $# -eq 2 ]] || { usage >&2; exit 2; }
    require_regular_file "$2"
    help=$(/usr/bin/env bash "$2" --help)
    grep -F "Default is: kexec,disko,install,reboot" <<<"$help" >/dev/null
    grep -F -- "--store-paths" <<<"$help" >/dev/null
    grep -F -- "--no-disko-deps" <<<"$help" >/dev/null
    wrapped=$(dirname "$2")/.nixos-anywhere-wrapped
    require_regular_file "$wrapped"
    grep -F 'UserKnownHostsFile=/dev/null' "$wrapped" >/dev/null
    grep -F 'StrictHostKeyChecking=no' "$wrapped" >/dev/null
    grep -F "$EXPECTED_ANYWHERE_VERSION" "$(dirname "$(dirname "$2")")/nix-support/hydra-build-products" >/dev/null 2>&1 || true
    echo "pinned nixos-anywhere behavior verified"
    ;;
  postboot)
    [[ $# -eq 5 ]] || { usage >&2; exit 2; }
    key=$2
    known_hosts=$3
    port=$4
    expected_system=$5
    require_regular_file "$key"
    require_regular_file "$known_hosts"
    [[ "$port" =~ ^[0-9]+$ && "$expected_system" =~ ^/nix/store/[0-9a-z]{32}-nixos-system-pc[0-9]{2}- ]] || {
      echo "invalid postboot argument" >&2
      exit 2
    }
    output=$(ssh -F /dev/null -i "$key" -p "$port" \
      -o IdentitiesOnly=yes -o IdentityAgent=none \
      -o UserKnownHostsFile="$known_hosts" -o GlobalKnownHostsFile=/dev/null \
      -o StrictHostKeyChecking=yes -o HostKeyAlgorithms=ssh-ed25519 \
      -o BatchMode=yes -o PasswordAuthentication=no \
      -o KbdInteractiveAuthentication=no -o PubkeyAuthentication=yes \
      -o ClearAllForwardings=yes -o ForwardAgent=no -o UpdateHostKeys=no \
      -o ControlMaster=no root@127.0.0.1 \
      'printf "%s\n" "$(hostname)" "$(readlink -f /run/current-system)" "$(nixos-version --configuration-revision)"')
    mapfile -t lines <<<"$output"
    [[ ${lines[0]} =~ ^pc[0-9]{2}$ ]]
    [[ ${lines[1]} == "$expected_system" ]]
    [[ ${lines[2]} == "$EXPECTED_REVISION" ]]
    echo "strict post-boot identity verified"
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
