{ lib, pkgs }:
{ profile, root ? null }:
let
  checked = import ./eval-home-files.nix { inherit lib; } (builtins.toJSON profile);
  files = checked.files or [];
  sourceFor = entry:
    if entry ? text then pkgs.writeText "student-home-text" entry.text
    else
      let
        hash = lib.removePrefix "imports/" entry.source;
        rootContents = if root == null then {} else builtins.readDir root;
        imports = if (rootContents.imports or null) == "directory" then builtins.readDir (root + "/imports") else {};
        source = root + "/${entry.source}";
      in
      if root == null || (imports.${hash} or null) != "regular" then throw "home files: missing regular imported payload"
      else if builtins.hashFile "sha256" source != hash then throw "home files: imported payload digest mismatch"
      else builtins.path { path = source; name = "student-home-import-${hash}"; };
  prepared = map (entry: { inherit entry; source = sourceFor entry; }) files;
in
pkgs.runCommand "nixorium-student-home-files" { nativeBuildInputs = [ pkgs.jq ]; } ''
  mkdir -p "$out/home"
  printf '[]\n' > entries.json
  total=0
  ${lib.concatMapStringsSep "\n" (path: ''
    install -d -m 0700 "$out/home/"${lib.escapeShellArg path}
  '') (checked.directories or [])}
  ${lib.concatMapStringsSep "\n" (item: ''
    size=$(stat -c %s -- ${lib.escapeShellArg (toString item.source)})
    if [ "$size" -gt 16777216 ]; then echo "home files: imported file exceeds 16 MiB" >&2; exit 1; fi
    total=$((total + size))
    if [ "$total" -gt 67108864 ]; then echo "home files: payload exceeds 64 MiB" >&2; exit 1; fi
    # Only fresh bytes and declared executability are carried into the seed.
    install -D -m ${if item.entry.executable or false then "0700" else "0600"} \
      ${lib.escapeShellArg (toString item.source)} "$out/home/"${lib.escapeShellArg item.entry.destination}
    digest=$(sha256sum -- ${lib.escapeShellArg (toString item.source)} | cut -d ' ' -f 1)
    jq --arg destination ${lib.escapeShellArg item.entry.destination} --arg sha256 "$digest" \
      --argjson size "$size" --argjson executable ${if item.entry.executable or false then "true" else "false"} \
      '. + [{destination:$destination,sha256:$sha256,size:$size,executable:$executable}]' entries.json > next.json
    mv next.json entries.json
  '') prepared}
  install -m 0600 entries.json "$out/manifest.json"
''
