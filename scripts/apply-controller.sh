# shellcheck shell=bash
# Body of nixorium-apply-controller. modules/management.nix replaces the
# placeholders written between at signs (the deployment path and the operation
# gate) and wraps it with writeShellApplication.
REPOSITORY=@REPOSITORY@
EXPECTED_REVISION="${1:-}"

STATE_DIRECTORY=/var/lib/nixorium/controller
[[ -d "$STATE_DIRECTORY" && ! -L "$STATE_DIRECTORY" \
    && "$(stat -c '%U:%G:%a' "$STATE_DIRECTORY")" == root:root:755 ]] || {
  echo "Error: controller state directory has unsafe ownership or permissions" >&2
  exit 1
}
PROGRESS_FILE="$STATE_DIRECTORY/progress.json"
PROGRESS_STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
PROGRESS_STATE=running
PROGRESS_PHASE=starting
PROGRESS_CURRENT=0
PROGRESS_TOTAL=4
PROGRESS_RECENT='[]'
TEMPORARY_RECORD=""
keep_temporary=false
USB_WORKER_PAUSED=false

publish_progress() {
  local state="$1"
  local phase="$2"
  local activity="$3"
  local current="$4"
  local temporary

  PROGRESS_STATE="$state"
  PROGRESS_PHASE="$phase"
  PROGRESS_CURRENT="$current"
  PROGRESS_RECENT="$(jq -c --arg activity "$activity" \
    '. + [$activity] | if length > 5 then .[-5:] else . end' \
    <<<"$PROGRESS_RECENT")"
  temporary="$(mktemp "$STATE_DIRECTORY/.progress.XXXXXX")"
  jq -n \
    --arg operation controller-apply \
    --arg state "$PROGRESS_STATE" \
    --arg phase "$PROGRESS_PHASE" \
    --arg startedAt "$PROGRESS_STARTED_AT" \
    --arg updatedAt "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" \
    --argjson current "$PROGRESS_CURRENT" \
    --argjson total "$PROGRESS_TOTAL" \
    --argjson recent "$PROGRESS_RECENT" \
    '{schemaVersion: 1, operation: $operation, state: $state, phase: $phase,
      startedAt: $startedAt, updatedAt: $updatedAt, current: $current,
      total: $total, recent: $recent}' >"$temporary"
  chown admin:users "$temporary"
  chmod 0600 "$temporary"
  mv -fT -- "$temporary" "$PROGRESS_FILE"
  echo "$activity"
}

cleanup_controller() {
  local status="$?"
  if [[ "$keep_temporary" == true && -n "$TEMPORARY_RECORD" ]]; then
    rm -f -- "$TEMPORARY_RECORD"
  fi
  if [[ "$status" -ne 0 && "$PROGRESS_STATE" == running ]]; then
    publish_progress failed "$PROGRESS_PHASE" \
      "Controller apply stopped unexpectedly" "$PROGRESS_CURRENT" || true
  fi
  if [[ "$USB_WORKER_PAUSED" == true ]]; then
    # Release both locks before restarting the owner of the retained USB
    # reservation, including when validation/build/activation failed.
    exec 8>&- 9>&-
    systemctl --no-ask-password start --no-block nixorium-remote-install.service || true
  fi
}
trap cleanup_controller EXIT

fail() {
  local message="$*"
  if [[ "$PROGRESS_STATE" == running ]]; then
    publish_progress failed "$PROGRESS_PHASE" \
      "Failed: $message" "$PROGRESS_CURRENT" || true
  fi
  echo "Error: $message" >&2
  exit 1
}

@OPERATION_GATE@

publish_progress running starting "Starting controller apply" 0
publish_progress running validate "Validating the reviewed controller configuration" 0

[[ -d "$REPOSITORY" && ! -L "$REPOSITORY" ]] \
  || fail "configured deployment path is not a real directory"
[[ -f "$REPOSITORY/flake.nix" && -d "$REPOSITORY/.git" ]] \
  || fail "configured deployment path is not a Git Flake"
[[ "$(stat -c '%U' "$REPOSITORY")" == admin ]] \
  || fail "deployment repository must be owned by admin"
[[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" status --porcelain=v1 --untracked-files=normal)" ]] \
  || fail "deployment worktree must be clean before controller apply"
[[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" ls-files -- \
    secret-key admin-ssh lab-credentials.json)" ]] \
  || fail "private key files must not be tracked by Git"

REVISION="$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" rev-parse HEAD)"
[[ "$REVISION" =~ ^[0-9a-f]{40}$ ]] \
  || fail "deployment HEAD is not a full Git object ID"
if [[ -n "$EXPECTED_REVISION" ]]; then
  [[ "$EXPECTED_REVISION" =~ ^[0-9a-f]{40}$ ]] \
    || fail "expected controller revision is invalid"
  [[ "$REVISION" == "$EXPECTED_REVISION" ]] \
    || fail "deployment revision differs from the reviewed controller plan"
