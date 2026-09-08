#!/usr/bin/env bash
set -euo pipefail

DEFAULT_RELEASE="v2.0.0-beta.3"
RELEASE="${NIXORIUM_RELEASE:-}"
INSTALL_DISK=""
REPOSITORY="giovantenne/nixorium"
RELEASES_API_URL="https://api.github.com/repos/${REPOSITORY}/releases?per_page=100"
TARGET_ROOT="${NIXORIUM_TARGET_ROOT:-/mnt}"
ADMIN_USER="admin"
DEPLOYMENT_NAME="nixorium-deployment"
INSTALLER_REF="${NIXORIUM_INSTALLER_REF:-}"
INSTALLER_ARGS=()

# Keep bootstrap downloads independent from any cache configured in the live environment.
export NIX_CONFIG=$'experimental-features = nix-command flakes\nsubstituters = https://cache.nixos.org/\ntrusted-public-keys = cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQX27P3FJrRo='

usage() {
  echo "Usage: install.sh [--release <tag>] [--disk <device>]" >&2
  echo "Example: install.sh --release v2.0.0-beta.3 --disk /dev/sda" >&2
}

choose_release() {
  local API_RESPONSE
  local CHOICE
  local CHOICE_NUMBER
  local INDEX
  local LABEL
  local RELEASED_TAG
  local TAG
  local TAG_ALREADY_LISTED
  local -a AVAILABLE_RELEASES=()

  echo "Fetching published Nixorium releases..." >&3
  if API_RESPONSE="$(curl -fsSL "$RELEASES_API_URL")"; then
    while IFS= read -r TAG; do
      if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
        continue
      fi

      TAG_ALREADY_LISTED=false
      for RELEASED_TAG in "${AVAILABLE_RELEASES[@]}"; do
        if [[ "$RELEASED_TAG" == "$TAG" ]]; then
          TAG_ALREADY_LISTED=true
          break
        fi
      done

      if [[ "$TAG_ALREADY_LISTED" == "false" ]]; then
        AVAILABLE_RELEASES+=("$TAG")
      fi
    done < <(
      printf '%s\n' "$API_RESPONSE" |
        sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\(v[^"]*\)",*$/\1/p'
    )
  else
    echo "Warning: could not retrieve the GitHub release list." >&3
  fi

  TAG_ALREADY_LISTED=false
  for TAG in "${AVAILABLE_RELEASES[@]}"; do
    if [[ "$TAG" == "$DEFAULT_RELEASE" ]]; then
      TAG_ALREADY_LISTED=true
      break
    fi
  done
  if [[ "$TAG_ALREADY_LISTED" == "false" ]]; then
    AVAILABLE_RELEASES=("$DEFAULT_RELEASE" "${AVAILABLE_RELEASES[@]}")
  fi

  echo >&3
  echo "Select a tagged Nixorium release:" >&3
  for INDEX in "${!AVAILABLE_RELEASES[@]}"; do
    TAG="${AVAILABLE_RELEASES[$INDEX]}"
    if [[ "$TAG" == *-* ]]; then
      LABEL="prerelease"
    else
      LABEL="stable"
    fi
    if [[ "$TAG" == "$DEFAULT_RELEASE" ]]; then
      LABEL="${LABEL}, default"
    fi
    printf '  %d) %s (%s)\n' "$((INDEX + 1))" "$TAG" "$LABEL" >&3
  done

  while true; do
    printf 'Release [%s]: ' "$DEFAULT_RELEASE" >&3
    IFS= read -r CHOICE <&3

    if [[ -z "$CHOICE" ]]; then
      RELEASE="$DEFAULT_RELEASE"
      break
    fi
    if [[ "$CHOICE" =~ ^[0-9]+$ ]]; then
      CHOICE_NUMBER=$((10#$CHOICE))
      if (( CHOICE_NUMBER >= 1 && CHOICE_NUMBER <= ${#AVAILABLE_RELEASES[@]} )); then
        RELEASE="${AVAILABLE_RELEASES[$((CHOICE_NUMBER - 1))]}"
        break
      fi
    fi

    echo "Invalid selection. Enter a number from 1 to ${#AVAILABLE_RELEASES[@]}, or press Enter for ${DEFAULT_RELEASE}." >&3
  done

  echo "Selected ${RELEASE}." >&3
  echo >&3
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

if [[ -z "$RELEASE" ]]; then
  if { exec 3<>/dev/tty; } 2>/dev/null; then
    choose_release
    exec 3>&-
  else
    RELEASE="$DEFAULT_RELEASE"
    echo "No interactive terminal detected; using ${RELEASE}." >&2
    echo "Pass --release <tag> or set NIXORIUM_RELEASE to choose explicitly." >&2
  fi
fi

if [[ ! "$RELEASE" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "Error: '$RELEASE' is not a valid release tag." >&2
  exit 1
fi

if [[ -z "$INSTALLER_REF" ]]; then
  INSTALLER_REF="$RELEASE"
fi

if [[ "$TARGET_ROOT" != /* || "$TARGET_ROOT" == "/" ]]; then
  echo "Error: NIXORIUM_TARGET_ROOT must be an absolute mount path other than /." >&2
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

echo "Preparing Nixorium ${RELEASE}..."
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
    -c user.name="Nixorium Installer" \
    -c user.email="installer@nixorium.local" \
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
