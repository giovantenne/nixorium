#!/usr/bin/env bash
set -euo pipefail

# Launch the ITIS Meucci screensaver in a fullscreen Ghostty window.
# Used by the idle service to start the screensaver on inactivity.

SCREENSAVER_CLASS="org.meucci.screensaver"

# Skip if already running
if pgrep -f "$SCREENSAVER_CLASS" >/dev/null 2>&1; then
  exit 0
fi

# Launch Ghostty fullscreen with screensaver
# Override palette color 0 (black) to pure black to avoid grey lines in TTE effects
exec ghostty \
  --class="$SCREENSAVER_CLASS" \
  --fullscreen=true \
  --font-size=18 \
  --background=#000000 \
  --foreground=#f38d70 \
  --cursor-color=#000000 \
  --palette=0=#000000 \
  --palette=8=#000000 \
  --mouse-hide-while-typing=true \
  --window-padding-x=0 \
  --window-padding-y=0 \
  --gtk-titlebar=false \
  -e /etc/lab/cmd-screensaver.sh
