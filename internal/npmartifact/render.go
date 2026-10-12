package npmartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

func TarballName(dep Dependency) string {
	hash := sha256.Sum256([]byte(dep.Integrity))
	return "nixorium-npm-" + hex.EncodeToString(hash[:8]) + ".tgz"
}

func Render(m Manifest) (string, string) {
	files := []string{}
	bins := []string{}
	keys := []string{}
	destinations := []string{}
	packageCopies := []string{}
	binCopies := []string{}
	parents := []string{"bin", "lib", "lib/node_modules"}
	for key := range m.Bins {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		binCopies = append(binCopies, `        ${pkgs.coreutils}/bin/cp -P --no-preserve=ownership "${baseline}/bin/`+key+`" "$PREFIX/bin/`+key+`"`)
		destinations = append(destinations, "bin/"+key)
		bins = append(bins, "  ln -s '../lib/node_modules/"+m.Package+"/"+m.Bins[key]+"' \"$out/bin/"+key+"\"\n  chmod +x \"$out/lib/node_modules/"+m.Package+"/"+m.Bins[key]+"\"\n  for FLAG in --version --help; do\n    ${pkgs.util-linux}/bin/prlimit --as=4294967296 --data=2147483648 --nproc=32 --cpu=30 --fsize=67108864 timeout 30 ${pkgs.bubblewrap}/bin/bwrap --unshare-all --die-with-parent --new-session --ro-bind /nix/store /nix/store --tmpfs /tmp --dir /tmp/home --proc /proc --dev /dev --clearenv --setenv HOME /tmp/home --setenv PATH ${pkgs.nodejs}/bin --setenv NODE_OPTIONS '--max-old-space-size=512 --disable-wasm-trap-handler' \"$out/bin/"+key+"\" \"$FLAG\" >/dev/null\n  done")
	}
	for _, dep := range m.Dependencies {
		if dep.Optional {
			continue
		}
		p := strings.TrimPrefix(dep.Path, "node_modules/")
		if !strings.Contains(p, "/node_modules/") {
			packageCopies = append(packageCopies, `        ${pkgs.coreutils}/bin/cp -R --no-preserve=ownership "${baseline}/lib/node_modules/`+p+`" "$PREFIX/lib/node_modules/`+p+`"`+"\n"+`        ${pkgs.coreutils}/bin/chmod -R u+w "$PREFIX/lib/node_modules/`+p+`"`)
			destinations = append(destinations, "lib/node_modules/"+p)
			if strings.HasPrefix(p, "@") {
				parents = append(parents, "lib/node_modules/"+strings.Split(p, "/")[0])
			}
		}
		url, _ := json.Marshal(dep.URL)
		integrity, _ := json.Marshal(dep.Integrity)
		files = append(files, "    (pkgs.fetchurl { url = "+string(url)+"; hash = "+string(integrity)+"; name = \""+TarballName(dep)+"\"; })")
	}
	// npm metadata/cache preparation never executes hooks. Install hooks and CLI
	// tests run only inside the explicit offline namespace in this expression.
	expression := strings.NewReplacer("@@FILES@@", strings.Join(files, "\n"), "@@BINS@@", strings.Join(bins, "\n"), "@@VERSION@@", m.Version, "@@NODE@@", m.NodeRuntime).Replace(`{ pkgs }:
let
  tarballs = [
@@FILES@@
  ];
in
assert toString pkgs.nodejs == "@@NODE@@" || throw "The prepared npm runtime changed; prepare this package again";
pkgs.runCommand "nixorium-npm-baseline-@@VERSION@@" {
  nativeBuildInputs = [ pkgs.nodejs pkgs.coreutils pkgs.findutils ];
  preferLocalBuild = true;
  allowSubstitutes = false;
} ''
  # Package hooks and CLIs execute only in explicit Bubblewrap namespaces,
  # independent of the controller daemon's sandbox configuration.
  export HOME="$TMPDIR/home"
  export NPM_CONFIG_CACHE="$TMPDIR/cache"
  export NPM_CONFIG_USERCONFIG="$TMPDIR/userconfig"
  export NPM_CONFIG_GLOBALCONFIG="$TMPDIR/globalconfig"
  export NPM_CONFIG_UPDATE_NOTIFIER=false
  mkdir -p "$HOME" work
  touch "$NPM_CONFIG_USERCONFIG" "$NPM_CONFIG_GLOBALCONFIG"
  cp ${./package.json} work/package.json
  cp ${./package-lock.json} work/package-lock.json
  for tarball in ${pkgs.lib.escapeShellArgs (map toString tarballs)}; do
    npm cache add --ignore-scripts --offline "$tarball" >/dev/null
  done
  ${pkgs.util-linux}/bin/prlimit --as=4294967296 --data=2147483648 --nproc=32 --cpu=120 --fsize=536870912 \
  timeout 180 ${pkgs.bubblewrap}/bin/bwrap --unshare-all --die-with-parent --new-session \
    --ro-bind /nix/store /nix/store --bind "$PWD/work" /work --bind "$NPM_CONFIG_CACHE" /cache \
    --tmpfs /tmp --dir /tmp/home --proc /proc --dev /dev --clearenv \
    --setenv HOME /tmp/home --setenv PATH ${pkgs.lib.makeBinPath [ pkgs.nodejs pkgs.bash pkgs.coreutils ]} \
    --setenv NPM_CONFIG_CACHE /cache --setenv NPM_CONFIG_USERCONFIG /tmp/userconfig \
    --setenv NPM_CONFIG_GLOBALCONFIG /tmp/globalconfig --setenv NPM_CONFIG_UPDATE_NOTIFIER false \
    --setenv NPM_CONFIG_SCRIPT_SHELL ${pkgs.bash}/bin/bash --setenv NPM_CONFIG_OFFLINE true \
    --setenv NODE_OPTIONS '--max-old-space-size=512 --disable-wasm-trap-handler' \
    --remount-ro / --chdir /work ${pkgs.nodejs}/bin/npm ci --offline --omit=optional --no-audit --no-fund --engine-strict >/dev/null
  cd work
  test -z "$(find node_modules -name '*.node' -o -name binding.gyp)"
  rm -rf node_modules/.bin
  test -z "$(find node_modules -type l)"
  mkdir -p "$out/lib" "$out/bin" "$out/nix-support"
  cp -a node_modules "$out/lib/node_modules"
  patchShebangs "$out/lib/node_modules"
  # Keep public, verified sources in the installed/cache-distributed closure.
  printf '%s\n' ${pkgs.lib.escapeShellArgs (map toString tarballs)} > "$out/nix-support/npm-sources"
@@BINS@@
''
`)
	module := strings.NewReplacer("@@DESTINATIONS@@", strings.Join(destinations, " "), "@@PACKAGECOPIES@@", strings.Join(packageCopies, "\n"), "@@PARENTS@@", strings.Join(parents, " "), "@@BINCOPIES@@", strings.Join(binCopies, "\n")).Replace(`{ pkgs, lib, labSettings, ... }:
let
  baseline = import ./default.nix { inherit pkgs; };
in
{
  # Only one prepared baseline module may be imported per host. Conflicting
  # ExecStart definitions fail evaluation rather than overwriting another seed.
  systemd.services.nixorium-student-npm-baseline = {
    description = "Restore the administrator's prepared npm baseline after home reset";
    wantedBy = [ "multi-user.target" ];
    requiredBy = [ "systemd-user-sessions.service" "display-manager.service" ];
    after = [ "home-reset.service" ];
    requires = [ "home-reset.service" ];
    before = [ "systemd-user-sessions.service" "display-manager.service" ];
    restartIfChanged = false;
    stopIfChanged = false;
    unitConfig = {
      RefuseManualStart = true;
      "X-OnlyManualStart" = true;
      "X-StopOnRemoval" = false;
    };
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      Restart = "no";
      User = labSettings.studentUser;
      Group = "users";
      UMask = "0022";
      NoNewPrivileges = true;
      PrivateTmp = true;
      PrivateDevices = true;
      ProtectSystem = "strict";
      ProtectHome = "tmpfs";
      BindPaths = [ "/home/${labSettings.studentUser}/.local/npm" ];
      RestrictAddressFamilies = [ "AF_UNIX" ];
      CapabilityBoundingSet = [];
      ExecStart = pkgs.writeShellScript "nixorium-restore-npm-baseline" ''
        set -eu
        PREFIX="/home/${labSettings.studentUser}/.local/npm"
        # Never replace unrelated seed entries. Each destination must be absent.
        for ENTRY in @@PARENTS@@; do
          if [ -L "$PREFIX/$ENTRY" ]; then
            echo "Unsafe npm seed directory" >&2
            exit 1
          fi
        done
        for ENTRY in @@DESTINATIONS@@; do
          if [ -e "$PREFIX/$ENTRY" ] || [ -L "$PREFIX/$ENTRY" ]; then
            echo "Another npm seed owns $ENTRY; refusing to replace it" >&2
            exit 1
          fi
        done
        for ENTRY in @@PARENTS@@; do
          ${pkgs.coreutils}/bin/mkdir -p "$PREFIX/$ENTRY"
        done
@@PACKAGECOPIES@@
@@BINCOPIES@@
      '';
    };
  };
  environment.extraInit = lib.mkAfter ''
    export NPM_CONFIG_PREFIX="$HOME/.local/npm"
    export PATH="$HOME/.local/npm/bin:$PATH"
  '';
}
`)
	return expression, module
}
