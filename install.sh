#!/usr/bin/env bash
set -euo pipefail

DEFAULT_RELEASE="v2.0.0-beta.2"
RELEASE="${NIXOS_LAB_RELEASE:-$DEFAULT_RELEASE}"
INSTALL_DISK=""
REPOSITORY="giovantenne/nixos-lab"
TARGET_ROOT="${NIXOS_LAB_TARGET_ROOT:-/mnt}"
ADMIN_USER="admin"
DEPLOYMENT_NAME="nixos-lab-deployment"
INSTALLER_REF="${NIXOS_LAB_INSTALLER_REF:-}"
INSTALLER_ARGS=()

# Keep bootstrap downloads independent from any cache configured in the live environment.
export NIX_CONFIG=$'experimental-features = nix-command flakes\nsubstituters = https://cache.nixos.org/\ntrusted-public-keys = cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQX27P3FJrRo='

usage() {
  echo "Usage: install.sh [--release <tag>] [--disk <device>]" >&2
  echo "Example: install.sh --release v2.0.0-beta.2 --disk /dev/sda" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --release)
      if [[ $# -lt 2 ]]; then
        echo "Error: --release requires a tag." >&2
        usage
        exit 1
      fi
      RELEASE="$2"
      shift 2
      ;;
    --disk)
      if [[ $# -lt 2 ]]; then
        echo "Error: --disk requires a device." >&2
        usage
        exit 1
      fi
      INSTALL_DISK="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Error: unknown argument '$1'." >&2
      usage
      exit 1
      ;;
  esac
done

if [[ ! "$RELEASE" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "Error: '$RELEASE' is not a valid release tag." >&2
  exit 1
fi

if [[ -z "$INSTALLER_REF" ]]; then
  INSTALLER_REF="$RELEASE"
fi

if [[ "$TARGET_ROOT" != /* || "$TARGET_ROOT" == "/" ]]; then
  echo "Error: NIXOS_LAB_TARGET_ROOT must be an absolute mount path other than /." >&2
  exit 1
fi

UPSTREAM_REF="github:${REPOSITORY}/${RELEASE}"
INSTALLER_URL="https://raw.githubusercontent.com/${REPOSITORY}/${INSTALLER_REF}/scripts/install-controller.sh"
DISKO_LAYOUT_URL="https://raw.githubusercontent.com/${REPOSITORY}/${RELEASE}/lib/disko-layout.nix"
TEMP_INSTALLER="$(mktemp)"
TEMP_DEPLOYMENT="$(mktemp -d)"

cleanup() {
  rm -f "$TEMP_INSTALLER"
  rm -rf "$TEMP_DEPLOYMENT"
}
trap cleanup EXIT

if command -v git >/dev/null 2>&1; then
  GIT_COMMAND=(git)
else
  GIT_COMMAND=(
    nix
    --extra-experimental-features
    "nix-command flakes"
    shell
    nixpkgs#git
    --command
    git
  )
fi

echo "Preparing NixOS Lab ${RELEASE}..."
curl -fsSL "$INSTALLER_URL" -o "$TEMP_INSTALLER"

(
  cd "$TEMP_DEPLOYMENT"
  nix --extra-experimental-features "nix-command flakes" \
    flake init -t "${UPSTREAM_REF}#site"
  "${GIT_COMMAND[@]}" init -b master
  "${GIT_COMMAND[@]}" add .
  nix --extra-experimental-features "nix-command flakes" flake lock
  "${GIT_COMMAND[@]}" add flake.lock
  "${GIT_COMMAND[@]}" \
    -c user.name="NixOS Lab Installer" \
    -c user.email="installer@nixos-lab.local" \
    commit -m "chore: initialize lab deployment"
)

MASTER_HOST_NUMBER="$(
  nix --extra-experimental-features "nix-command flakes" \
    eval "path:${TEMP_DEPLOYMENT}#labMeta.controller.number" --json
)"
STUDENT_USER="$(
  nix --extra-experimental-features "nix-command flakes" \
    eval "path:${TEMP_DEPLOYMENT}#labMeta.users.student" --raw
)"

if [[ -n "$INSTALL_DISK" ]]; then
  INSTALLER_ARGS+=("$INSTALL_DISK")
fi

echo "Installing the controller from the generated private deployment..."
echo "Wait for the final bootstrap completion message before rebooting."
FLAKE_REF="path:${TEMP_DEPLOYMENT}" \
  DISKO_LAYOUT_URL="$DISKO_LAYOUT_URL" \
  MASTER_HOST_NUMBER="$MASTER_HOST_NUMBER" \
  STUDENT_USER="$STUDENT_USER" \
  bash "$TEMP_INSTALLER" "${INSTALLER_ARGS[@]}"

ADMIN_HOME="${TARGET_ROOT}/home/${ADMIN_USER}"
DEPLOYMENT_TARGET="${ADMIN_HOME}/${DEPLOYMENT_NAME}"

if [[ ! -d "$ADMIN_HOME" ]]; then
  echo "Error: the installed admin home was not found at ${ADMIN_HOME}." >&2
  echo "The controller is installed, but the deployment repository must be recreated after reboot." >&2
  exit 1
fi

if sudo test -e "$DEPLOYMENT_TARGET"; then
  echo "Error: deployment target already exists: ${DEPLOYMENT_TARGET}" >&2
  exit 1
fi

ADMIN_OWNER="$(sudo stat -c '%u:%g' "$ADMIN_HOME")"
sudo install -d -m 0700 "$DEPLOYMENT_TARGET"
sudo cp -a "${TEMP_DEPLOYMENT}/." "${DEPLOYMENT_TARGET}/"
sudo chown -R "$ADMIN_OWNER" "$DEPLOYMENT_TARGET"

echo "Installation complete."
echo "After reboot, the deployment repository will be available at:"
echo "  ~/${DEPLOYMENT_NAME}"
echo "Reboot with: reboot"
