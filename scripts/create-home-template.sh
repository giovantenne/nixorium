#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 5 ]]; then
  echo "Usage: create-home-template.sh <template-dir> <git-name> <git-email> <xdg-bin> <assets-dir>" >&2
  exit 1
fi

TEMPLATE_DIR="$1"
GIT_NAME="$2"
GIT_EMAIL="$3"
XDG_USER_DIRS_BIN="$4"
ASSETS_DIR="$5"

echo "Creating home template..."

# Clean and recreate template directory
rm -rf "$TEMPLATE_DIR"
mkdir -p "$TEMPLATE_DIR"

# Create .bashrc that sources system profile
cat > "$TEMPLATE_DIR/.bashrc" << 'EOF'
# Source global definitions
if [ -f /etc/bashrc ]; then
  . /etc/bashrc
fi
EOF

# Create .profile for login shells (source bashrc)
cat > "$TEMPLATE_DIR/.profile" << 'EOF'
# Source bashrc
if [ -f ~/.bashrc ]; then
  . ~/.bashrc
fi
EOF

# Create .gitconfig
cat > "$TEMPLATE_DIR/.gitconfig" << EOF
[user]
    name = $GIT_NAME
    email = $GIT_EMAIL
EOF

# Create XDG standard directories using xdg-user-dirs
HOME="$TEMPLATE_DIR" "$XDG_USER_DIRS_BIN" --force

# Create config directories
mkdir -p "$TEMPLATE_DIR/.config"
mkdir -p "$TEMPLATE_DIR/.local/share"
mkdir -p "$TEMPLATE_DIR/.local/npm"

# Copy mimeapps.list
cp "$ASSETS_DIR/mimeapps.list" "$TEMPLATE_DIR/.config/mimeapps.list"

# Copy VS Code settings
mkdir -p "$TEMPLATE_DIR/.config/Code/User"
cp "$ASSETS_DIR/vscode-settings.json" "$TEMPLATE_DIR/.config/Code/User/settings.json"

# Create VS Code argv.json to use basic password store (avoids gnome-keyring warning)
mkdir -p "$TEMPLATE_DIR/.vscode"
cat > "$TEMPLATE_DIR/.vscode/argv.json" << 'EOF'
{
  "password-store": "basic",
  "enable-crash-reporter": false
}
EOF

echo "Home template created successfully"
