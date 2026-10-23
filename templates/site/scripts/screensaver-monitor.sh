#!/usr/bin/env bash
set -euo pipefail

# Monitor GNOME idle time and run the text-effects screensaver.
# It never locks the session.
#
# Timeline:
#   IDLE_SCREENSAVER_MS -> launch screensaver
#   IDLE_BLANK_MS       -> stop it: GNOME blanks the screen at the same time
#                          (idle-delay in the core desktop module), no lock
#   On activity          -> kill screensaver
#
# The service passes the Nix store paths of the terminal script and the logo:
#   SCREENSAVER_SHELL bash used to run SCREENSAVER_CMD (it needs no executable bit)
#   SCREENSAVER_CMD   script that runs the effects inside the terminal
#   SCREENSAVER_TEXT  text file the effects animate
: "${SCREENSAVER_SHELL:=bash}"
: "${SCREENSAVER_CMD:?SCREENSAVER_CMD is not set}"
: "${SCREENSAVER_TEXT:?SCREENSAVER_TEXT is not set}"

# Idle thresholds (milliseconds)
IDLE_SCREENSAVER_MS=300000
IDLE_BLANK_MS=1800000

SCREENSAVER_CLASS="org.nixorium.screensaver"
POLL_INTERVAL=2

SCREENSAVER_ACTIVE=false

get_idle_ms() {
  gdbus call --session \
    --dest org.gnome.Mutter.IdleMonitor \
    --object-path /org/gnome/Mutter/IdleMonitor/Core \
    --method org.gnome.Mutter.IdleMonitor.GetIdletime 2>/dev/null |
  grep -oP '(?<=uint64 )\d+' || echo "0"
}

launch_screensaver() {
  # Skip if already running
  if pgrep -f "$SCREENSAVER_CLASS" >/dev/null 2>&1; then
    return
  fi

  # Fullscreen Ghostty running the screensaver. The text path is an argument
  # because the window may be created by an already running Ghostty process.
  # Palette colors 0 and 8 are forced to pure black to avoid grey lines in TTE
  # effects.
  ghostty \
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
    -e "$SCREENSAVER_SHELL" "$SCREENSAVER_CMD" "$SCREENSAVER_TEXT" &
}

kill_screensaver() {
  pkill -f "$SCREENSAVER_CLASS" 2>/dev/null || true
}

while true; do
  IDLE_MS=$(get_idle_ms)

  if [[ "$IDLE_MS" -ge "$IDLE_BLANK_MS" ]]; then
    # The screen is blank: nothing to animate, nothing to spend CPU on.
    if pgrep -f "$SCREENSAVER_CLASS" >/dev/null 2>&1; then
      kill_screensaver
    fi
  elif [[ "$IDLE_MS" -ge "$IDLE_SCREENSAVER_MS" ]] && [[ "$SCREENSAVER_ACTIVE" == "false" ]]; then
    launch_screensaver
    SCREENSAVER_ACTIVE=true
  elif [[ "$IDLE_MS" -lt "$IDLE_SCREENSAVER_MS" ]] && [[ "$SCREENSAVER_ACTIVE" == "true" ]]; then
    kill_screensaver
    SCREENSAVER_ACTIVE=false
  fi

  sleep "$POLL_INTERVAL"
done
