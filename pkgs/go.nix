{ fetchurl, go_1_26, lib }:

# Security floor for the Go toolchain that builds Nixorium. The pinned package
# set can lag behind Go point releases with standard-library fixes; rebuild that
# release from the official source until nixpkgs ships it, then use nixpkgs as is.
let
  minimum = "1.26.9";
in
if lib.versionAtLeast go_1_26.version minimum then go_1_26
else go_1_26.overrideAttrs (_: {
  version = minimum;
  src = fetchurl {
    url = "https://go.dev/dl/go${minimum}.src.tar.gz";
    hash = "sha256-lzXX3Ntls10/pXfwQGRzfAO4nPGitx5uaf4vPG+f1Mo=";
  };
})
