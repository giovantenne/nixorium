#!/usr/bin/env bash
set -euo pipefail

# Launch the lab screensaver in a fullscreen Foot window.
# Used by the idle service to start the screensaver on inactivity.

SCREENSAVER_CLASS="org.nixorium.screensaver"

# Skip if already running
if pgrep -f "$SCREENSAVER_CLASS" >/dev/null 2>&1; then
  exit 0
fi

# Launch Foot fullscreen with screensaver
# Override palette color 0 (black) and bright color 0 to pure black
# to avoid grey lines in TTE effects
exec foot \
  --app-id="$SCREENSAVER_CLASS" \
  --fullscreen \
  --font='JetBrainsMono Nerd Font Mono:size=18' \
  -o colors.background=000000 \
  -o colors.foreground=f38d70 \
  -o colors.cursor-color=000000 \
  -o colors.regular0=000000 \
  -o colors.bright0=000000 \
  -o mouse-hide-while-typing=yes \
  -e /etc/lab/cmd-screensaver.sh
