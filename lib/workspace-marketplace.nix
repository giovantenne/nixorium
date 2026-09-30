{ lib, pkgs }:
# Add pinned Marketplace extensions to the package set seen by workspace
# resolution and seed builds. Each pin fetches exact bytes (by hash) on the
# controller at build time; clients receive the result like any package.
# A pin replaces a packaged extension with the same identifier.
entries:
let
  build = entry: pkgs.vscode-utils.buildVscodeMarketplaceExtension {
    mktplcRef = {
      inherit (entry) publisher name version hash;
    } // lib.optionalAttrs (entry ? platform) { arch = entry.platform; };
  };
  pinned = lib.foldl' (result: entry:
    let publisher = lib.toLower entry.publisher; in
    result // {
      ${publisher} = (result.${publisher} or {}) // { ${lib.toLower entry.name} = build entry; };
    }
  ) {} entries;
in
if entries == [] then pkgs else pkgs // {
  vscode-extensions = pkgs.vscode-extensions // builtins.mapAttrs (publisher: extensions:
    (pkgs.vscode-extensions.${publisher} or {}) // extensions
  ) pinned;
}
