#!/usr/bin/env bash
set -euo pipefail

# Monitor GNOME idle time and launch TTE screensaver + lock screen.
# Replaces GNOME's built-in screensaver with a custom TTE animation.
#
# Timeline:
#   IDLE_SCREENSAVER_MS -> launch screensaver
#   IDLE_LOCK_MS        -> lock screen (loginctl lock-session)
#   On activity          -> kill screensaver

# Idle thresholds (milliseconds)
IDLE_SCREENSAVER_MS=150000  # 2.5 minutes
IDLE_LOCK_MS=160000         # ~2.5 min + 10 seconds

SCREENSAVER_CLASS="org.meucci.screensaver"
POLL_INTERVAL=2

SCREENSAVER_ACTIVE=false
LOCK_SENT=false

get_idle_ms() {
  gdbus call --session \
    --dest org.gnome.Mutter.IdleMonitor \
    --object-path /org/gnome/Mutter/IdleMonitor/Core \
    --method org.gnome.Mutter.IdleMonitor.GetIdletime 2>/dev/null |
  grep -oP '(?<=uint64 )\d+' || echo "0"
}

launch_screensaver() {
  if ! pgrep -f "$SCREENSAVER_CLASS" >/dev/null 2>&1; then
    /etc/lab/launch-screensaver.sh &
  fi
}

kill_screensaver() {
  pkill -f "$SCREENSAVER_CLASS" 2>/dev/null || true
}

lock_screen() {
  loginctl lock-session 2>/dev/null || true
}

while true; do
  IDLE_MS=$(get_idle_ms)

  if [[ "$IDLE_MS" -ge "$IDLE_LOCK_MS" ]] && [[ "$LOCK_SENT" == "false" ]]; then
    kill_screensaver
    lock_screen
    LOCK_SENT=true
  elif [[ "$IDLE_MS" -ge "$IDLE_SCREENSAVER_MS" ]] && [[ "$SCREENSAVER_ACTIVE" == "false" ]]; then
    launch_screensaver
    SCREENSAVER_ACTIVE=true
  elif [[ "$IDLE_MS" -lt "$IDLE_SCREENSAVER_MS" ]] && [[ "$SCREENSAVER_ACTIVE" == "true" ]]; then
    kill_screensaver
    SCREENSAVER_ACTIVE=false
    LOCK_SENT=false
  fi

  sleep "$POLL_INTERVAL"
done
