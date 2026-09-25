#!/usr/bin/env bash
set -euo pipefail

# Called by controller activation, before the sandboxed worker starts.
# Preserve the standard OpenSSH path while isolating writable host trust data.
[[ $# == 3 && "$1" == /* && "$2" =~ ^[0-9]+$ && "$3" =~ ^[0-9]+$ ]] || exit 2
ssh_directory=$1
owner_uid=$2
owner_gid=$3
managed_directory="$ssh_directory/nixorium-known-hosts"
legacy="$ssh_directory/known_hosts"
target="$managed_directory/known_hosts"

fail() { echo "Cannot migrate SSH known_hosts: $*" >&2; exit 1; }
private_directory() {
  local path=$1
  if [[ ! -e "$path" && ! -L "$path" ]]; then
    mkdir -m 0700 -- "$path"
    chown "$owner_uid:$owner_gid" "$path"
  fi
  [[ -d "$path" && ! -L "$path" && "$(stat -c '%u:%a' "$path")" == "$owner_uid:700" ]] \
    || fail "unsafe directory $path"
}
known_hosts_file() {
  local path=$1
  [[ -f "$path" && ! -L "$path" && "$(stat -c %u "$path")" == "$owner_uid" \
      && "$(stat -c %h "$path")" == 1 && "$(stat -c %s "$path")" -le 1048576 ]] \
    || fail "unsafe file $path"
  case "$(stat -c %a "$path")" in 600|644) ;; *) fail "unsafe permissions $path" ;; esac
}

private_directory "$ssh_directory"
private_directory "$managed_directory"
lock="$managed_directory/.nixorium-known-hosts.lock"
if [[ ! -e "$lock" && ! -L "$lock" ]]; then
  (umask 077; set -o noclobber; : > "$lock")
  chown "$owner_uid:$owner_gid" "$lock"
fi
known_hosts_file "$lock"
[[ "$(stat -c %a "$lock")" == 600 ]] || fail "unsafe lock permissions"
exec 9<>"$lock"
flock -x 9

if [[ -L "$legacy" ]]; then
  [[ "$(readlink "$legacy")" == "$target" ]] || fail "unexpected known_hosts symlink"
  known_hosts_file "$target"
  exit 0
fi
if [[ -e "$legacy" ]]; then known_hosts_file "$legacy"; fi
if [[ -e "$target" || -L "$target" ]]; then
  known_hosts_file "$target"
  [[ ! -e "$legacy" ]] || cmp -s "$legacy" "$target" \
    || fail "legacy and managed files differ; retain both for reconciliation"
fi

temporary=$(mktemp -d "$ssh_directory/.nixorium-known-hosts-migration.XXXXXX")
trap 'rm -rf -- "$temporary"' EXIT
if [[ ! -e "$target" ]]; then
  if [[ -e "$legacy" ]]; then cat "$legacy" > "$temporary/data"; else : > "$temporary/data"; fi
  chmod 0600 "$temporary/data"
  chown "$owner_uid:$owner_gid" "$temporary/data"
  sync -f "$temporary/data"
  mv -T "$temporary/data" "$target"
  sync -f "$managed_directory"
fi
ln -s "$target" "$temporary/link"
chown -h "$owner_uid:$owner_gid" "$temporary/link"
mv -T "$temporary/link" "$legacy"
sync -f "$ssh_directory"
