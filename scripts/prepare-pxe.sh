# shellcheck shell=bash
# Body of nixorium-prepare-pxe. modules/management.nix replaces the
# placeholders written between at signs (the deployment path, the operation
# gate and the shared PXE build helpers) and wraps it with
# writeShellApplication.
REPOSITORY=@REPOSITORY@

PROGRESS_FILE="$STATE_DIRECTORY/progress.json"
PROGRESS_STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
PROGRESS_STATE=running
PROGRESS_PHASE=starting
PROGRESS_CURRENT=0
PROGRESS_TOTAL=0
PROGRESS_RECENT='[]'
CLIENTS_FILE=""
MANIFEST_TEMP=""
BUILD_DIRECTORY=""

publish_progress() {
  local state="$1"
  local phase="$2"
  local activity="$3"
  local current="$4"
  local total="$5"
  local temporary

  PROGRESS_STATE="$state"
  PROGRESS_PHASE="$phase"
  PROGRESS_CURRENT="$current"
  PROGRESS_TOTAL="$total"
  if [[ -n "$activity" ]]; then
    PROGRESS_RECENT="$(jq -c --arg activity "$activity" \
      '. + [$activity] | if length > 5 then .[-5:] else . end' \
      <<<"$PROGRESS_RECENT")"
  fi
  temporary="$(mktemp "$STATE_DIRECTORY/.progress.XXXXXX")"
  jq -n \
    --arg operation pxe-prepare \
    --arg state "$PROGRESS_STATE" \
    --arg phase "$PROGRESS_PHASE" \
    --arg startedAt "$PROGRESS_STARTED_AT" \
    --arg updatedAt "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" \
    --argjson current "$PROGRESS_CURRENT" \
    --argjson total "$PROGRESS_TOTAL" \
    --argjson recent "$PROGRESS_RECENT" \
    '{
      schemaVersion: 1,
      operation: $operation,
      state: $state,
      phase: $phase,
      startedAt: $startedAt,
      updatedAt: $updatedAt,
      current: $current,
      total: $total,
      recent: $recent
    }' >"$temporary"
  chmod 0600 "$temporary"
  mv -fT -- "$temporary" "$PROGRESS_FILE"
  [[ -z "$activity" ]] || echo "$activity"
}

cleanup() {
  local status="$?"
  [[ -z "$CLIENTS_FILE" ]] || rm -f -- "$CLIENTS_FILE"
  [[ -z "$MANIFEST_TEMP" ]] || rm -f -- "$MANIFEST_TEMP"
  [[ -z "$BUILD_DIRECTORY" ]] || rm -rf -- "$BUILD_DIRECTORY"
  if [[ "$status" -ne 0 && "$PROGRESS_STATE" == running ]]; then
    publish_progress failed "$PROGRESS_PHASE" \
      "Preparation stopped unexpectedly" "$PROGRESS_CURRENT" "$PROGRESS_TOTAL" || true
  fi
}
trap cleanup EXIT

fail() {
  local message="$*"
  if [[ "$PROGRESS_STATE" == running ]]; then
    publish_progress failed "$PROGRESS_PHASE" \
      "Failed: $message" "$PROGRESS_CURRENT" "$PROGRESS_TOTAL" || true
  fi
  echo "Error: $message" >&2
  exit 1
}

@OPERATION_GATE@

publish_progress running starting "Starting PXE preparation" 0 0
publish_progress running validate "Validating the reviewed deployment" 0 0

repo_git() {
  GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
    git -c core.fsmonitor=false -c core.hooksPath=/dev/null -C "$REPOSITORY" "$@"
}

[[ -d "$REPOSITORY" && ! -L "$REPOSITORY" ]] \
  || fail "configured deployment path is not a real directory"
[[ -f "$REPOSITORY/flake.nix" && -d "$REPOSITORY/.git" ]] \
  || fail "configured deployment path is not a Git Flake"
[[ "$(stat -c '%U' "$REPOSITORY")" == admin ]] \
  || fail "deployment repository must be owned by admin"
[[ -z "$(repo_git status --porcelain=v1 --untracked-files=normal)" ]] \
  || fail "deployment worktree must be clean before PXE preparation"
[[ -z "$(repo_git ls-files -- \
    secret-key admin-ssh lab-credentials.json)" ]] \
  || fail "private key files must not be tracked by Git"

[[ -d /var/cache/nixorium/admin && ! -L /var/cache/nixorium/admin \
    && "$(stat -c '%U:%G:%a' /var/cache/nixorium/admin)" == admin:users:700 ]] \
  || fail "administrator Nix cache directory has unsafe ownership or permissions"
