#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "Usage: home-reset.sh <snapshots-dir> <home-dir> <template-dir> <owner>" >&2
  exit 1
fi

SNAPSHOTS_DIR="$1"
HOME_DIR="$2"
TEMPLATE_DIR="$3"
OWNER="$4"
KEYFILE_DIR=""

cleanup() {
  if [[ -n "$KEYFILE_DIR" ]]; then
    rm -rf "$KEYFILE_DIR"
  fi
}
trap cleanup EXIT

# Validate template exists before doing anything destructive
if [ ! -d "$TEMPLATE_DIR" ]; then
  echo "Error: template directory '$TEMPLATE_DIR' does not exist" >&2
  exit 1
fi

# Safety: ensure HOME_DIR is a real absolute path under /home
if [[ "$HOME_DIR" != /home/* ]]; then
  echo "Error: home directory must be under /home/" >&2
  exit 1
fi

# If something goes wrong between delete and copy, log a critical error.
trap 'echo "CRITICAL: home-reset failed, home may be incomplete" >&2' ERR

echo "Starting home reset..."

# Ensure snapshots directory exists
mkdir -p "$SNAPSHOTS_DIR"

# Runtime caches and installed tools are reproducible/replaceable and can be
# very large. Exclude them from the recoverable work snapshots.
EPHEMERAL_PATHS=(
  ".local/share/docker"
  ".local/npm"
  ".npm"
)

for EPHEMERAL_PATH in "${EPHEMERAL_PATHS[@]}"; do
  if [ -e "$HOME_DIR/$EPHEMERAL_PATH" ]; then
    rm -rf "$HOME_DIR/$EPHEMERAL_PATH"
  fi
done

# Check if home has any content (not first boot)
if [ -n "$(find "$HOME_DIR" -maxdepth 1 -mindepth 1 -print -quit 2>/dev/null)" ]; then
  echo "Rotating snapshots..."

  # Remove oldest snapshot (5)
  if [ -d "$SNAPSHOTS_DIR/snapshot-5" ]; then
    if btrfs subvolume show "$SNAPSHOTS_DIR/snapshot-5" >/dev/null 2>&1; then
      btrfs subvolume delete "$SNAPSHOTS_DIR/snapshot-5"
    else
      rm -rf "$SNAPSHOTS_DIR/snapshot-5"
    fi
  fi

  # Rotate snapshots: 4->5, 3->4, 2->3, 1->2
  for I in 4 3 2 1; do
    NEXT=$((I + 1))
    if [ -d "$SNAPSHOTS_DIR/snapshot-$I" ]; then
      mv "$SNAPSHOTS_DIR/snapshot-$I" "$SNAPSHOTS_DIR/snapshot-$NEXT"
    fi
  done

  # Create new snapshot of current home
  echo "Creating snapshot of current home..."
  btrfs subvolume snapshot "$HOME_DIR" "$SNAPSHOTS_DIR/snapshot-1"

  # Make snapshot accessible to admin/teacher (veyon-master group)
  chgrp veyon-master "$SNAPSHOTS_DIR/snapshot-1"
  chmod 750 "$SNAPSHOTS_DIR/snapshot-1"
  chmod -R g+rX "$SNAPSHOTS_DIR/snapshot-1"
fi

# Clear home directory content while keeping the mounted subvolume itself.
echo "Clearing home directory..."
if ! find "$HOME_DIR" -xdev -mindepth 1 -delete; then
  echo "Error: could not completely clear '$HOME_DIR'." >&2
  echo "Nested mounts or Btrfs subvolumes must be removed before login can continue." >&2
  exit 1
fi

if [ -n "$(find "$HOME_DIR" -xdev -mindepth 1 -print -quit)" ]; then
  echo "Error: '$HOME_DIR' is not empty after cleanup; refusing to restore over stale data." >&2
  exit 1
fi

# Copy template to home
echo "Copying template to home..."
cp -a "$TEMPLATE_DIR/." "$HOME_DIR/"

# Pick a random wallpaper and write dconf user database
BACKGROUNDS_DIR="/etc/lab/backgrounds"
if [ -d "$BACKGROUNDS_DIR" ]; then
  WALLPAPERS=("$BACKGROUNDS_DIR"/*.jpg)
  if [ ${#WALLPAPERS[@]} -gt 0 ]; then
    PICK="${WALLPAPERS[$((RANDOM % ${#WALLPAPERS[@]}))]}"
    DCONF_DIR="$HOME_DIR/.config/dconf"
    mkdir -p "$DCONF_DIR"
    KEYFILE_DIR=$(mktemp -d)
    cat > "$KEYFILE_DIR/user.txt" <<EOF
[org/gnome/desktop/background]
picture-uri='file://${PICK}'
picture-uri-dark='file://${PICK}'
picture-options='zoom'

[org/gnome/desktop/screensaver]
picture-uri='file://${PICK}'
picture-options='zoom'

[org/gnome/nautilus/icon-view]
default-zoom-level='small'
EOF
    dconf compile "$DCONF_DIR/user" "$KEYFILE_DIR"
    rm -rf "$KEYFILE_DIR"
    KEYFILE_DIR=""
    echo "Wallpaper set to $(basename "$PICK")"
  fi
fi

# Ensure correct ownership
chown -R "$OWNER" "$HOME_DIR"
chmod 700 "$HOME_DIR"

echo "Home reset completed"
