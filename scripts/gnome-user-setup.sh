#!/usr/bin/env bash
# Set GNOME favorites, theme, and dismiss welcome dialog for lab users
set -euo pipefail

# Only run for lab users
case "${USER:-}" in
  informatica|admin|docente) ;;
  *) exit 0 ;;
esac

# Wait for GNOME shell to be ready
sleep 2

# Dock and shell settings
if [ "${USER:-}" = "informatica" ]; then
  gsettings set org.gnome.shell favorite-apps \
    "['com.mitchellh.ghostty.desktop', 'chromium-browser.desktop', 'code.desktop', 'org.gnome.Nautilus.desktop', 'org.gnome.TextEditor.desktop']"
else
  current_favorites=$(gsettings get org.gnome.shell favorite-apps)
  updated_favorites=$(python3 - "$current_favorites" << 'PY'
import ast
import sys

raw = sys.argv[1].strip()
if raw.startswith("@as "):
    raw = raw[4:]

favorites = ast.literal_eval(raw)
if "io.veyon.desktop" not in favorites:
    if "code.desktop" in favorites:
        favorites.insert(favorites.index("code.desktop") + 1, "io.veyon.desktop")
    else:
        favorites.append("io.veyon.desktop")
print(repr(favorites))
PY
  ) || updated_favorites=""
  if [[ -n "$updated_favorites" ]]; then
    gsettings set org.gnome.shell favorite-apps "$updated_favorites"
  fi
fi

if gsettings list-schemas | grep -qx "org.gnome.shell.extensions.dash-to-dock"; then
  gsettings set org.gnome.shell.extensions.dash-to-dock show-trash false
fi

gsettings set org.gnome.shell welcome-dialog-last-shown-version '9999'

# Terminal defaults and global shortcuts
gsettings set org.gnome.desktop.default-applications.terminal exec 'ghostty'
gsettings set org.gnome.desktop.default-applications.terminal exec-arg '--'
gsettings set org.gnome.settings-daemon.plugins.media-keys custom-keybindings \
  "['/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom0/', '/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom1/', '/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom2/']"
gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom0/ name 'Ghostty'
gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom0/ command '/run/current-system/sw/bin/ghostty'
gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom0/ binding '<Super>Return'
gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom1/ name 'Chromium'
gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom1/ command '/run/current-system/sw/bin/chromium'
gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom1/ binding '<Super><Shift>Return'
gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom2/ name 'Code'
gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom2/ command '/run/current-system/sw/bin/code'
gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom2/ binding '<Primary><Shift>c'
