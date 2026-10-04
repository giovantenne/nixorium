#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

source "$REPO_ROOT/scripts/lib/lab-meta.sh"
touch "$TEST_ROOT/flake.nix"
for COUNT in 0 2; do
  jq -n --argjson count "$COUNT" '{schemaVersion:2,
    controller:{name:"pc99",number:99,staticIp:"",dhcpIp:"192.0.2.10"},
    clients:{count:$count,hosts:[range(1; $count+1) | {name:("pc0" + tostring),ip:("10.0.0." + tostring)}]},
    network:{base:"10.0.0.0",prefixLength:24,ifaceName:"enp1s0",cachePort:5000,pxeHttpPort:8080},
    users:{student:"student",teacher:"teacher"}}' > "$TEST_ROOT/lab-meta.json"
  load_lab_meta "$TEST_ROOT"
  [[ "$LAB_CONTROLLER_STATIC_IP" == "" && "$LAB_CONTROLLER_DHCP_IP" == "192.0.2.10" ]]
  [[ "$LAB_CLIENT_COUNT" == "$COUNT" && "$LAB_NETWORK_BASE" == "10.0.0.0" ]]
  [[ "$LAB_IFACE_NAME" == enp1s0 && "$LAB_CACHE_PORT" == 5000 && "$LAB_PXE_HTTP_PORT" == 8080 ]]
  [[ "$LAB_STUDENT_USER" == student && "$LAB_TEACHER_USER" == teacher ]]
done
# Session helper: fake login and GNOME observations as shell functions.
(
  source "$REPO_ROOT/scripts/session-state.sh"
  SESSION_LIST=""
  FAKE_IDLE_MS=0
  SESSION_TYPE=wayland
  SESSION_REMOTE=no
  BUS_AVAILABLE=yes
  loginctl() {
    case "$1" in
      list-sessions) printf '%s' "$SESSION_LIST" ;;
      show-session)
        case "$3" in
          --property=Type) printf '%s\n' "$SESSION_TYPE" ;;
          --property=Remote) printf '%s\n' "$SESSION_REMOTE" ;;
          --property=TimestampMonotonic) printf '%s\n' 100000000 ;;
          --property=Class) if [[ "$2" == m* ]]; then printf 'manager\n'; else printf 'user\n'; fi ;;
        esac ;;
    esac
  }
  read_uptime() { printf '%s\n' 3700.00; }
  id() { printf 'student\n'; }
  runuser() {
    [[ "$BUS_AVAILABLE" == yes ]] || return 1
    printf '(uint64 %s,)\n' "$FAKE_IDLE_MS"
  }
  BUSES=" /run/user/1000/bus "
  bus_available() { [[ "$BUSES" == *" $1 "* ]]; }
  check() {
    local EXPECTED="$1"
    local ACTUAL
    ACTUAL="$(main)"
    [[ "$ACTUAL" == "$EXPECTED" ]] || { echo "session helper: expected $EXPECTED, got $ACTUAL" >&2; exit 1; }
  }
  check idle
  SESSION_LIST=$'c1 42 gdm seat0\n'
  check idle
  SESSION_LIST=$'2 1000 student seat0\n'
  FAKE_IDLE_MS=5000
  check active
  # Untouched since login (the session started 3600 s ago).
  FAKE_IDLE_MS=3590000
  check unused
  # Away for longer than the threshold.
  FAKE_IDLE_MS=700000
  check unused
  SESSION_TYPE="tty"
  check active
  SESSION_TYPE=wayland
  SESSION_REMOTE=yes
  check active
  SESSION_REMOTE=no
  BUS_AVAILABLE=no
  check active
  BUS_AVAILABLE=yes
  # Every user session must be unused; one without an observable bus is not.
  SESSION_LIST=$'2 1000 student seat0\n3 1001 teacher seat0\n'
  check active
  BUSES=" /run/user/1000/bus /run/user/1001/bus "
  check unused
  # A systemd user manager session alongside an untouched login.
  SESSION_LIST=$'2 1000 student seat0\nm4 1000 student\n'
  check unused
  SESSION_LIST=$'m4 1000 student\n'
  check idle
  SESSION_LIST=$'x notanumber student\n'
  if (main >/dev/null 2>&1); then echo "session helper accepted an invalid inventory" >&2; exit 1; fi
)

