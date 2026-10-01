#!/usr/bin/env bash
set -euo pipefail

# Remove old NixOS system generations after a reviewed plan (ADR 0022).
#
#   nixorium-clean-generations --plan
#   nixorium-clean-generations --apply EXPECT
#
# Always kept: the newest KEEP_GENERATIONS system generations, the generation
# the profile points to, the running system and the booted system. Nothing
# else is touched: user profiles, homes and other garbage-collector roots stay.
# The plan prints a versioned line contract:
#   format 1
#   keep N REASON        (REASON: newest, current, running, booted)
#   remove N
#   free BYTES           (available bytes on the Nix store file system)
#   expect DIGEST        (identifies the exact set of generations to remove)
# Apply recomputes the plan and refuses with exit code 3 ("changed") when the
# digest differs, so a changed computer is never cleaned without a new review.

KEEP_GENERATIONS=10
PROFILE_DIRECTORY=/nix/var/nix/profiles
PROFILE_NAME=system

list_generations() {
  local LINK
  for LINK in "$PROFILE_DIRECTORY/$PROFILE_NAME"-*-link; do
    [[ -L "$LINK" ]] || continue
    LINK="${LINK##*/"$PROFILE_NAME"-}"
    LINK="${LINK%-link}"
    [[ "$LINK" =~ ^[0-9]+$ ]] && printf '%s\n' "$LINK"
  done | sort -rn
}

generation_path() {
  readlink -f "$PROFILE_DIRECTORY/$PROFILE_NAME-$1-link"
}

# The generation number the system profile points to.
profile_generation() {
  local TARGET
  TARGET="$(readlink "$PROFILE_DIRECTORY/$PROFILE_NAME" || true)"
  TARGET="${TARGET##*/}"
  TARGET="${TARGET#"$PROFILE_NAME"-}"
  printf '%s\n' "${TARGET%-link}"
}

running_path() {
  readlink -f /run/current-system
}

booted_path() {
  readlink -f /run/booted-system 2>/dev/null || true
}

store_free_bytes() {
  df --output=avail -B1 /nix/store | tail -n 1 | tr -d ' '
}

delete_generations() {
  /run/current-system/sw/bin/nix-env --profile "$PROFILE_DIRECTORY/$PROFILE_NAME" --delete-generations "$@"
}

collect_garbage() {
  /run/current-system/sw/bin/nix-store --gc
}

rewrite_boot_menu() {
  "$PROFILE_DIRECTORY/$PROFILE_NAME/bin/switch-to-configuration" boot
}

# Prints "keep N REASON" and "remove N" lines, newest first.
classify() {
  local PROFILE RUNNING BOOTED NUMBER PATH_OF COUNT=0
  PROFILE="$(profile_generation)"
  RUNNING="$(running_path)"
  BOOTED="$(booted_path)"
  while read -r NUMBER; do
    [[ -n "$NUMBER" ]] || continue
    PATH_OF="$(generation_path "$NUMBER")"
    COUNT=$((COUNT + 1))
    if (( COUNT <= KEEP_GENERATIONS )); then
      printf 'keep %s newest\n' "$NUMBER"
    elif [[ "$NUMBER" == "$PROFILE" ]]; then
      printf 'keep %s current\n' "$NUMBER"
    elif [[ "$PATH_OF" == "$RUNNING" ]]; then
      printf 'keep %s running\n' "$NUMBER"
    elif [[ -n "$BOOTED" && "$PATH_OF" == "$BOOTED" ]]; then
      printf 'keep %s booted\n' "$NUMBER"
    else
      printf 'remove %s\n' "$NUMBER"
    fi
  done < <(list_generations)
}

removal_digest() {
  local REMOVE="$1"
  printf '%s\n' "$REMOVE" | sha256sum | cut -c1-16
}

removal_list() {
  local CLASSIFIED="$1"
  printf '%s\n' "$CLASSIFIED" | sed -n 's/^remove \([0-9]*\)$/\1/p' | sort -n | tr '\n' ' ' | sed 's/ $//'
}

plan() {
  local CLASSIFIED REMOVE
  CLASSIFIED="$(classify)"
  REMOVE="$(removal_list "$CLASSIFIED")"
  printf 'format 1\n'
  [[ -z "$CLASSIFIED" ]] || printf '%s\n' "$CLASSIFIED"
  printf 'free %s\n' "$(store_free_bytes)"
  printf 'expect %s\n' "$(removal_digest "$REMOVE")"
}

apply() {
  local EXPECT="$1" CLASSIFIED REMOVE BEFORE AFTER BOOT=ok
  [[ "$EXPECT" =~ ^[0-9a-f]{16}$ ]] || { echo "expected removal digest is invalid" >&2; exit 2; }
  CLASSIFIED="$(classify)"
  REMOVE="$(removal_list "$CLASSIFIED")"
  if [[ "$(removal_digest "$REMOVE")" != "$EXPECT" ]]; then
    echo changed
    exit 3
  fi
  BEFORE="$(store_free_bytes)"
  printf 'format 1\n'
  if [[ -z "$REMOVE" ]]; then
    printf 'unchanged\nfree-before %s\nfree-after %s\n' "$BEFORE" "$BEFORE"
    return 0
  fi
  # shellcheck disable=SC2086 # REMOVE holds validated generation numbers.
  delete_generations $REMOVE >&2
  collect_garbage >&2
  rewrite_boot_menu >&2 || BOOT=failed
  AFTER="$(store_free_bytes)"
  printf 'removed %s\nfree-before %s\nfree-after %s\nboot-menu %s\n' "$REMOVE" "$BEFORE" "$AFTER" "$BOOT"
}

main() {
  case "${1:-}" in
    --plan)
      [[ $# -eq 1 ]] || { echo "usage: nixorium-clean-generations --plan | --apply EXPECT" >&2; exit 2; }
      plan
      ;;
    --apply)
      [[ $# -eq 2 ]] || { echo "usage: nixorium-clean-generations --plan | --apply EXPECT" >&2; exit 2; }
      [[ "$(id -u)" == 0 ]] || { echo "cleaning generations requires root" >&2; exit 2; }
      apply "$2"
      ;;
    *)
      echo "usage: nixorium-clean-generations --plan | --apply EXPECT" >&2
      exit 2
      ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