fi

# Pin the Git fetcher to the reviewed commit so ignored private keys are
# excluded and a concurrent clean commit cannot change the build input.
[[ -d /var/cache/nixorium/admin && ! -L /var/cache/nixorium/admin \
    && "$(stat -c '%U:%G:%a' /var/cache/nixorium/admin)" == admin:users:700 ]] \
  || fail "administrator Nix cache directory has unsafe ownership or permissions"
export XDG_CACHE_HOME=/var/cache/nixorium/admin
export NIX_CONFIG="experimental-features = nix-command flakes"

as_admin() {
  runuser -u admin -- "$@"
}

FLAKE_URL="$(as_admin nixorium config source --repo "$REPOSITORY" --revision "$REVISION")"

as_admin nixorium config validate --repo "$REPOSITORY" --json \
  || fail "deployment configuration validation failed"
CONTROLLER_READINESS="$(as_admin nix eval "$FLAKE_URL#deploymentStatus" --json --no-write-lock-file \
  | jq -ce '.controller')" \
  || fail "deployment status does not report controller readiness"
jq -e '.ready == true' <<<"$CONTROLLER_READINESS" >/dev/null \
  || fail "controller readiness must be true before controller apply"

# Only an explicit capability from the pinned configuration can omit lab
# keys. Old deployments continue to require both installed pairs.
if ! jq -e '.requiresKeys == false' <<<"$CONTROLLER_READINESS" >/dev/null; then
  as_admin nixorium setup keys --verify-only --repo "$REPOSITORY" --json \
    || fail "deployment key correspondence verification failed"
  cmp -s "$REPOSITORY/admin-ssh" /home/admin/.ssh/id_ed25519 \
    || fail "installed admin SSH key is absent or differs"
  cmp -s "$REPOSITORY/secret-key" /var/lib/nixorium/keys/harmonia-secret-key \
    || fail "installed Harmonia signing key is absent or differs"
fi

publish_progress running build "Validated controller prerequisites" 1
publish_progress running build "Building the reviewed controller system" 1

LAB_META="$(as_admin nix eval "$FLAKE_URL#labMeta" --json --no-write-lock-file)" \
  || fail "could not evaluate controller network metadata"
CONTROLLER_NAME="$(jq -er '.controller.name' <<<"$LAB_META")" \
  || fail "evaluated controller name is missing"
[[ "$CONTROLLER_NAME" =~ ^pc[0-9]+$ ]] \
  || fail "evaluated controller name is invalid"
DEPLOYMENT_MODE="$(jq -er '.deploymentMode // "laboratory"' <<<"$LAB_META")" \
  || fail "evaluated deployment mode is missing"
[[ "$DEPLOYMENT_MODE" == controller || "$DEPLOYMENT_MODE" == laboratory ]] \
  || fail "evaluated deployment mode is invalid"
if [[ "$DEPLOYMENT_MODE" == laboratory ]]; then
  [[ ! -e /var/lib/nixorium/pxe/session.json \
      && ! -L /var/lib/nixorium/pxe/session.json ]] \
    || fail "PXE networking is transitioning or active; stop or recover installation mode before controller apply"
  CONTROLLER_IFACE="$(jq -er '.controller.ifaceName // .network.ifaceName' <<<"$LAB_META")" \
    || fail "evaluated controller interface is missing"
  CONTROLLER_STATIC_IP="$(jq -er '.controller.staticIp' <<<"$LAB_META")" \
    || fail "evaluated controller static address is missing"
  CONTROLLER_PREFIX_LENGTH="$(jq -er '.network.prefixLength' <<<"$LAB_META")" \
    || fail "evaluated network prefix is missing"
  [[ "$CONTROLLER_IFACE" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,14}$ ]] \
    || fail "evaluated controller interface is invalid"
  [[ "$CONTROLLER_STATIC_IP" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] \
    || fail "evaluated controller static address is invalid"
  [[ "$CONTROLLER_PREFIX_LENGTH" =~ ^[0-9]+$ \
      && "$CONTROLLER_PREFIX_LENGTH" -ge 1 \
      && "$CONTROLLER_PREFIX_LENGTH" -le 30 ]] \
    || fail "evaluated network prefix is invalid"
fi

SYSTEM_PATH="$(as_admin nix build "$FLAKE_URL#nixosConfigurations.$CONTROLLER_NAME.config.system.build.toplevel" \
  --no-write-lock-file --no-link --print-out-paths)"
