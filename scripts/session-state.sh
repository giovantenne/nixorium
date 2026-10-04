#!/usr/bin/env bash
set -euo pipefail

# Report whether anyone is using this computer, for power-control reviews.
# Output is one word, a versioned contract with the management controller:
#   idle    no user session
#   unused  user sessions exist, but none received keyboard or mouse input
#           recently or since it started (for example an untouched autologin)
#   active  a user session is in use, or its use cannot be determined
# Exit code 2 means unavailable.

# A session without input for this long counts as not in use.
UNUSED_AFTER_MS=600000
# Input older than the session start minus this margin means "never touched".
LOGIN_MARGIN_MS=30000

read_uptime() {
  local SECONDS_UP
  read -r SECONDS_UP _ < /proc/uptime
  printf '%s\n' "$SECONDS_UP"
}

bus_available() {
  [[ -S "$1" ]]
}

# Only user-class sessions are logins; "manager" and "background" sessions
# run services. An unreadable class counts as a login.
interactive_session() {
  local CLASS
  CLASS="$(loginctl show-session "$1" --property=Class --value 2>/dev/null)" || return 0
  [[ "$CLASS" != manager && "$CLASS" != manager-early && "$CLASS" != background && "$CLASS" != background-light ]]
}

# Succeeds only when the session demonstrably has no recent input. Any
# failure to observe it keeps the conservative "in use" answer.
session_unused() {
  local SESSION="$1"
  local USER_ID="$2"
  local TYPE
  local REMOTE
  local STARTED_US
  local UPTIME
  local USER_NAME
  local BUS
  local REPLY
  local IDLE_MS
  local AGE_MS

  TYPE="$(loginctl show-session "$SESSION" --property=Type --value 2>/dev/null)" || return 1
  REMOTE="$(loginctl show-session "$SESSION" --property=Remote --value 2>/dev/null)" || return 1
  [[ "$TYPE" == wayland || "$TYPE" == x11 ]] || return 1
  [[ "$REMOTE" == no ]] || return 1
  STARTED_US="$(loginctl show-session "$SESSION" --property=TimestampMonotonic --value 2>/dev/null)" || return 1
  [[ "$STARTED_US" =~ ^[0-9]+$ ]] || return 1
  UPTIME="$(read_uptime)" || return 1
  [[ "$UPTIME" =~ ^([0-9]+)\.([0-9]{2}) ]] || return 1
  AGE_MS=$(( (10#${BASH_REMATCH[1]} * 1000 + 10#${BASH_REMATCH[2]} * 10) - STARTED_US / 1000 ))

  USER_NAME="$(id -nu "$USER_ID" 2>/dev/null)" || return 1
  BUS="/run/user/${USER_ID}/bus"
  bus_available "$BUS" || return 1
  # Ask the user's own GNOME Shell for the time since the last input event.
  REPLY="$(runuser -u "$USER_NAME" -- env DBUS_SESSION_BUS_ADDRESS="unix:path=${BUS}" \
    timeout 5 gdbus call --session --dest org.gnome.Mutter.IdleMonitor \
    --object-path /org/gnome/Mutter/IdleMonitor/Core \
    --method org.gnome.Mutter.IdleMonitor.GetIdletime 2>/dev/null)" || return 1
  [[ "$REPLY" =~ ^\(uint64\ ([0-9]+),\)$ ]] || return 1
  IDLE_MS="${BASH_REMATCH[1]}"

  (( IDLE_MS >= UNUSED_AFTER_MS || IDLE_MS + LOGIN_MARGIN_MS >= AGE_MS ))
}

main() {
  local SESSIONS
  local SESSION
  local USER_ID
  local STATE="idle"

  SESSIONS="$(loginctl list-sessions --no-legend --no-pager)" \
    || { echo "session inventory is unavailable" >&2; exit 2; }
  while read -r SESSION USER_ID _; do
    [[ -z "${SESSION:-}" ]] && continue
    [[ "$USER_ID" =~ ^[0-9]+$ ]] \
      || { echo "session inventory is invalid" >&2; exit 2; }
    if (( USER_ID >= 1000 )); then
      # Service managers and background sessions are not interactive use.
      interactive_session "$SESSION" || continue
      if ! session_unused "$SESSION" "$USER_ID"; then
        printf 'active\n'
        exit 0
      fi
      STATE="unused"
    fi
  done <<< "$SESSIONS"
  printf '%s\n' "$STATE"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