# Generation cleanup helper: a fake system profile with 15 generations.
(
  source "$REPO_ROOT/scripts/clean-generations.sh"
  PROFILE_DIRECTORY="$TEST_ROOT/profiles"
  mkdir -p "$PROFILE_DIRECTORY"
  for NUMBER in $(seq 1 15); do
    mkdir -p "$TEST_ROOT/store/system-$NUMBER"
    ln -s "$TEST_ROOT/store/system-$NUMBER" "$PROFILE_DIRECTORY/system-$NUMBER-link"
  done
  ln -s system-15-link "$PROFILE_DIRECTORY/system"
  running_path() { printf '%s\n' "$TEST_ROOT/store/system-2"; }
  booted_path() { printf '%s\n' "$TEST_ROOT/store/system-3"; }
  store_free_bytes() { printf '%s\n' 1000; }
  delete_generations() { printf '%s\n' "$*" > "$TEST_ROOT/deleted"; }
  collect_garbage() { :; }
  rewrite_boot_menu() { :; }
  id() { printf '0\n'; }
  PLAN="$(main --plan)"
  [[ "$(grep -c '^keep .* newest$' <<<"$PLAN")" == 10 ]]
  grep -Fx 'keep 2 running' <<<"$PLAN" >/dev/null
  grep -Fx 'keep 3 booted' <<<"$PLAN" >/dev/null
  [[ "$(sed -n 's/^remove //p' <<<"$PLAN" | sort -n | tr '\n' ' ')" == '1 4 5 ' ]]
  grep -Fx 'free 1000' <<<"$PLAN" >/dev/null
  EXPECT="$(sed -n 's/^expect //p' <<<"$PLAN")"
  [[ "$EXPECT" =~ ^[0-9a-f]{16}$ ]]
  # A different expectation (the computer changed after review) is refused.
  if (main --apply 0000000000000000 >/dev/null); then echo "cleanup accepted a stale review" >&2; exit 1; fi
  [[ ! -e "$TEST_ROOT/deleted" ]]
  RESULT="$(main --apply "$EXPECT")"
  [[ "$(cat "$TEST_ROOT/deleted")" == '1 4 5' ]]
  grep -Fx 'removed 1 4 5' <<<"$RESULT" >/dev/null
  grep -Fx 'boot-menu ok' <<<"$RESULT" >/dev/null
  # Nothing to remove: the helper reports unchanged and deletes nothing.
  rm "$TEST_ROOT/deleted" "$PROFILE_DIRECTORY"/system-{1,4,5}-link
  EXPECT="$(main --plan | sed -n 's/^expect //p')"
  grep -Fx unchanged <<<"$(main --apply "$EXPECT")" >/dev/null
  [[ ! -e "$TEST_ROOT/deleted" ]]
  if (main --apply 'x; rm -rf /' >/dev/null 2>&1); then echo "cleanup accepted an invalid digest" >&2; exit 1; fi
)

# Every next-step code is explained in the troubleshooting guide.
while IFS= read -r CODE; do
  grep -F "\`${CODE}\`" "$REPO_ROOT/docs/troubleshooting.md" >/dev/null \
    || { echo "next-step code ${CODE} is not explained in docs/troubleshooting.md" >&2; exit 1; }
done < <(sed -n 's/^\t"\([A-Z-]*\)": {$/\1/p' "$REPO_ROOT/internal/domain/next_steps.go")
[[ "$(sed -n 's/^\t"\([A-Z-]*\)": {$/\1/p' "$REPO_ROOT/internal/domain/next_steps.go" | wc -l)" -ge 10 ]]

echo 'UX shell regression tests passed.'
