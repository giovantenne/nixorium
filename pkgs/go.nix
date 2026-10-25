{ fetchurl, go_1_26, lib, runtimeShell, stdenv }:

# Security floor for the Go toolchain that builds Nixorium. The pinned package
# set can lag behind Go point releases with standard-library fixes; until
# nixpkgs ships the release, use the official build from go.dev instead of
# compiling Go on every controller after each package update.
let
  minimum = "1.26.9";
  platform = "${stdenv.hostPlatform.go.GOOS}-${stdenv.hostPlatform.go.GOARCH}";
  binaryHashes = {
    linux-amd64 = "sha256-QtFYtNj3thrAqDBWfJQKhgmPt6rFLkZ6Xr7APvXML40=";
  };
  tools = "share/go/pkg/tool/${stdenv.hostPlatform.go.GOOS}_${stdenv.hostPlatform.go.GOARCH}";

  official = stdenv.mkDerivation {
    pname = "go";
    version = minimum;
    src = fetchurl {
      url = "https://go.dev/dl/go${minimum}.${platform}.tar.gz";
      hash = binaryHashes.${platform};
    };
    # The official files are used as published: no stripping or patching.
    dontConfigure = true;
    dontBuild = true;
    dontFixup = true;
    installPhase = ''
      runHook preInstall
      mkdir -p $out/share/go $out/bin
      cp -a . $out/share/go
      # The official linker writes /lib64/ld-linux…, which NixOS lacks. The
      # nixpkgs Go reads GO_LDSO at link time instead (buildGoModule sets it to
      # the compiler's loader); do the same in front of the linker. A later -I
      # given by the caller still wins.
      mv $out/${tools}/link $out/${tools}/link-unwrapped
      cat > $out/${tools}/link <<EOF
      #!${runtimeShell}
      if [ -n "\''${GO_LDSO:-}" ]; then
        exec -a "\$0" "$out/${tools}/link-unwrapped" -I "\$GO_LDSO" "\$@"
      fi
      exec -a "\$0" "$out/${tools}/link-unwrapped" "\$@"
      EOF
      chmod +x $out/${tools}/link
      ln -s $out/share/go/bin/go $out/share/go/bin/gofmt $out/bin
      runHook postInstall
    '';
    passthru = {
      inherit (go_1_26) GOOS GOARCH CGO_ENABLED;
    };
    meta = go_1_26.meta // {
      sourceProvenance = [ lib.sourceTypes.binaryNativeCode ];
    };
  };
in
if lib.versionAtLeast go_1_26.version minimum then go_1_26
else if binaryHashes ? ${platform} then official
else go_1_26.overrideAttrs (_: {
  version = minimum;
  src = fetchurl {
    url = "https://go.dev/dl/go${minimum}.src.tar.gz";
    hash = "sha256-lzXX3Ntls10/pXfwQGRzfAO4nPGitx5uaf4vPG+f1Mo=";
  };
})
