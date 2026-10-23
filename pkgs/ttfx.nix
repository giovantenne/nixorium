{ fetchFromGitHub, lib, rustPlatform }:

# Terminal text effects for the lab screensaver: a Rust port of
# terminaltexteffects that uses a fraction of its CPU. Packaged here until
# nixpkgs ships it; then the screensaver uses nixpkgs' ttfx instead.
rustPlatform.buildRustPackage rec {
  pname = "ttfx";
  version = "0.3.2";
  src = fetchFromGitHub {
    owner = "omacom-io";
    repo = "ttfx";
    rev = "v${version}";
    hash = "sha256-bwFjC6ZkZibkgXjoYVH2VuqqeXklGR9kmRl2fTitWBU=";
  };
  cargoHash = "sha256-DNrg12MNqBcQi6yvoJObM1gtE90iGBCxeQ3RwueYCE4=";
  meta = {
    description = "Terminal text effects, a Rust port of terminaltexteffects";
    homepage = "https://github.com/omacom-io/ttfx";
    license = lib.licenses.mit;
    mainProgram = "ttfx";
  };
}