[[ "$SYSTEM_PATH" == /nix/store/* && "$SYSTEM_PATH" != *[[:space:]]* \
    && -x "$SYSTEM_PATH/bin/switch-to-configuration" ]] \
  || fail "controller build did not return one valid NixOS system closure"
[[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" status --porcelain=v1 --untracked-files=normal)" \
    && "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" rev-parse HEAD)" == "$REVISION" ]] \
  || fail "deployment changed while the controller was building; activation refused"

publish_progress running activate "Built controller system; beginning activation" 2
ACTIVATION_RECORD="$STATE_DIRECTORY/applied.json"
if [[ -L "$ACTIVATION_RECORD" ]]; then
  fail "refusing symlink controller activation record"
fi
if [[ -e "$ACTIVATION_RECORD" && ! -f "$ACTIVATION_RECORD" ]]; then
  fail "controller activation record is not a regular file"
fi

# Invalidate any earlier success before activation. A failed switch can
# update /run/current-system before a later activation snippet fails, so
# the active symlink alone is not durable evidence of completion.
rm -f -- "$ACTIVATION_RECORD"
# Match nixos-rebuild switch: register the generation before activation
# so boot entries, rollback history, and GC retention use this system.
nix-env --profile /nix/var/nix/profiles/system --set "$SYSTEM_PATH" \
  || fail "could not register the controller system generation"
[[ "$(readlink -f /nix/var/nix/profiles/system)" == "$SYSTEM_PATH" ]] \
  || fail "persistent system profile differs before controller activation"
"$SYSTEM_PATH/bin/switch-to-configuration" switch
ACTIVE_SYSTEM="$(readlink -f /run/current-system)"
[[ "$ACTIVE_SYSTEM" == "$SYSTEM_PATH" ]] \
  || fail "active system differs after controller activation"
[[ "$(readlink -f /nix/var/nix/profiles/system)" == "$SYSTEM_PATH" ]] \
  || fail "persistent system profile differs after controller activation"

# NetworkManager's profile nixorium-lab carries the controller's
# laboratory address; activating it is asynchronous, and a controller
# switching live from an earlier release (or from controller-only mode)
# may still hold another connection on the card. Wait for the reviewed
# address, activate the profile if it does not come, and verify it
# before PXE preparation is allowed to continue. During network
# installation the PXE session owns the address and it stays away.
static_address_present() {
  ip -4 -o address show dev "$CONTROLLER_IFACE" scope global \
    | awk -v cidr="$CONTROLLER_STATIC_CIDR" \
      '$3 == "inet" && $4 == cidr { found = 1 } END { exit !found }'
}
if [[ "$DEPLOYMENT_MODE" == laboratory && ! -e /var/lib/nixorium/pxe/session.json ]]; then
  CONTROLLER_STATIC_CIDR="$CONTROLLER_STATIC_IP/$CONTROLLER_PREFIX_LENGTH"
  ip link show dev "$CONTROLLER_IFACE" >/dev/null \
    || fail "configured controller interface is not present after activation"
  if nmcli -t general status >/dev/null 2>&1; then
    for _ in $(seq 30); do
      static_address_present && break
      sleep 1
    done
    if ! static_address_present; then
      # A copy of nixorium-lab saved outside Nixorium (GNOME Settings) has no
      # laboratory address; the profile writer removes it and restores the
      # declared profile.
      systemctl restart NetworkManager-ensure-profiles.service || true
      for _ in $(seq 30); do
        static_address_present && break
        sleep 1
      done
    fi
    if ! static_address_present; then
      nmcli --wait 60 connection up nixorium-lab ifname "$CONTROLLER_IFACE" >/dev/null \
        || fail "could not activate the laboratory network profile; run: nmcli connection up nixorium-lab ifname $CONTROLLER_IFACE"
    fi
  elif ! static_address_present; then
    # Without NetworkManager (a minimal controller) set the address itself.
    ip address replace "$CONTROLLER_STATIC_CIDR" dev "$CONTROLLER_IFACE" \
      || fail "could not apply the controller static address after activation"
  fi
  static_address_present \
    || fail "controller static address is absent after activation; run: nmcli connection up nixorium-lab ifname $CONTROLLER_IFACE"
fi

publish_progress running verify "Activated controller system and verified networking" 3

TEMPORARY_RECORD="$(mktemp --tmpdir="$STATE_DIRECTORY" .applied.json.XXXXXX)"
keep_temporary=true
jq -n \
  --arg revision "$REVISION" \
  --arg systemPath "$SYSTEM_PATH" \
  --arg activatedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{schemaVersion: 1, revision: $revision, systemPath: $systemPath, activatedAt: $activatedAt}' \
  > "$TEMPORARY_RECORD"
chmod 0644 "$TEMPORARY_RECORD"
sync "$TEMPORARY_RECORD"
mv -fT -- "$TEMPORARY_RECORD" "$ACTIVATION_RECORD"
keep_temporary=false
sync "$STATE_DIRECTORY"
publish_progress completed complete "Controller revision activated and verified" 4
