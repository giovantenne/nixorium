#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

source "$REPO_ROOT/scripts/home-reset.sh"
mkdir -p "$TEST_ROOT/backgrounds"
mapfile -d '' -t WALLPAPERS < <(nixorium_collect_wallpapers "$TEST_ROOT/backgrounds")
[[ ${#WALLPAPERS[@]} -eq 0 ]]
touch "$TEST_ROOT/backgrounds/a.jpeg" "$TEST_ROOT/backgrounds/b.PNG" "$TEST_ROOT/backgrounds/readme.txt"
mkdir "$TEST_ROOT/backgrounds/directory.jpg"
mapfile -d '' -t WALLPAPERS < <(nixorium_collect_wallpapers "$TEST_ROOT/backgrounds")
[[ ${#WALLPAPERS[@]} -eq 2 ]]
[[ "${WALLPAPERS[0]}" == "$TEST_ROOT/backgrounds/a.jpeg" ]]
[[ "${WALLPAPERS[1]}" == "$TEST_ROOT/backgrounds/b.PNG" ]]

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
echo 'UX shell regression tests passed.'
