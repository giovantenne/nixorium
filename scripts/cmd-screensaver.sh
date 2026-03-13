#!/usr/bin/env bash
set -euo pipefail

# Run the lab screensaver using random TTE effects.
# This script is meant to be launched inside a fullscreen terminal.

SCREENSAVER_TEXT="/etc/lab/screensaver.txt"

if [[ ! -f "$SCREENSAVER_TEXT" ]]; then
  echo "Error: $SCREENSAVER_TEXT not found" >&2
  exit 1
fi

# Hide the cursor
tput civis 2>/dev/null || true

STTY_STATE="$(stty -g)"

# Draw in the terminal alternate screen to avoid scrollback/cursor drift.
tput smcup 2>/dev/null || true

# Restore terminal state on exit
trap 'stty "$STTY_STATE" 2>/dev/null || true; tput cnorm 2>/dev/null || true; tput rmcup 2>/dev/null || true' EXIT

# Non-blocking reads
stty -echo -icanon time 0 min 0

reset_canvas() {
  # Home + full clear (+ scrollback clear when supported).
  printf '\033[H\033[2J\033[3J'
}

# Clear screen
reset_canvas

# Available effects
EFFECTS=(
  beams binarypath blackhole bouncyballs bubbles burn colorshift
  crumble decrypt errorcorrect expand fireworks highlight laseretch
  matrix middleout orbittingvolley overflow pour print rain
  randomsequence rings scattered slice slide spotlights spray
  swarm sweep synthgrid unstable vhstape waves wipe
)

while true; do
  EFFECT="${EFFECTS[$((RANDOM % ${#EFFECTS[@]}))]}"

  # Ensure each effect starts from a stable full-screen origin.
  reset_canvas

  tte -i "$SCREENSAVER_TEXT" \
    --frame-rate 120 \
    --canvas-width 0 \
    --canvas-height 0 \
    --anchor-canvas c \
    --anchor-text c \
    "$EFFECT" 2>/dev/null &
  TTE_PID=$!

  while kill -0 "$TTE_PID" 2>/dev/null; do
    if read -rsn1 -t 0.1 _; then
      kill "$TTE_PID" 2>/dev/null || true
      exit 0
    fi
  done

  wait "$TTE_PID" 2>/dev/null || true

  sleep 1
  reset_canvas
done