export XDG_CACHE_HOME=/var/cache/nixorium/admin
export NIX_CONFIG="experimental-features = nix-command flakes"

REVISION="$(repo_git rev-parse HEAD)" \
  || fail "could not resolve deployment revision"
FLAKE_URL="$(nixorium config source --repo "$REPOSITORY" --revision "$REVISION")"

nixorium config validate --repo "$REPOSITORY" --json >/dev/null \
  || fail "deployment configuration validation failed"
[[ "$(nix eval "$FLAKE_URL#deploymentStatus.ready" --json --no-write-lock-file)" == true ]] \
  || fail "deploymentStatus.ready must be true before PXE preparation"

publish_progress running validate "Validated deployment configuration" 0 0
publish_progress running network "Checking controller network and binary cache" 0 0

META="$(nix eval "$FLAKE_URL#labMeta" --json --no-write-lock-file)" \
  || fail "could not evaluate labMeta"
IFACE="$(jq -er '.network.ifaceName' <<<"$META")" \
  || fail "labMeta does not contain a valid interface"
CONFIGURED_DHCP_IP="$(jq -er '.controller.dhcpIp' <<<"$META")" \
  || fail "labMeta does not contain a valid controller DHCP address"
STATIC_IP="$(jq -er '.controller.staticIp' <<<"$META")" \
  || fail "labMeta does not contain a valid controller static address"
CACHE_PORT="$(jq -er '.network.cachePort' <<<"$META")" \
  || fail "labMeta does not contain a valid cache port"
PXE_HTTP_PORT="$(jq -er '.network.pxeHttpPort' <<<"$META")" \
  || fail "labMeta does not contain a valid PXE HTTP port"

ip link show dev "$IFACE" >/dev/null \
  || fail "configured interface $IFACE does not exist"
mapfile -t ADDRESSES < <(ip -4 -o addr show dev "$IFACE" scope global \
  | awk '{ split($4, address, "/"); print address[1] }')
((${#ADDRESSES[@]} > 0)) \
  || fail "configured interface $IFACE has no global IPv4 address"
DHCP_IP=""
OBSERVED_NON_STATIC=()
for address in "${ADDRESSES[@]}"; do
  if [[ "$address" != "$STATIC_IP" && "$address" != 169.254.* ]]; then
    OBSERVED_NON_STATIC+=("$address")
    [[ "$address" == "$CONFIGURED_DHCP_IP" ]] && DHCP_IP="$address"
  fi
done
if [[ -z "$DHCP_IP" ]]; then
  if [[ "${#OBSERVED_NON_STATIC[@]}" -eq 1 ]]; then
    DHCP_IP="${OBSERVED_NON_STATIC[0]}"
    echo "Configured DHCP address $CONFIGURED_DHCP_IP is no longer assigned; preparing PXE for the unambiguous live address $DHCP_IP"
  else
    OBSERVED="${OBSERVED_NON_STATIC[*]:-none}"
    fail "configured DHCP address $CONFIGURED_DHCP_IP is not assigned to $IFACE and the live controller address is ambiguous (observed non-static addresses: $OBSERVED)"
  fi
fi

systemctl --no-ask-password is-active --quiet nixorium-harmonia.service \
  || fail "Harmonia cache service is not active"
curl --fail --silent --show-error --max-time 5 \
  "http://$DHCP_IP:$CACHE_PORT/nix-cache-info" | grep -q '^StoreDir:' \
  || fail "Harmonia cache health check failed at $DHCP_IP:$CACHE_PORT"

publish_progress running network "Controller network and binary cache are ready" 0 0

@PXE_BUILD@

mapfile -t CLIENT_NAMES < <(jq -er '.clients.hosts[].name' <<<"$META")
# Parallel to the names: computers of a class share one system.
# shellcheck disable=SC2034 # read by build_pxe_outputs
mapfile -t CLIENT_CLASSES < <(jq -er '.clients.hosts[] | (.class // .name)' <<<"$META")
publish_progress running clients \
  "Evaluating shared artifacts and client systems in bounded batches" 0 "${#CLIENT_NAMES[@]}"
build_pxe_outputs "$FLAKE_URL" "${CLIENT_NAMES[@]}"
[[ -f "$KERNEL_PATH/bzImage" ]] || fail "prepared kernel output lacks bzImage"
[[ -f "$INITRD_PATH/initrd" ]] || fail "prepared initrd output lacks initrd"
[[ -f "$IPXE_SCRIPT_PATH/netboot.ipxe" ]] || fail "prepared iPXE output lacks netboot.ipxe"
[[ -f "$FIRMWARE_PATH/snponly.efi" ]] || fail "prepared firmware output lacks snponly.efi"

CLIENTS_FILE="$(mktemp "$STATE_DIRECTORY/.clients.XXXXXX")"
MANIFEST_TEMP="$(mktemp "$STATE_DIRECTORY/.prepared.XXXXXX")"
for index in "${!CLIENT_NAMES[@]}"; do
  printf '%s\t%s\n' "${CLIENT_NAMES[$index]}" "${CLIENT_PATHS[$index]}" >>"$CLIENTS_FILE"
done
publish_progress running clients "Built shared artifacts and all client systems" \
  "${#CLIENT_NAMES[@]}" "${#CLIENT_NAMES[@]}"
CLIENTS="$(jq -Rn '[inputs | split("\t") | {name: .[0], storePath: .[1]}]' <"$CLIENTS_FILE")"

publish_progress running publish "Retaining artifacts and publishing the manifest" 0 0
ROOTS_DIRECTORY="$STATE_DIRECTORY/roots"
ROOT_GENERATION="$ROOTS_DIRECTORY/$REVISION"
install -d -m 0755 "$ROOT_GENERATION"
root_path() {
  local name="$1"
  local store_path="$2"
  local root="$ROOT_GENERATION/$name"
  if [[ -L "$root" ]]; then
    [[ "$(readlink -f "$root")" == "$store_path" ]] \
      || fail "existing PXE GC root $name differs at revision $REVISION"
    return
  fi
  [[ ! -e "$root" ]] || fail "PXE GC root $name is not a symlink"
  nix-store --realise "$store_path" --add-root "$root" --indirect >/dev/null \
    || fail "could not retain prepared store path for $name"
}
root_path kernel "$KERNEL_PATH"
root_path initrd "$INITRD_PATH"
root_path ipxe-script "$IPXE_SCRIPT_PATH"
root_path firmware "$FIRMWARE_PATH"
while IFS=$'\t' read -r name client_path; do
  root_path "client-$name" "$client_path"
done <"$CLIENTS_FILE"

[[ "$(repo_git rev-parse HEAD)" == "$REVISION" \
    && -z "$(repo_git status --porcelain=v1 --untracked-files=normal)" ]] \
  || fail "deployment changed during preparation; prepare the reviewed revision again"

jq -n \
  --arg revision "$REVISION" \
  --arg preparedAt "$(date --utc --iso-8601=seconds)" \
  --arg iface "$IFACE" \
  --arg dhcpIp "$DHCP_IP" \
  --arg staticIp "$STATIC_IP" \
  --argjson cachePort "$CACHE_PORT" \
  --argjson pxeHttpPort "$PXE_HTTP_PORT" \
  --arg kernel "$KERNEL_PATH" \
  --arg initrd "$INITRD_PATH" \
  --arg ipxeScript "$IPXE_SCRIPT_PATH" \
  --arg firmware "$FIRMWARE_PATH" \
  --argjson clients "$CLIENTS" \
  '{
    schemaVersion: 1,
    revision: $revision,
    preparedAt: $preparedAt,
    controller: { dhcpIp: $dhcpIp, staticIp: $staticIp },
    network: { ifaceName: $iface, cachePort: $cachePort, pxeHttpPort: $pxeHttpPort },
    artifacts: {
      kernel: { storePath: $kernel, relativePath: "bzImage" },
      initrd: { storePath: $initrd, relativePath: "initrd" },
      ipxeScript: { storePath: $ipxeScript, relativePath: "netboot.ipxe" },
      firmware: { storePath: $firmware, relativePath: "snponly.efi" }
    },
    clients: $clients
  }' >"$MANIFEST_TEMP"
chmod 0644 "$MANIFEST_TEMP"
sync -f "$MANIFEST_TEMP"
mv -T "$MANIFEST_TEMP" "$STATE_DIRECTORY/prepared.json"
sync -f "$STATE_DIRECTORY"
MANIFEST_TEMP=""
rm -f -- "$CLIENTS_FILE"
CLIENTS_FILE=""
find "$ROOTS_DIRECTORY" -mindepth 1 -maxdepth 1 -type d \
  ! -name "$REVISION" -exec rm -rf -- {} +
publish_progress completed complete \
  "Prepared PXE artifacts for ${#CLIENT_NAMES[@]} clients" \
  "${#CLIENT_NAMES[@]}" "${#CLIENT_NAMES[@]}"
rm -rf -- "$BUILD_DIRECTORY"
BUILD_DIRECTORY=""
trap - EXIT
echo "Prepared PXE artifacts at revision $REVISION"
