#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT
STATE_DIRECTORY="$TEMP_DIR/state"
mkdir -p "$STATE_DIRECTORY" "$TEMP_DIR/bin"
ORIGINAL_PATH="$PATH"
TEST_NIX_EXECUTABLE="$(readlink -f "$(command -v nix)")"
export NIXORIUM_TEST_STORE_PATH="${TEST_NIX_EXECUTABLE%/bin/nix}"
BASH_PATH="$(readlink -f "$BASH")"
SECOND_STORE_PATH="${BASH_PATH%/bin/bash}"
# Hosted CI may run this script with /usr/bin/bash instead of a store Bash.
if [[ ! "$SECOND_STORE_PATH" =~ ^/nix/store/[0-9a-z]{32}-[^/[:space:]]+$ \
    || "$SECOND_STORE_PATH" == "$NIXORIUM_TEST_STORE_PATH" ]]; then
  for STORE_CANDIDATE in /nix/store/*; do
    if [[ "$STORE_CANDIDATE" =~ ^/nix/store/[0-9a-z]{32}-[^/[:space:]]+$ \
        && -d "$STORE_CANDIDATE" && "$STORE_CANDIDATE" != "$NIXORIUM_TEST_STORE_PATH" ]]; then
      SECOND_STORE_PATH="$STORE_CANDIDATE"
      break
    fi
  done
fi
export NIXORIUM_TEST_SECOND_STORE_PATH="$SECOND_STORE_PATH"
export NIXORIUM_TEST_LOG="$TEMP_DIR/calls"

cat >"$TEMP_DIR/bin/nix" <<'NIX'
#!/usr/bin/env bash
set -euo pipefail
printf '%q ' "$@" >>"$NIXORIUM_TEST_LOG"
printf '\n' >>"$NIXORIUM_TEST_LOG"
case "$1" in
  eval)
    [[ "$*" == *'--no-write-lock-file --no-update-lock-file' ]]
    [[ "$NIXORIUM_PXE_BUILD_FLAKE" == git+file:///deployment* ]]
    jq -n --arg first "$NIXORIUM_TEST_STORE_PATH" --arg second "$NIXORIUM_TEST_SECOND_STORE_PATH" \
      --arg mode "${NIXORIUM_TEST_MODE:-}" --argjson names "$NIXORIUM_PXE_BUILD_CLIENTS" '
      def first: {drvPath: "/nix/store/00000000000000000000000000000000-first.drv", storePath: $first};
      def second: {drvPath: "/nix/store/11111111111111111111111111111111-second.drv", storePath: $second};
      {artifacts: [first, first, first, first],
       clients: [({name: $names[0]} + first), ({name: $names[1]} + second)]}
      | if $mode == "invalid" then .clients[0].storePath = "/tmp" else . end
      | if $mode == "invalid-derivation" then .clients[0].drvPath = "/tmp/build.drv" else . end
      | if $mode == "identity" then .clients[0].name = "pc99" else . end
    '
    ;;
  build)
    [[ "$2" == /nix/store/00000000000000000000000000000000-first.drv^out ]]
    [[ "$3" == /nix/store/11111111111111111111111111111111-second.drv^out ]]
    [[ "$4" == --out-link && "$#" -eq 5 ]]
    [[ "${NIXORIUM_TEST_MODE:-}" != failure ]] || exit 7
    LINK="$5"
    # Reverse completion order; mapping must come from evaluated identities.
    [[ "${NIXORIUM_TEST_MODE:-}" == missing ]] || ln -s "$NIXORIUM_TEST_SECOND_STORE_PATH" "$LINK-1"
    ln -s "$NIXORIUM_TEST_STORE_PATH" "$LINK"
    [[ "${NIXORIUM_TEST_MODE:-}" != extra ]] || ln -s /tmp "$LINK-extra"
    ;;
  *) exit 7 ;;
esac
exit 0
NIX
chmod +x "$TEMP_DIR/bin/nix"
export PATH="$TEMP_DIR/bin:$PATH"
source "$REPO_ROOT/scripts/lib/pxe-build.sh"
fail() { echo "Error: $*" >&2; exit 1; }

build_pxe_outputs 'git+file:///deployment?rev=reviewed' pc02 pc01
[[ "$(wc -l <"$NIXORIUM_TEST_LOG")" -eq 2 ]]
[[ "${#CLIENT_PATHS[@]}" -eq 2 && "${CLIENT_PATHS[0]}" == "$NIXORIUM_TEST_STORE_PATH" ]]
[[ "${CLIENT_PATHS[1]}" == "$NIXORIUM_TEST_SECOND_STORE_PATH" ]]
[[ "$KERNEL_PATH" == "$NIXORIUM_TEST_STORE_PATH" && -L "$BUILD_DIRECTORY/result-1" ]]
for MODE in failure missing invalid invalid-derivation identity extra; do
  if (export NIXORIUM_TEST_MODE="$MODE"; build_pxe_outputs 'git+file:///deployment' pc02 pc01) >/dev/null 2>&1; then
    echo "Error: accepted $MODE build output" >&2
    exit 1
  fi
done
BEFORE="$(wc -l <"$NIXORIUM_TEST_LOG")"
if (build_pxe_outputs 'git+file:///deployment' pc01 '../outside') >/dev/null 2>&1; then
  echo 'Error: accepted invalid client identity' >&2
  exit 1
fi
[[ "$(wc -l <"$NIXORIUM_TEST_LOG")" -eq "$BEFORE" ]]
if (build_pxe_outputs 'git+file:///deployment') >/dev/null 2>&1; then
  echo 'Error: accepted empty client inventory' >&2
  exit 1
fi

if [[ "${NIXORIUM_TEST_PXE_NIX:-}" == 1 ]]; then
  # Exercise real indexed derivation links with two empty fixture outputs.
  # This builds no systems and needs only the already installed mkdir binary.
  export PATH="$ORIGINAL_PATH"
  mkdir -p "$TEMP_DIR/flake"
  MKDIR_PATH="$(dirname "$(readlink -f "$(command -v mkdir)")")/mkdir"
  cat >"$TEMP_DIR/flake/flake.nix" <<EOF
{
  outputs = { self }: let
    mkFixture = name: builtins.derivation {
      inherit name;
      system = "x86_64-linux";
      builder = builtins.appendContext "$MKDIR_PATH" { "${MKDIR_PATH%/bin/mkdir}".path = true; };
      args = [ "-p" (builtins.placeholder "out") ];
    };
    first = mkFixture "nixorium-pxe-link-first";
    second = mkFixture "nixorium-pxe-link-second";
  in {
    nixosConfigurations = {
      netboot.config.system.build = { kernel = first; netbootRamdisk = first; netbootIpxeScript = second; };
      pc02.config.system.build.toplevel = second;
      pc01.config.system.build.toplevel = first;
    };
    packages.x86_64-linux.pxeFirmware = first;
  };
}
EOF
  build_pxe_outputs "path:$TEMP_DIR/flake" pc02 pc01
  [[ "$KERNEL_PATH" == *-nixorium-pxe-link-first ]]
  [[ "$IPXE_SCRIPT_PATH" == *-nixorium-pxe-link-second ]]
  [[ "${CLIENT_PATHS[0]}" == "$IPXE_SCRIPT_PATH" ]]
  [[ "${CLIENT_PATHS[1]}" == "$KERNEL_PATH" ]]
  # Retain compatibility with deployment fixtures exporting literal store paths.
  sed -i 's/first = mkFixture "nixorium-pxe-link-first";/first = toString (mkFixture "nixorium-pxe-link-first");/; s/second = mkFixture "nixorium-pxe-link-second";/second = toString (mkFixture "nixorium-pxe-link-second");/' "$TEMP_DIR/flake/flake.nix"
  build_pxe_outputs "path:$TEMP_DIR/flake" pc02 pc01
  [[ "${CLIENT_PATHS[0]}" == "$IPXE_SCRIPT_PATH" ]]
  [[ "${CLIENT_PATHS[1]}" == "$KERNEL_PATH" ]]
fi

echo 'PXE grouped build regression checks completed successfully.'
